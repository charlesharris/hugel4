# Project Research Summary

**Project:** hugel v0.2 "The Shared Garden"
**Domain:** Single-binary Go CLI migrating its substrate (event log, draw log, knowledge pile) from file-backed storage under one home directory into shared PostgreSQL + Apache AGE (property graph), behind a durable local write queue
**Researched:** 2026-09-11
**Confidence:** MEDIUM-HIGH overall (stack facts HIGH/API-verified; architecture HIGH on codebase, MEDIUM on Postgres/AGE patterns; features MEDIUM-HIGH; pitfalls MEDIUM, with two explicitly flagged claims needing re-verification)

## Executive Summary

This is a low-volume (~50 events/day, one gardener, occasionally two machines), no-CI, single-binary CLI absorbing a shared-database dependency it did not have before. All four research passes converge on the same shape: keep everything as simple as the volume allows, and spend the actual engineering effort on the two guarantees the milestone is named for — writes never blocking on the network, and a backup that has actually been restored. The stack is settled and low-risk (`pgx/v5`, `goose` for migrations, Apache AGE as a server-side extension, no new Go dependency for the write queue — reuse phase 01's fsynced-append primitive). The architecture is settled too: a new `internal/store` package centralizes the pool/migrations/AGE session state that the three domain packages (`events`, `draws`, `pile`) now share, a new `internal/queue` package sits beside it as a local durable outbox, and `internal/pile` is deliberately sequenced last and given its own phase because it is the one place git currently supplies both durability and a versioning/audit mechanism that Postgres does not automatically replace.

The single largest risk this research surfaces is not technical difficulty but complacency: this project has already learned once, at cost, that "a guarantee nobody exercises is not a guarantee" (phase 01's health-check saga). Every new durability claim this milestone introduces — the queue drains, the backup restores, the health check is honest — is exactly as capable of being quietly false as the original event-health check was, and PITFALLS names this as a standing practice (a "prove it can go red" test for every new health signal) rather than a one-time fix. The queue's silent-drain-stall failure mode and the restore drill's silent-non-restore failure mode are literally the same shape of mistake, recurring at two different layers of the same milestone.

Two decisions the research could not make on its own, and should be made by a human before the roadmap locks phase boundaries, are surfaced prominently below. Everything else — build order, anti-features to refuse, and what must be deliberately preserved from the file-backed system rather than lost by default — is reconciled into a single account across all four documents.

## Key Findings

### Recommended Stack

(STACK.md — Confidence: HIGH, all versions API-verified against GitHub Releases/tags/go.mod on 2026-09-11, not recalled from training data.)

**Core technologies:**
- `github.com/jackc/pgx/v5` (v5.11.0) — pure-Go Postgres driver, already the project's committed choice; confirmed current. Use raw `pgx.Connect` for short-lived CLI invocations (no pool), `pgxpool.Pool` only inside the resident `hugel garden` TUI process once it becomes session-persistent.
- Apache AGE (1.8.0, branch `release/PG18`) — a **server-side Postgres extension, not a Go dependency.** Adds zero lines to `go.sum`. Call `cypher()` through plain `pgx`; do not add the official `apache/age` Go driver (stale, `go 1.19` floor, pulls in `lib/pq` as a second driver — explicitly avoid).
- `github.com/pressly/goose/v3` (v3.28.0, requires Go 1.26.0 exactly) — migrations, embeds via `go:embed`, runs on `*sql.DB` via `pgx/v5/stdlib` (no second driver), ships a Postgres advisory-lock session locker directly relevant to "several machines" both trying to run startup migrations. `golang-migrate/v4` is a legitimate, near-equal fallback if goose's `Provider` API proves awkward in practice — this is the closest call in the stack research and switching later should not be treated as wasted work.
- Local write queue — **hand-rolled, zero new dependencies.** No dominant Go library exists for disk-backed outbox semantics (dque/goque/nutsdb/boltqueue all leave hugel to build the actual semantics anyway); reuse the fsynced JSONL append primitive phase 01 already built and tested. Escalate to `go.etcd.io/bbolt` only if keyed dedupe/out-of-order ack is ever needed — not required today.
- Local Postgres+AGE for dev/test — no automatic spin-up in the default hermetic `go test ./...`. `embedded-postgres`/`pg_tmp` **cannot** load AGE (they only fetch stock upstream binaries); use `testcontainers-go` + the official `apache/age` Docker image, confined to an opt-in build-tag-gated (`pgtest`) test package that never reaches the production binary's dependency graph.

**Caveat carried forward at HIGH confidence but time-sensitive:** AGE branches per Postgres major version. PG19 exists only as a beta-tagged branch with no formal release — **pin the Postgres server to 18** until AGE cuts a stable PG19 release; do not follow Postgres to 19 once it goes GA.

**Caveat carried forward at MEDIUM confidence:** AGE's openCypher coverage (what's supported vs. missing — e.g., no `datetime()`, restricted clause combination) is compiled from AGE's own release notes and its own tracked compliance issue (`apache/age#2323`), which is about as authoritative as third-party research gets for pre-1.0 extension software, but is a snapshot and may have moved on. Re-verify against the pinned AGE version at implementation time, not at roadmap time.

### Expected Features

(FEATURES.md — Confidence: MEDIUM-HIGH, patterns are established industry practice, sizing is opinionated and calibrated specifically to ~50 events/day.)

**Must have (table stakes):**
- Monotonic append-only sequence id + immutability enforcement (trigger/permission revoke) on the event table — converts a JSONL append-only *habit* into a store-enforced *guarantee*
- Idempotency key (client-generated at emit time) + actor/machine id on every row — the single most load-bearing piece of new surface the existing `Emit` call sites must grow; serves both queue-retry dedup and general event-log integrity, design it once
- Local durable write queue extending phase 01's fsync/fail-loudly discipline, draining opportunistically, at-least-once + idempotent apply (not exactly-once — that's a known, accepted trade-off, not a gap)
- Per-source (not global) ordering: preserve local FIFO per actor/machine; do not attempt to guarantee cross-machine order
- Queue depth/health folded into the *existing* `yield --health` surface, not a new dashboard
- Optimistic version check (`updated_at`/version column) on the one mutable store (the pile) — the only real cross-machine concurrency risk, because Postgres is now the single arbiter of order and there is no independent-replica merge problem to solve
- Restore drill that actually runs, on an enforced cadence

**Should have (differentiators, add after validation):**
- Structure-aware soil ranking consulting the graph (not just lexical match) — the actual payoff of building AGE at all, but blocked on substrate widening producing non-trivial edges first
- Cross-entity multi-hop path queries, once a concrete soil use case names the specific traversal needed

**Explicitly refuse (anti-features — do not create a phase for these):**
- Table partitioning, retention/deletion policy, snapshotting on the event table (18K rows/year never approaches a size where any of this matters, and hugel's value is keeping everything)
- Schema versioning/event envelope frameworks, optimistic-concurrency version numbers on the event log (one codebase, one deploy, no aggregate contention)
- Exactly-once delivery via distributed transactions (at-least-once + idempotency key is observably identical and vastly simpler)
- A message-broker-style queue (Kafka/NATS/etc.), priority/reordering queues, a separate queue-status UI
- Vector clocks / CRDTs for cross-machine conflict resolution, multi-master/leader-election topology, machine-specific merge UI (all solve for *independent replicas reconciling later* — this architecture centralizes on one shared Postgres specifically to avoid having independent copies to reconcile)
- Migrating to a dedicated graph database, human-asserted graph edges, LLM edge inference at extraction time (the latter two are measured failures already on record in this project's own pile), a graph visualization/explorer UI

### Architecture Approach

(ARCHITECTURE.md — Confidence: HIGH on current-codebase facts, MEDIUM on Postgres/AGE/testcontainers pattern claims.)

A new `internal/store` package (parallel in role to the existing `internal/config`) owns the pool, migration runner, transaction helper, and AGE per-connection session setup (`LOAD 'age'`, `search_path`, via `pgxpool`'s `AfterConnect`) — the one thing events/draws/pile now share that none of them individually owned before. `internal/events`, `internal/draws`, `internal/pile` are MODIFIED, not replaced: every existing call-site API (`events.Emit`, `draws.Append`, `pile.Store.Put`) stays untouched. A new `internal/queue` package sits *beside* `store`, not wrapping or nested inside it — domain packages write to `queue` on the hot path (never to `store` directly), `queue.Drain` is the only path into `store`. Reads overlay `store` with undrained `queue` entries for read-your-own-writes on one machine.

**Major components:**
1. `internal/store` — pool, migrations, AGE session plumbing, `Tx` helper. Built first; nothing downstream can be built against a moving target.
2. `internal/queue` — local durable outbox, generalized directly from phase 01's `internal/events` fsync/fail-loudly/health mechanism. Buildable largely in parallel with `store` once the "op" shape is agreed; `Drain()` is the one piece strictly sequential after `store` exists.
3. Testing infrastructure (schema-per-test-run harness + testcontainers + a DSN guard mirroring `config.Sandbox()`'s "refuse by construction" property) — must land as effectively "step 0," not follow-up work, because per-test transaction rollback silently stops isolating once code acquires a second pooled connection (which AGE's per-connection session state will do routinely).
4. `internal/events` (converted first — smallest surface, best-already-tested), then `internal/draws` (converted second — no dependency on events, can parallelize), then `internal/pile` (converted **last, as its own phase** — git today supplies both write mechanism and versioning/audit history, and that decision has not been made yet; see Decision 1 below).
5. `hugel migrate import` — one-shot (not dual-write, not read-through) importer, justified by data volume (13KB events, 22 draws, a few MB of pile fitting comfortably in one pass). Can be built in parallel with domain-package conversions, gated only on `store` existing.

**Smallest genuinely useful slice:** `store` + `queue` + `internal/events` converted end-to-end, `yield --health` reporting through the new path, `migrate import` handling just `events.jsonl`, and the schema-per-test harness in place — ships the entire substrate mechanism proven on the lowest-risk domain before either `draws` or `pile`'s harder decisions are in scope.

### Critical Pitfalls

(PITFALLS.md — Confidence: MEDIUM overall; individual pitfalls corroborated across sources and against this project's own code, but see the two flagged caveats below.)

1. **The drain loop dies and nothing says so** — background drain has no natural caller waiting on its result, unlike `Emit`'s synchronous error return. Avoid by treating "is the queue draining" as a first-class health fact (`LastDrainSuccess` / `DrainFailingSince`), following the exact pattern already in `internal/events/events.go`. Ship in the same phase as the queue, not deferred.
2. **Retry after partial local ack produces duplicate rows** — a crash between remote-commit and local-mark-sent is the modal failure of this design, not an edge case. Idempotency key assigned at *enqueue* time (not send time) + `ON CONFLICT DO NOTHING` on insert makes duplicates inert. Must be in the wire format from the first write — retrofitting onto already-written rows is a second migration.
3. **A backup that has run error-free for months does not restore** — a backup job's own success signal (exit 0, plausible file size) says nothing about restorability. Verify the *restore*, not the backup: automated restore into a disposable instance with row-count/content/live-Cypher-query checks, on an enforced cadence tied to an event the gardener can't avoid noticing (not a calendar reminder).
4. **The restore drill itself decays** — built once, run once during its own phase, never scheduled again. Pin it to something unavoidable (a `yield --health` field for "restore last verified," aging out after N days) rather than trusting anyone to remember.
5. **A health check reports the wrong thing with total confidence** — named explicitly as a recurrence risk of the exact mistake phase 01 already made four verification passes to catch. Every new health signal this milestone adds (queue-drain, restore-freshness, Postgres reachability) needs its own "break the mechanism, confirm the check turns red" test, not just a happy-path test. This is named as a standing practice for every phase that adds a health signal, not a one-time fix.

**Caveat carried forward at MEDIUM confidence (partly vendor-doc-derived, not the AGE project's own docs):** `pg_upgrade` does not support databases with AGE installed (OID-referencing internal types are not stable across major-version upgrades). Treat every Postgres major-version upgrade as dump-and-restore, never `pg_upgrade`, until this is re-verified directly against `github.com/apache/age`'s own documentation at the point AGE is actually pinned.

## Decisions the Research Could Not Make — Flag for Human Resolution

These are named independently by multiple research passes and materially change phase scope and schema design. They should be resolved before phase boundaries lock, not discovered mid-implementation.

**1. Does the pile's *content* move into Postgres, or does Postgres hold only metadata/edges over git-backed markdown?**
Raised independently by ARCHITECTURE (§3, "forces a decision this milestone hasn't made yet — whether Postgres rows get their own history/audit mechanism or whether git versioning is deliberately kept in parallel") and PITFALLS (Pitfall 17, "the pile's git-diffable review history stops being reviewable the moment it's a Postgres table"). PROJECT.md is explicit that bd stays authoritative for beads and Postgres carries relations, but is silent on this specific question for the pile. If content moves fully into Postgres, an append-only audit table (entry id, changed field, old/new value, actor, timestamp) must exist **from day one** — bolting provenance on later means the history before that point is unrecoverable, which is exactly the class of loss this milestone exists to prevent. This decision gates: `internal/pile`'s conversion phase scope, the migration importer's design for pile entries, and whether Pitfall 17 needs its own mitigation work at all. **Recommend resolving before scoping the `internal/pile` conversion phase**, since it is already sequenced last and hardest — the extra lead time can be spent on this decision rather than lost to it mid-phase.

**2. What does "several machines" mean for cross-machine event ordering — wall-clock timestamps (with clock-skew risk), or something else?**
Raised independently by FEATURES (§3, "timestamps used for display/ordering, not for correctness... never rely on `occurred_at` comparison for anything that must be correct") and PITFALLS (Pitfall 9, "import order is inferred from file position, and that assumption breaks the moment there is more than one source file"). The research's own working answer — a monotonic *sequence id* is the ordering authority, timestamps are for display only — resolves *future* writes once events flow through a single Postgres sequence. It does **not** resolve the one-time migration problem: the existing `events.jsonl` history across machines has no sequence number at all, only wall-clock timestamps, so the importer must merge multiple source files by some rule and that rule's failure mode (clock skew) needs to be written down and accepted explicitly rather than silently assumed via file-concatenation order. **Recommend deciding and recording this before the migration/import phase is planned**, and separately confirming that newly-emitted events (post substrate-widening) carry a per-machine monotonic sequence number going forward so this stops being unanswerable for all future history.

## What Must Be Deliberately Preserved, Not Regretted Later

PITFALLS identifies concrete properties of the current file-backed system that do not survive the migration by default and need a shipped replacement in the same phase that removes the file-backed version, not as follow-up work:

- **`grep`/`jq` debuggability** — "what happened Tuesday" today needs no tool beyond what's on the machine, works offline, works when Postgres itself is what's broken. Ship a `hugel events tail`/`hugel events grep` command in the same phase that moves events into Postgres, including visibility into whatever is still sitting undrained in the local queue.
- **"One bad line costs one event, never the history"** — `events.Load()`'s resilience property does not survive a naive bulk `INSERT`/one-transaction import or drain, where one malformed row aborts the whole batch (or a swallowed error silently drops it all). Insert row-by-row or with per-row `SAVEPOINT` isolation, in both the one-shot importer and the ongoing drain path, and log every skipped row with enough content to reconstruct it.
- **Git-diffable pile history** — see Decision 1 above; this is the concrete mechanism at risk, not an abstract concern.
- **Working with no network** — already a first-class constraint (writes queue locally), but extend the same posture to reads: any code reading "current state" from Postgres must account for writes queued locally but not yet drained, or reads on one machine can appear to regress relative to writes just made on that same machine.

## Implications for Roadmap

Reconciling ARCHITECTURE's build order, FEATURES' sequencing, and PITFALLS' pitfall-to-phase mapping into one account. All three documents agree on the top-level shape: **testing infrastructure and the storage seam come first, `internal/events` is the smallest safe first vertical slice, the graph is strictly gated on substrate widening producing non-empty data, and `internal/pile` and backup/restore are each large and hard enough to deserve their own phase rather than being folded into whatever else is happening.**

### Phase 1: Storage seam + test harness (`internal/store`, schema-per-test harness, DSN guard)
**Rationale:** Nothing downstream — queue, domain-package conversion, migration importer — can be built against a moving target, and building Postgres-touching code before a `config.Sandbox()`-equivalent exists means either untested code or code tested unsafely against a real shared instance (Pitfall 19). This is explicitly named as needing to land *before or bundled with* the first Postgres-writing phase, not retrofitted after.
**Delivers:** pool, `AfterConnect` AGE session plumbing (even though no graph queries exist yet), migration runner (goose), `Tx` helper, schema-per-test-run harness backed by testcontainers + `apache/age` image, a Postgres-DSN sandbox guard.
**Avoids:** Pitfall 19 (untestable/unsafely-tested Postgres code).

### Phase 2: Local write queue (`internal/queue`)
**Rationale:** Can start largely in parallel with Phase 1 once the "op" shape is agreed, but its `Drain()` implementation and any conversion of a domain package must wait on `store` existing. Must exist *before* any domain package is converted — writing straight to the database first and retrofitting a queue later means rebuilding the write path twice, and leaves every gate/tender run network-dependent in the meantime (an explicit anti-goal).
**Delivers:** fsynced local append (direct generalization of phase 01's `internal/events` mechanism), idempotency key assigned at enqueue time, strictly single-threaded FIFO drain, opportunistic-drain-on-next-invocation + an explicit drain command, queue depth/drain-health folded into `yield --health`.
**Avoids:** Pitfalls 1 (silent drain stall), 2 (duplicate rows on retry), 3 (reordering from parallel drain), 4 (unbounded queue growth), 5 (queue's own torn-write corruption), 16 (bulk-insert losing many rows for one bad row), 18 (health check that can't go red — needs a "prove it can go red" test per new health field).

### Phase 3: Convert `internal/events` end-to-end
**Rationale:** Smallest surface, fewest call sites relative to importance, already the most rigorously tested domain in the codebase (phase 01). This is the smallest genuinely useful slice — it proves the entire substrate mechanism (pool, AGE plumbing, migrations, queue, drain, read-your-own-writes overlay) on the lowest-risk domain before either draws' four read call sites or the pile's unresolved git-vs-Postgres question are in scope.
**Delivers:** events table (monotonic id, actor_id, idempotency_key unique, immutability enforced via trigger/permission), `yield --health` reporting through the new path, `hugel events tail`/`grep` (preserving grep-debuggability, Pitfall 15), `hugel migrate import` handling `events.jsonl` (with reconciliation-gated success, Pitfall 11, and a documented cross-machine merge-order decision, Pitfall 9 / Decision 2 above).
**Addresses:** FEATURES §1 table stakes (monotonic id, immutability, idempotency key, actor id, basic indexing).

### Phase 4: Convert `internal/draws`
**Rationale:** No dependency on events' conversion — can run in parallel with Phase 3 as a separate workstream if desired. Flat record shape, no git entanglement, genuinely simpler than either events or pile.
**Delivers:** draws table, migration import for `draws.jsonl`, same reconciliation/timestamp-fidelity discipline as Phase 3 (Pitfalls 8, 11).

### Phase 5: Convert `internal/pile` — its own phase, gated on Decision 1
**Rationale:** Explicitly the hardest of the three domain conversions and should not be bundled with either of the others. Git today supplies both the write mechanism and the pile's versioning/history — moving to Postgres forces the content-vs-metadata decision (Decision 1) that the roadmap should resolve before this phase is scoped, not during it. This phase also has the highest migration risk of the three: converging-write keys designed for single-store serialization can collide once multiple machines' independent pile histories are imported into one table (Pitfall 10), requiring an explicit pre-import collision report before any `ON CONFLICT` logic runs.
**Delivers:** pile table(s) with optimistic version check (`updated_at`/version) for cross-machine write safety, an audit/history table if content moves into Postgres (per Decision 1), migration import with a collision report as a hard pre-condition.
**Avoids:** Pitfall 10 (convergence-key collision across machines), Pitfall 17 (loss of git-diffable review history) — contingent on Decision 1 being resolved.

### Phase 6: Backup, restore, and the restore drill
**Rationale:** PITFALLS explicitly recommends sequencing this *before or alongside* the AGE graph work, not after, since AGE data has its own restore hazards (Pitfalls 12, 13) that the drill needs to already be exercising by the time the graph phase lands. This is also the phase that directly answers the milestone's central constraint ("a backup that has never been restored is not a backup") and should not be treated as optional polish.
**Delivers:** automated backup, an automated restore into a disposable instance with row-count/content/live-query fidelity checks (not just exit-code success), a `yield --health` field for "restore last verified" with an enforced maximum age, an explicit decision on whether the AGE graph specifically is restored via `pg_dump` or rebuilt via drop-and-replay from base tables (recommended: the latter, since the graph is already a droppable/rebuildable projection by design), an upgrade-tooling check that refuses `pg_upgrade` when AGE is present.
**Avoids:** Pitfalls 6, 7 (backup that doesn't restore; drill that decays), 12, 13 (`pg_upgrade`/AGE incompatibility caveat — MEDIUM confidence, re-verify before implementing; dump/restore leaving the graph unqueryable).

### Phase 7: Apache AGE graph, wired but gated on substrate widening
**Rationale:** All three documents agree this cannot start meaningfully until the Active substrate-widening roadmap items (widen event emission, stop discarding bd's dependency graph, enrich bead structural metadata) have landed and run long enough to produce non-trivial data — building the graph today would produce, in PROJECT.md's own words, "a graph derived from this substrate today would be very nearly empty." This phase is graph-*infrastructure* (extension installed, session plumbing wired, basic neighbor/path queries proven), not yet the differentiator (structure-aware soil ranking) — that is explicitly a later, separately-validated phase per FEATURES' "Add After Validation" tier.
**Delivers:** AGE extension installed and pinned to PG18 (MEDIUM-confidence caveat on PG19 timing — re-check at implementation time), basic Cypher neighbor/variable-depth queries for entities that already carry relationships (code↔code via cochange, ticket↔ticket via bd's preserved dependency graph), a synthetic-scale benchmark run *before* any query ships into the draw path (Pitfall 14 — untested Cypher against an empty graph hides a performance cliff that appears once edges exist).
**Avoids:** Pitfall 14 (performance cliff on populated graph), and structurally avoids the anti-features FEATURES names (no dedicated graph DB, no human-asserted or LLM-inferred edges, no visualization UI).

### Phase Ordering Rationale

- **Dependency chain, strict:** Phase 1 (store) → Phase 2 (queue) → Phases 3–4 (events, draws — parallelizable against each other) → Phase 5 (pile, gated on Decision 1) → Phase 6 (backup/restore, informed by whatever AGE-specific restore hazards Phase 7 will introduce, hence sequenced before/alongside it) → Phase 7 (AGE graph, gated on substrate-widening items that are outside this migration's own critical path but are Active elsewhere in PROJECT.md).
- **Parallelizable within the above:** Phase 1 and Phase 2's non-drain work; the test harness alongside Phase 1's schema; the migration importer's development against Phases 3–5's conversions (it only needs `store` to exist, not the domain packages converted); Phase 3 vs. Phase 4 against each other.
- **This ordering avoids the single costliest anti-pattern named across all four documents:** writing straight to the database on the hot path with the queue "added later," and building the graph before the substrate is dense enough for it to mean anything.

### Research Flags

Phases likely needing deeper research during planning (`--research-phase`):
- **Phase 5 (`internal/pile` conversion):** blocked on Decision 1 (content-vs-metadata) with no default answer from this research pass; needs its own discussion/spike before planning can proceed, not just deeper technical research.
- **Phase 6 (backup/restore):** the `pg_upgrade`/AGE incompatibility claim is MEDIUM confidence and partly vendor-doc-derived — verify directly against `github.com/apache/age`'s own docs before finalizing the upgrade-refusal check.
- **Phase 7 (AGE graph):** AGE's openCypher coverage gaps (MEDIUM confidence, a dated snapshot of `apache/age#2323`) should be re-checked against the actually-pinned AGE version before committing to specific query patterns in the draw path.

Phases with standard, well-documented patterns (safe to skip `--research-phase`):
- **Phase 1 (storage seam):** pgx/goose/testcontainers patterns are HIGH confidence, API-verified, and directly sourced from official docs and source reads.
- **Phase 2 (write queue):** direct generalization of phase 01's already-built-and-tested mechanism; the "no dominant library" finding means the design is hand-rolled from known-good prior art, not novel research.
- **Phases 3–4 (events, draws conversion):** mechanical application of the Phase 1/2 seam to two well-understood, already-simple domains.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All versions verified directly against GitHub Releases/tags API and raw `go.mod` files on 2026-09-11 — not recalled from training data. The one MEDIUM sub-finding (AGE's openCypher coverage) is flagged inline above. |
| Features | MEDIUM-HIGH | Patterns (event sourcing, outbox, multi-device sync, property graphs) are well-established industry practice; sizing/prioritization is this research's own opinionated calibration to hugel's stated volume, cross-checked against PROJECT.md's own prior recorded refusals (zero-uptake human-asserted edges, failed LLM edge inference). |
| Architecture | MEDIUM-HIGH | Codebase facts (current package structure, call sites, `config.Sandbox` mechanics) are HIGH — read directly from source. Postgres/pgx/AGE/testcontainers integration patterns are MEDIUM — corroborated by current community docs and one pgx maintainer discussion, not yet proven against this codebase in practice. |
| Pitfalls | MEDIUM | Queue/outbox/backup pitfalls are well-corroborated general operational knowledge cross-checked against this project's own phase 01 code (higher effective confidence than the individual LOW-confidence web sources would suggest alone). AGE-specific pitfalls (`pg_upgrade` incompatibility, restore-ordering hazards) are MEDIUM and partly vendor-doc-derived (Postgres Pro, Microsoft Learn) rather than the AGE project's own documentation — explicitly flagged for re-verification at the point AGE is pinned. |

**Overall confidence:** MEDIUM-HIGH — the stack and near-term architecture are solid enough to build against directly; the two flagged AGE-specific claims and the two human decisions above are the load-bearing uncertainties.

### Gaps to Address

- **Pile content location (Decision 1 above):** no default answer — resolve via discussion before scoping Phase 5, not during it.
- **Cross-machine event ordering for the one-time migration (Decision 2 above):** the going-forward answer (sequence id) is settled; the backward-looking migration-merge answer is not — write down the accepted clock-skew failure mode explicitly before Phase 3's importer is built.
- **`pg_upgrade`/AGE incompatibility:** re-verify against `apache/age`'s own docs before Phase 6 finalizes its upgrade-refusal tooling.
- **AGE openCypher coverage gaps:** re-check against the actually-pinned AGE version before Phase 7 commits to specific Cypher query shapes for the draw path.
- **goose vs. golang-migrate:** not a gap so much as a deliberately reversible choice — if goose's `Provider` API proves awkward during Phase 1, switching to golang-migrate costs almost nothing extra and should not be treated as replanning waste.

## Sources

### Primary (HIGH confidence)
- `github.com/jackc/pgx` GitHub Releases/tags API and raw `go.mod` — pgx v5.11.0, 2026-09-07
- `github.com/apache/age` GitHub Releases/tags API and raw commit data — AGE 1.8.0/PG18, 2026-07-09; PG19 beta-only tag with no formal release
- `github.com/pressly/goose` GitHub Releases API and raw source (`dialect.go`, `provider.go`, `lock/postgres.go`) — goose v3.28.0, 2026-09-02
- `github.com/golang-migrate/migrate` GitHub Releases API and raw source — v4.20.1, 2026-09-09
- Direct reads of `internal/events/events.go`, `internal/config/sandbox.go`, `internal/draws/draws.go`, `internal/pile/store.go`, `internal/gate/run.go`, `internal/tender/*.go`, `go.mod` (this repository)
- `.planning/PROJECT.md` (Current Milestone, Constraints, Key Decisions, Context)

### Secondary (MEDIUM confidence)
- Apache AGE 1.8.0/1.7.0 release notes and `apache/age#2323` (openCypher compliance gaps)
- pgx maintainer discussion `jackc/pgx#1700` (pooling guidance for CLI vs. long-running processes)
- Postgres Pro Enterprise docs, Microsoft Learn Azure Database for PostgreSQL AGE overview (pg_upgrade/AGE incompatibility — needs re-verification against AGE's own docs)
- Outbox-pattern and idempotent-consumer writeups (`github.com/nikolayk812/pgx-outbox`, DEV Community, freeCodeCamp) — corroborated at-least-once delivery as the accepted trade-off
- Postgres backup/restore and `pg_stat_archiver` failure-mode writeups (OneUptime, DEV Community)
- Testcontainers-based hermetic database testing writeups (multiple DEV Community posts)

### Tertiary (LOW confidence, cross-checked against each other and project code)
- General web search on Go disk-backed queue libraries (dque, goque, nutsdb, boltqueue) — no dominant option found
- JSONL/timestamp import pitfall case studies (a published PostgreSQL-migration timestamp-corruption case study)
- Distributed clocks / CRDT sync-conflict writeups — used to confirm this architecture's centralized-Postgres model avoids the problem class these solve, not as direct implementation guidance

---
*Research completed: 2026-09-11*
*Ready for roadmap: yes, contingent on the two flagged decisions (pile content location; cross-machine migration ordering) being resolved before Phase 5 and Phase 3's importer are scoped, respectively*
