# Stack Research

**Domain:** Go CLI moving its substrate (events, draws, knowledge pile) into shared PostgreSQL, queried as a property graph via Apache AGE, behind a durable local write queue
**Researched:** 2026-09-11
**Confidence:** HIGH (all versions verified against GitHub Releases/Tags API and go.mod files on 2026-09-11, not recalled from training data)

## Executive Summary

Five decisions, in order of how much they cost the binary and the user:

1. **Postgres driver: `github.com/jackc/pgx/v5` (v5.11.0, 2026-09-07).** Pure Go, no cgo. Already the project's stated choice — confirmed current, confirmed pure Go, confirmed the right choice. Use raw `pgx.Connect` (no pool) for short-lived CLI invocations; use `pgxpool` only inside the resident `hugel garden` TUI process.
2. **Apache AGE: current release 1.8.0, branch `release/PG18`, published 2026-07-09.** It is a **server-side Postgres extension**, not a Go library — it adds zero lines to `go.sum` and has no cgo implication for the `hugel` binary at all. It runs *inside* the Postgres server your binary connects to. It supports PG 11–18 as shipped releases; a PG19 branch exists but is **beta-only** (tagged against PG19beta1, no formal release cut) — do not let the Postgres server upgrade to 19 once it goes GA until AGE cuts a matching release. Do not add the official `apache/age` Go driver (stale, duplicate-driver, unmaintained) — call `cypher()` through plain `pgx`.
3. **Migrations: `github.com/pressly/goose/v3` (v3.28.0, 2026-09-02, requires Go ≥1.26 — exact match).** Embeds via `go:embed` + `fs.FS`, runs against `*sql.DB` (so it rides on `pgx/v5/stdlib`, already in `go.sum` — no second driver), and ships a Postgres advisory-lock session locker so migrations stay safe when triggered from more than one machine. `golang-migrate` is a legitimate, very close alternative — see below for the deciding factor.
4. **Local write queue: hand-rolled, reusing the existing fsynced JSONL append primitive. Zero new dependencies.** No established Go library dominates this space; every candidate (dque, goque, nutsdb, boltqueue) is a niche, thinly-maintained project that still leaves hugel to build the actual outbox semantics on top. Phase 01 already proved the durable-append half of this problem (fsync-before-return, error surfaced not swallowed, failure marker, health surface) — re-proving that inside someone else's KV store is pure downside.
5. **Local Postgres for dev/test: no automatic spin-up in the default test run.** AGE is a compiled Postgres extension, so pure-Go options (`embedded-postgres`, `pg_tmp`) cannot produce a database with AGE loaded — they only fetch stock upstream Postgres binaries. The only real options are Docker images that already have AGE baked in (`apache/age` on Docker Hub), driven by `testcontainers-go` (v0.44.0) for an opt-in, build-tag-gated integration suite, and by `docker-compose` for the developer's own always-on local dev database. Neither may run inside the default hermetic `go test ./...` — that stays exactly as offline as it is today.

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `github.com/jackc/pgx/v5` | v5.11.0 (2026-09-07) | Postgres wire-protocol driver | Pure Go (no cgo), the de facto standard Go Postgres driver, requires Go ≥1.25 (project is on 1.26). Already the project's committed choice per PROJECT.md; this research confirms it's current, not stale. |
| `github.com/jackc/pgx/v5/pgxpool` | same module, same version | Connection pool for the resident TUI process | Ships inside the pgx module (no separate version to track). Concurrency-safe; use only where the process is long-lived (`hugel garden`). |
| `github.com/pressly/goose/v3` | v3.28.0 (2026-09-02) | Schema migrations, embedded in the binary | `go.mod` requires `go 1.26.0` — matches the project exactly. `Provider` API operates on `*sql.DB` + `fs.FS`, so `go:embed` migrations compile straight into the single binary; ships a Postgres advisory-lock locker for safe multi-machine startup migrations. |
| Apache AGE (server extension) | 1.8.0 for PG18 (published 2026-07-09) | Property-graph query layer inside Postgres | Not a Go dependency — a `CREATE EXTENSION` on the server. Cypher via `SELECT * FROM cypher(...) AS (...)` through plain `pgx`, no ORM, no extra driver. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/golang-migrate/migrate/v4` + `database/pgx/v5` + `source/iofs` | v4.20.1 (2026-09-09) | Alternative migration runner | Only if goose's `Provider` model turns out to be a poor fit in practice — see "Alternatives Considered." Its `database/pgx/v5` subpackage reuses the same pgx already in `go.sum`, adding only `github.com/jackc/pgerrcode`. |
| `go.etcd.io/bbolt` | v1.5.0 (2026-06-21) | Escalation path for the local write queue | Only if the queue outgrows strict FIFO drain-in-order (e.g. needs keyed dedupe or out-of-order ack). Pure Go, no cgo, one dependency, mmap-based B+tree — the same store etcd and Consul use. Not needed today. |
| `github.com/testcontainers/testcontainers-go/modules/postgres` | v0.44.0 (2026-08-07) | Opt-in, build-tag-gated integration tests against a live Postgres+AGE | Never imported by the production binary or the default hermetic test run — confined to a `pgtest`-tagged test package that pulls the `apache/age` Docker image. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `apache/age` Docker image (Docker Hub) | Local dev database with AGE pre-installed | Matches AGE's own `docker-compose.yml` (`apache/age:dev_snapshot_master` for latest master, or a version-pinned tag for a stable release). Use for the developer's own always-on local DB. |
| `docker-compose` | Developer-run local Postgres+AGE, not test automation | Zero Go dependency — a YAML file the developer runs by hand. Do not wire it into `go test`. |

## Installation

```bash
# Core
go get github.com/jackc/pgx/v5@v5.11.0
go get github.com/pressly/goose/v3@v3.28.0

# Only if the opt-in Postgres+AGE integration suite is built
go get github.com/testcontainers/testcontainers-go/modules/postgres@v0.44.0

# Apache AGE itself is NOT a go get — it is installed on the Postgres server:
#   docker pull apache/age:<tag-matching-your-PG-major>
# or built from source per release/PGxx branch (gcc, flex, bison, PG dev headers).
```

## Detailed Findings by Question

### 1. Postgres driver

`jackc/pgx/v5` is current at **v5.11.0**, released **2026-09-07** (three days before this research). `go.mod` declares `go 1.25.0` as the floor, so it builds cleanly under the project's Go 1.26. It is pure Go — no cgo anywhere in the module graph — and its own `go.mod` pulls in only `pgpassfile`, `pgservicefile`, `puddle/v2` (pool internals), `golang.org/x/sync`, and `golang.org/x/text`. That is the entire footprint pgx adds to `go.sum`: five modules, all pure Go, all already widely vetted.

There is no serious alternative to consider here. `database/sql` with `lib/pq` is the other pure-Go option, but `lib/pq` is in maintenance mode (no new features, community has moved on) and using it alongside pgx (which some tooling, like golang-migrate's default postgres driver, still defaults to) would mean **two different Postgres drivers in one `go.sum`** for no benefit — avoid that combination explicitly (see "What NOT to Use").

**Pooling — short-lived CLI vs. resident TUI.** This is a real architectural fork, not a detail:
- **Short-lived CLI processes** (`hugel gate`, `hugel yield`, every one-shot invocation that runs "many times a day"): use a single `pgx.Connect(ctx, dsn)` / `pgx.ConnectConfig`, do the work, `defer conn.Close(ctx)`. A `pgxpool.Pool` exists to amortize connection setup cost across *many concurrent operations in one long-lived process* — spinning one up just to run one query and tear it down adds pool warm-up/min-conns bookkeeping for zero benefit. The pgx maintainer's own guidance (surfaced in the project's GitHub Discussions, #1700, "Best practice for long-running pgxpool") explicitly frames the pool as long-lived-app machinery; the CLI-shaped guidance is to construct the connection from an explicit config passed in by the caller (not a package-level `init()` singleton) so the connection string can come from a CLI flag as easily as an environment variable.
- **The resident TUI** (`hugel garden`, once it becomes session-persistent as this milestone plans): use `pgxpool.Pool`, created once at startup and passed down rather than held as a global. This is exactly the shape pgxpool is for — multiple goroutines (render loop, background refresh, tender-progress polling) safely sharing a small number of live connections for the life of the session.

Confidence: HIGH — version and `go.mod` floor confirmed directly against the GitHub API and raw `go.mod` file; pooling guidance corroborated by both the pgx maintainer discussion and standard idiomatic Go practice.

### 2. Apache AGE

**This is the finding most likely to be misjudged if research is skipped, so it gets the most space.**

Apache AGE branches and releases **per PostgreSQL major version**, not as one linear version line. Verified directly against the GitHub Releases and tags API for `apache/age` on 2026-09-11:

| PG major | Latest AGE release on that branch | Published |
|----------|-----------------------------------|-----------|
| 18 | 1.8.0 | 2026-07-09 |
| 17 | 1.7.0 | 2026-02-11 |
| 16 | 1.6.0 | (2025) |
| 15 | 1.6.0 | (2025) |
| 14 | 1.6.0 | (2025) |
| 11–13 | 1.3.0–1.5.0 | (older) |
| **19** | **no published release** — a `PG19/v1.8.0-rc0` git tag exists (commit dated 2026-07-06), but it has **no corresponding GitHub Release** | — |

The PG19 tag's own commit message says it plainly: *"NOTE: This is a beta release of Apache AGE on PostgreSQL 19 (19beta1) ... this is a beta release and you should backup everything before using them."* PostgreSQL 19 itself is not yet GA as of this research date — it is in beta (Beta 1 released 2026-06-04, Beta 3 released 2026-08-13), with GA expected imminently given the project's usual September/October cadence, but not yet announced. **Flag clearly: when PostgreSQL 19 goes GA, Apache AGE will not have a matching stable release yet — pin the Postgres server to 18 until AGE cuts a formal PG19 release, not just a beta-tagged branch.** This is a live, dated risk, not a hypothetical one, and it directly confirms and sharpens the caution already recorded in PROJECT.md's Key Decisions ("AGE supports PG 11–18 so the server pins below the current release").

**Installation** is server-side, not a Go dependency:
- Build from source per branch tag (`git checkout release/PG18/1.8.0`, `make install` — needs gcc, flex, bison, and the target Postgres's dev headers/`pg_config`), or
- The official `apache/age` Docker image (Docker Hub), tagged to match the PG major + AGE version, or `apache/age:dev_snapshot_master` for the bleeding edge.
- Once installed, a session enables it with `CREATE EXTENSION age; LOAD 'age'; SET search_path = ag_catalog, "$user", public;` (confirmed against the official quickstart). None of this touches `go.sum` or cgo — the `hugel` binary just connects to a Postgres server that happens to have the extension loaded.

**Cypher / openCypher coverage.** AGE implements a real subset of openCypher, and the gaps are concrete, not vague:
- Present and recently strengthened: `MATCH`/`CREATE`/`MERGE`/`WHERE`/`RETURN`/`WITH`/`ORDER BY`, variable-length path patterns, `shortest_path()`/`all_shortest_paths()` (added in 1.8.0, 2026-07), list comprehension and map projections (1.6.0+), predicate functions `all()`/`any()`/`none()`/`single()` (1.8.0), and — as of 1.8.0 — `MERGE ... ON CREATE SET ... ON MATCH SET`, which was **missing until this release** (confirmed via the 1.8.0 release notes; earlier AGE versions genuinely could not do conditional merge-set).
- Still missing/restricted, per the project's own tracked compliance issue (`apache/age#2323`, "Multiple standard features not supported"): `datetime()` and other temporal functions are unsupported; a `CREATE` clause with a labeled node must stand alone in its own clause (cannot be freely combined with other clause content the way openCypher allows); some function-call-expression positions are restricted.
- **Integration-relevant quirk, not a limitation exactly:** every `cypher()` call is wrapped in ordinary SQL and requires an explicit `AS (col1 type1, col2 type2, ...)` output shape on the outer `SELECT`. Forgetting or mis-ordering this is the most common cause of query failures reported in the ecosystem. For a `pgx`-based, no-ORM client this is actually a *good* fit: the return shape of every graph query is statically declared at the call site, matching hugel's existing "no ORM" discipline rather than fighting it.

**On the Go driver — do not add it.** `apache/age/drivers/golang`'s own `go.mod` requires Go 1.19 (stale relative to the project's 1.26), depends on `github.com/lib/pq` (a **second, different** Postgres driver alongside pgx — exactly the duplication to avoid), an ANTLR4 Go runtime to parse `agtype` results, and pins `testify v1.7.0` — all signs of a driver that has not kept pace with the rest of the AGE project. The idiomatic, and considerably lighter, integration is: issue `SELECT * FROM cypher('graphname', $$ ... $$) AS (v agtype)` through plain `pgx`, and decode `agtype` results directly — its wire format is JSON-like text with a trailing type tag, decodable with a small hand-written helper (or `encoding/json` after stripping the tag) rather than a whole extra driver stack.

Confidence: HIGH for version/release facts (verified against GitHub Releases and tags API directly, including the exact commit and its message for the PG19 tag). MEDIUM for the openCypher gap list (compiled from the project's own release notes and its own tracked compliance issue, which is about as authoritative as third-party research gets for pre-1.0-grade extension software, but the issue tracker is necessarily a snapshot and may have closed items since).

### 3. Migration tooling

Compared four: `golang-migrate`, `goose`, `tern`, `atlas`. All four have real, current 2026 releases (verified via GitHub Releases API):

| Tool | Latest | Published | Requires |
|------|--------|-----------|----------|
| `pressly/goose/v3` | v3.28.0 | 2026-09-02 | Go 1.26.0 |
| `golang-migrate/migrate/v4` | v4.20.1 | 2026-09-09 | Go 1.25.11 |
| `jackc/tern/v2` | v2.4.3 | 2026-08-23 | Go 1.25.0 |
| `ariga/atlas` | v1.3.0 | 2026-08-02 | — |

**goose (recommended).** Its core `goose` package works against `*sql.DB` (not a bespoke connection abstraction), so it rides on `pgx/v5/stdlib` — the `database/sql`-compatibility shim that already ships inside the pgx module the project is already using — with zero second driver. Its module-level `go.mod` looks enormous (it lists ClickHouse, MSSQL, Spanner, etc. as `require`s), but that is the *repository's* go.mod covering every dialect the CLI tool supports; the importable `goose` core package does not unconditionally import any of those driver packages — dialects are plain string/int constants, and only the driver you actually import (pgx's stdlib shim, in this case) reaches your build. Practical added footprint when embedding: the `goose` package itself plus `go.uber.org/multierr` (error aggregation) — everything else is pruned by Go's module graph. It embeds cleanly via `go:embed migrations/*.sql` + `goose.NewProvider(dialect, db, embedMigrations)`, supports per-file `-- +goose Up` / `-- +goose Down` annotations for explicit rollback, and ships `lock/postgres.go` — a Postgres advisory-lock session locker, directly relevant to "one garden reachable from several machines": if two machines both try to run startup migrations against the shared database at once, the second one blocks/waits on the lock rather than racing.

**golang-migrate — genuinely close, name the deciding factor.** Its dedicated `database/pgx/v5` subpackage is equally clean: it imports only `pgx/v5/pgconn`, `pgx/v5/stdlib`, and `jackc/pgerrcode` — one small addition beyond what pgx already contributes to `go.sum`. Its `source/iofs` package is the `go:embed` equivalent for migration files. It also does per-migration Up/Down files (as separate files, not annotated sections within one file — a matter of taste) and is the more widely-known tool by community size. **The deciding factor, not a dismissal:** golang-migrate's public embedding API is string-URL-based (`migrate.NewWithSourceInstance("iofs", ...)` / connection-string-shaped source and database identifiers), which is a slightly awkward fit for a program that already builds a typed `pgx.Config` once and wants to reuse it, whereas goose's `Provider` takes the already-open `*sql.DB` directly. If goose's `Provider` API turns out to have any rough edge in practice, golang-migrate is a safe, well-supported fallback with near-identical dependency cost — this is close enough that either is a defensible pick, and the roadmap should not treat switching later as a wasted decision.

**tern — not recommended for this project, despite being written by the same author as pgx.** tern is fundamentally a standalone CLI tool driven by a `tern.conf` file and a migrations directory, not designed to be embedded as a library the way goose's `Provider` is. Its `go.mod` pulls in `Masterminds/sprig/v3` (a full template-function library, for parameterizing migration SQL) and `spf13/cobra` (a CLI framework) — neither of which buys hugel anything once tern's own CLI entry point is bypassed in favor of embedding, since hugel already has its own CLI/flag handling. Paying for a templating engine and a second CLI framework inside a single binary that has neither ORM nor config library by design is the wrong trade.

**atlas — not recommended.** Atlas's headline capability is *declarative* schema management: you describe the desired end-state schema (HCL or SQL) and Atlas computes the diff against the live database, which for anything beyond trivial diffs requires spinning up a disposable "dev database" (in practice, via Docker) to validate the computed migration against. That is a materially heavier operational model — and a second Docker dependency alongside the one already needed for AGE-enabled test Postgres — for a capability (linear, hand-written, forward-only-plus-explicit-rollback migrations) that goose already covers directly. Its own dependency graph (an HCL parser, its own SQL-dialect abstraction layer) is sized for that different job.

**Startup / rollback summary:**
- **Embedding:** goose and golang-migrate both embed cleanly via `go:embed`; tern does not have an idiomatic embedding story; atlas embeds but drags in a much larger surface for capability not needed here.
- **Running at startup:** both goose and golang-migrate can run `Up()` on process start; goose's advisory-lock locker is the more directly relevant feature for the "several machines, one shared database" shape this milestone targets.
- **Rollback:** goose's per-file `-- +goose Down` and golang-migrate's separate `*.down.sql` files are equivalent in capability — write a down migration for every up migration in either tool, this is a discipline question, not a tooling one.

Confidence: HIGH — versions and `go.mod` contents verified directly; the embedding-API and locking claims verified by reading the actual source files (`dialect.go`, `provider.go`, `lock/postgres.go`, and the `database/pgx/v5/pgx.go` driver source), not summarized secondhand.

### 4. Durable local write queue

There is **no established, dominant Go library** for "disk-backed outbox in front of a remote database" the way there is for, say, HTTP routing. The candidates surfaced are all small, single-purpose, and thinly maintained relative to their scope claims:

| Library | What it is | Concern |
|---------|-----------|---------|
| `joncrlsn/dque` | Generic FIFO disk-backed durable queue, one dependency | Small maintainer base; still requires hugel to build outbox semantics (drain, ack, retry, compaction) on top |
| `beeker1121/goque` | Persistent stack/queue backed by LevelDB | Depends on `syndtr/goleveldb`, which has had long maintenance gaps |
| `nutsdb/nutsdb` | Embedded multi-structure KV store (list/set/sorted-set) with its own WAL/merge/GC | Much larger surface (its own compaction, its own crash-recovery story) than a strict FIFO drain needs — a second durability subsystem to trust alongside the one already proven |
| `flowchartsman/boltqueue` | Thin FIFO-queue wrapper over bbolt | Low activity/adoption; if bbolt-backed structure is ever wanted, easier to write the ~50 lines directly against `bbolt` than to add a wrapper around it |

**Recommendation: hand-roll it, reusing the fsynced JSONL append primitive that already exists in this codebase.** This is not "nothing exists so we're stuck" — it's that the hard part of this problem (fsync-before-return, error surfaced rather than swallowed, a failure marker dated to the first failure of a streak, and a health surface that distinguishes "nothing has run" from "writes have been failing") is **already built, already tested, and explicitly called out in PROJECT.md as the foundation this milestone's queue rests on.** Concretely: append each pending Postgres write (an event, a draw, a pile-entry mutation) as one JSONL record to a queue file using the existing append function; drain by reading forward from a cursor, sending each record to Postgres, and compacting the file (rewrite minus the consumed prefix) on confirmed commit — the same shape `events.jsonl`/`draws.jsonl` already use. Net new dependency: **zero.**

**Escalation path, if it's ever needed:** if the queue needs random/keyed access — e.g., deduplicating by an idempotency key, or acknowledging out of FIFO order — `go.etcd.io/bbolt` (v1.5.0, 2026-06-21) is the correct next tool. It is pure Go, no cgo, a single dependency, and is the same embedded B+tree store used inside etcd and Consul, so its durability properties are exceptionally well-exercised in production elsewhere. This is explicitly *not* needed for a strict FIFO drain-in-order queue, which is all the current requirement calls for.

Confidence: MEDIUM-HIGH — the "no dominant library" conclusion is a survey finding (absence-of-evidence has a lower ceiling than presence-of-evidence), but it is consistent across every search performed, and the reuse recommendation follows directly from constraints already stated in PROJECT.md rather than from external research.

### 5. Local Postgres for development and test

**The AGE requirement changes this answer from the generic Go-ecosystem default.** `testcontainers-go`, `embedded-postgres`, and `pg_tmp` are usually compared as roughly interchangeable "spin up a real Postgres for tests" options. They are not interchangeable here, because two of the three cannot produce a database with AGE loaded at all:

- **`fergusstrange/embedded-postgres`** (pure Go, no cgo, small dependency footprint — `lib/pq`, `xi2/xz`, `goleak`, `testify`) downloads **stock upstream Postgres server binaries** from a Maven-hosted binary repository (`zonkyio/embedded-postgres-binaries`) at runtime and runs them as a subprocess. There is no mechanism to inject a compiled `age.so` extension into those binaries. It can test the plain Postgres substrate (events/draws/pile tables) but **cannot** test anything that runs `CREATE EXTENSION age` or `cypher()`. It also requires network egress on first use (to fetch the binary) — a real cost given this project has no CI and values hermetic tests.
- **`pg_tmp`** has the same fundamental problem (it wraps a system-installed or downloaded stock Postgres) and is a shell-script tool with sparse recent maintenance activity — not a compelling addition even setting the AGE problem aside.
- **`testcontainers-go/modules/postgres`** (v0.44.0, 2026-08-07) solves the actual problem because it can point at **any Docker image**, including the official `apache/age` image that ships AGE pre-compiled and pre-installed. This is the only one of the three that can produce a database the project's graph code can actually run against.

Given that, the real question isn't "which of the three tools" — it's "how do we use a Docker-based Postgres+AGE without breaking a codebase that has no CI and enforces hermetic tests by panicking on any path resolved outside a temp directory." The answer: **don't let it into the default test run at all.**

- Keep `go test ./...` exactly as offline and hermetic as it is today: test SQL/Cypher string construction, `agtype` encode/decode, migration ordering, and queue drain/compaction logic against fakes and in-memory fixtures — none of that logic actually needs a live database to be correct.
- Put every test that needs a live Postgres+AGE behind an explicit, opt-in gate — a build tag (e.g. `//go:build pgtest`) or an environment variable check (e.g. skip unless `HUGEL_TEST_PG=1` is set) — so it never runs by accident, and a contributor without Docker installed is never surprised by a hang or a failure.
- Inside that gated suite, use `testcontainers-go` with the `apache/age` image: it is the current Go-native standard for exactly this (container lifecycle bound to the test process, no separate compose file to remember to tear down), and its dependency chain (Docker client libraries, `moby/moby/api`, `opencontainers/image-spec`, `gopsutil`, etc.) is real but **confined to that test package** — it never reaches `go.sum` resolution for the production binary, and a user running `go install github.com/.../hugel` pays nothing for it.
- Use `docker-compose` (matching the `docker-compose.yml` AGE's own repository ships) for the developer's own persistent local dev database — a file a human runs by hand when iterating, not something `go test` touches.

Confidence: HIGH on the AGE-compatibility gap for `embedded-postgres`/`pg_tmp` (verified directly against `embedded-postgres`'s own README, which documents fetching binaries from a Maven repo with no extension-injection mechanism described anywhere). HIGH on `testcontainers-go`'s current version (verified via GitHub Releases API). MEDIUM on the exact `apache/age` Docker Hub tag naming scheme for a specific PG major (verified the image exists and is the official one; did not enumerate every current tag).

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|--------------------------|
| `pgx/v5` direct connection for short-lived CLI | `pgxpool.Pool` everywhere, including one-shot commands | If CLI invocations start doing multiple concurrent DB operations per process (currently they don't — one process, one linear sequence of work) |
| `goose` for migrations | `golang-migrate` | If goose's `Provider` API proves awkward in practice, or the team prefers golang-migrate's larger community/more third-party tooling. Cost is nearly identical either way — this is the closest call in this research. |
| Hand-rolled JSONL queue | `go.etcd.io/bbolt`-backed queue | If the queue needs keyed dedupe or out-of-order acknowledgement rather than strict FIFO drain |
| `testcontainers-go` for gated integration tests | `docker-compose` driving a fixed local Postgres+AGE, tests connect to it directly | If the team wants one long-lived local dev+test database instead of ephemeral per-test containers — trades test isolation for faster iteration; reasonable for a single-gardener project, worth revisiting if the "shared, several machines" topology means CI-like automation shows up later |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `apache/age/drivers/golang` (the official AGE Go driver) | Stale (`go 1.19` module floor, `testify v1.7.0`), depends on `lib/pq` (a **second** Postgres driver alongside pgx) and an ANTLR4 parser runtime just to decode `agtype` | Plain `pgx` + `SELECT * FROM cypher(...) AS (...)`, decode `agtype` by hand |
| `lib/pq` anywhere in the module graph | A second Postgres driver next to pgx buys nothing and doubles the wire-protocol surface to reason about; it is in maintenance mode, not active development | `pgx/v5` (and `pgx/v5/stdlib` for anything, like goose, that wants `database/sql`) |
| `jackc/tern` for migrations | CLI-first tool; pulls in a templating engine (`Masterminds/sprig`) and a CLI framework (`spf13/cobra`) that have no use once embedded, for no rollback/embedding advantage over goose | `goose` (or `golang-migrate`) |
| `ariga/atlas` for migrations | Declarative schema-diffing needs a disposable "dev database" (Docker) to compute against — a heavier operational model, and a second Docker dependency, than linear hand-written migrations need | `goose` |
| `fergusstrange/embedded-postgres` or `pg_tmp` for any AGE-touching test | Neither can load a custom Postgres extension — they only run stock upstream binaries | `testcontainers-go` + the `apache/age` Docker image, behind an opt-in build tag |
| `dque` / `goque` / `nutsdb` / `boltqueue` for the local write queue (today) | None is dominant or exceptionally well-maintained; every one still leaves hugel to build the actual outbox semantics on top, duplicating work the existing fsynced-append primitive already did | Reuse the existing JSONL append primitive; escalate to `bbolt` only if keyed access is later needed |
| Wiring `testcontainers-go` or Docker into the default `go test ./...` | Breaks the project's own hermeticity rule (tests panic on paths outside a temp dir) and its "no CI today" reality — a Docker dependency in the default run is an unannounced external-environment requirement | Gate live-Postgres tests behind a build tag / env var, off by default |
| Any ORM (`gorm`, `ent`, etc.) or config library (`viper`, etc.) | Out of scope per the project's own stated constraints (`no ORM, no config library`) — unrelated to this milestone's actual gap, and this research found no reason those constraints should change | Hand-written SQL/Cypher via `pgx`; existing flag/env handling |

## Stack Patterns by Variant

**If a command is a short-lived, one-shot CLI invocation (the common case — "many times a day"):**
- Use `pgx.Connect` / `pgx.ConnectConfig` directly, no pool.
- Because pool warm-up (min-conns bookkeeping, background goroutines) costs more than it returns for a process that runs one linear sequence of work and exits.

**If a command is the resident `hugel garden` TUI (this milestone's "session-persistent garden" requirement):**
- Use `pgxpool.Pool`, constructed once at startup and passed down explicitly (not a package-level global).
- Because multiple goroutines (render loop, background refresh, tender-progress polling) need to safely share a small number of live connections for the life of the session — exactly pgxpool's design target.

**If a migration needs to run from more than one machine at roughly the same time (this milestone's "shared topology" decision):**
- Rely on goose's Postgres advisory-lock session locker (or golang-migrate's equivalent), don't build your own locking.
- Because the "one garden reachable from several machines" requirement means startup migrations are no longer guaranteed single-writer by construction the way a single-user local CLI was.

**If a test needs to exercise real Cypher/AGE behavior (not just SQL string construction):**
- Gate it behind a build tag or env var, run it against `testcontainers-go` + the `apache/age` image.
- Because AGE cannot be faked with a stock Postgres, and this project's hermeticity rule means "needs Docker" must be opt-in, never silent.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|------------------|-------|
| `pgx/v5` v5.11.0 | Go ≥1.25 (project: 1.26) | `go.mod` floor confirmed directly; no cgo anywhere in the module. |
| `goose/v3` v3.28.0 | Go ≥1.26 (project: 1.26, exact) | Confirmed via `go.mod`; uses `*sql.DB`, so pairs with `pgx/v5/stdlib` cleanly. |
| `golang-migrate/migrate/v4` v4.20.1 | Go ≥1.25.11; `database/pgx/v5` subpackage pairs with `pgx/v5` already in `go.sum` | Avoid the default `database/postgres` driver package, which pulls in `lib/pq` instead — use `database/pgx/v5` explicitly. |
| Apache AGE 1.8.0 | PostgreSQL 18 (its matching branch); **not** PostgreSQL 19 (beta-only AGE branch, no stable release as of 2026-09-11) | Pin the Postgres *server* major version to 18 until AGE cuts a formal PG19 release — this is a server-side constraint, independent of the Go module versions above. |
| `testcontainers-go/modules/postgres` v0.44.0 | Requires a running Docker daemon; pairs with any Postgres-compatible image, including `apache/age` | Confine to a build-tag-gated test package; never a dependency of the production binary. |
| `bbolt` v1.5.0 | Pure Go, no cgo, any Go version this project uses | Not currently needed; recorded as the correct escalation path only. |

## Sources

- https://github.com/jackc/pgx/releases (GitHub Releases API, queried directly) — pgx v5.11.0, published 2026-09-07
- https://raw.githubusercontent.com/jackc/pgx/v5.11.0/go.mod — confirmed `go 1.25.0` floor and full dependency list
- https://raw.githubusercontent.com/jackc/pgx/v5.11.0/README.md — confirmed pure-Go, `pgx.Connect`/`pgxpool.New` usage
- GitHub Discussions `jackc/pgx#1700`, "Best practice for long-running pgxpool" — CLI vs. long-running-app pooling guidance
- https://github.com/apache/age/releases and https://api.github.com/repos/apache/age/tags — queried directly; confirmed PG18/v1.8.0 (published 2026-07-09) is the latest formal release, and the PG19 tag/branch exists but has no published GitHub Release
- https://api.github.com/repos/apache/age/git/refs/tags/PG19%2Fv1.8.0-rc0 and its target commit — confirmed the PG19 tag's commit message explicitly labels it a PostgreSQL-19-beta1 beta release
- https://github.com/apache/age/issues/2323, "Multiple standard features not supported" — AGE's own tracked openCypher compliance gaps
- Apache AGE 1.8.0 and 1.7.0 GitHub Release notes (fetched via API `body` field) — confirmed `shortest_path`/`all_shortest_paths`, `MERGE ON CREATE/ON MATCH SET`, predicate functions as recent (1.7.0/1.8.0) additions
- https://age.apache.org/getstarted/quickstart/ — confirmed `CREATE EXTENSION age; LOAD 'age'; SET search_path = ...` activation sequence (note: this page's stated PG-version-support list lags the actual release branches — trust the Releases/tags API over the prose docs for version support)
- https://raw.githubusercontent.com/apache/age/master/drivers/golang/go.mod — confirmed the official Go driver's stale `go 1.19` floor and `lib/pq`/ANTLR4 dependencies
- https://www.commandprompt.com/blog/two-features-just-left-postgresql-19/ and https://www.postgresql.org/docs/19/release-19.html — confirmed SQL/PGQ (`CREATE PROPERTY GRAPH`, `GRAPH_TABLE`) was reverted from PostgreSQL 19 on 2026-09-07, 47 commits removed, earliest possible return PostgreSQL 20 (~September 2027) — corroborates PROJECT.md's existing decision log entry
- https://www.postgresql.org/support/versioning/ — confirmed PostgreSQL 18 (released 2025-09-25) is the current stable major as of this research date, with PostgreSQL 19 still in beta
- https://api.github.com/repos/pressly/goose/releases and raw `go.mod`/`dialect.go`/`provider.go`/`lock/postgres.go` source — confirmed v3.28.0 (2026-09-02), `go 1.26.0` floor, `*sql.DB`-based `Provider` API, and the Postgres advisory-lock session locker
- https://api.github.com/repos/golang-migrate/migrate/releases and raw `database/postgres/postgres.go` / `database/pgx/v5/pgx.go` source — confirmed v4.20.1 (2026-09-09), the default driver's `lib/pq` dependency, and the `pgx/v5` subpackage's clean reuse of the existing pgx driver
- https://api.github.com/repos/jackc/tern/releases and raw `go.mod` — confirmed v2.4.3 (2026-08-23) and its `sprig`/`cobra` dependencies
- https://api.github.com/repos/ariga/atlas/releases — confirmed v1.3.0 (2026-08-02)
- https://api.github.com/repos/etcd-io/bbolt/releases — confirmed v1.5.0 (2026-06-21), pure Go
- https://api.github.com/repos/testcontainers/testcontainers-go/releases and raw `go.mod` files for the root module and `modules/postgres` — confirmed v0.44.0 (2026-08-07) and its Docker-client-heavy but test-package-confined dependency graph
- https://github.com/fergusstrange/embedded-postgres README (raw) — confirmed it fetches stock upstream Postgres binaries from a Maven repository at runtime, with no described mechanism for injecting a custom extension such as AGE
- Web search on Go persistent/disk-backed queue libraries (`dque`, `goque`, `nutsdb`, `boltqueue`, `bigqueue`) — confirmed no single dominant, actively-maintained option exists for this specific niche

---
*Stack research for: Postgres + Apache AGE substrate migration (hugel v0.2, "The Shared Garden")*
*Researched: 2026-09-11*
