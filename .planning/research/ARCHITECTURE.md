# Architecture Research: Postgres-Backed Substrate for hugel v0.2

**Domain:** Moving a file-backed Go CLI (events.jsonl, draws.jsonl, git-backed pile) onto a shared Postgres + Apache AGE substrate, behind a durable local write queue.
**Researched:** 2026-09-11
**Confidence:** MEDIUM-HIGH (codebase facts are HIGH — read directly from source; Postgres/pgx/AGE/testcontainers patterns are MEDIUM — corroborated by current docs/community sources, not yet proven against this codebase)

## Answer Summary

- **Storage seam:** a NEW `internal/store` package owns the pool, migrations, transactions and AGE session setup. `internal/events`, `internal/draws`, `internal/pile` are MODIFIED to keep their exact call-site APIs but delegate the write/read mechanism to `store`.
- **Write queue:** a NEW `internal/queue` package, sitting *beside* `store` (not wrapping it, not inside it). Domain packages write to `queue`, not to `store`, on the hot path. Reads overlay `store` with undrained `queue` entries for read-your-own-writes on the same machine. Drain is opportunistic-on-next-invocation by default, with an explicit drain command for the offline case; no background goroutine in the one-shot CLI.
- **Migration:** one-shot import command, not dual-write, given the data is tiny (13KB events, 22 draws, a few MB of pile). Rollback path is the queue's own durable log — the same replay discipline v0.2 already commits to for the AGE graph, one layer down.
- **Testing:** `config.Sandbox`'s panic-on-real-path guarantee is replaced by a schema-per-test-run guard (not per-test transaction — pgxpool + AGE's `AfterConnect` session state means pooled connections don't compose with the rollback trick), backed by testcontainers for CI. Most packages above the storage seam stay hermetic; only `internal/store`, `internal/events`, `internal/draws`, `internal/pile` genuinely need a database in their own tests.
- **Build order:** `internal/store` first, `internal/queue` in parallel against a stubbed interface, then convert `internal/events` end-to-end (smallest, safest, already best-tested) as the first genuinely useful slice, before `internal/draws`, before `internal/pile` (hardest, because it currently uses git as its history mechanism).

---

## 1. Where the Storage Seam Belongs

**Verdict:** NEW package `internal/store`. `internal/events`, `internal/draws`, `internal/pile` are MODIFIED, not replaced.

Phase 01 put the write mechanism (fsync, failure marker, health) *inside* `internal/events`, "beside the file convention it protects," and that was correct for a reason that does not carry over: at that point `internal/events` was the only package that owned a durable-write mechanism, and no other package shared its file, its lock, or its failure semantics. `internal/draws` had its own separate, much simpler append-only file; `internal/pile` had git. Three packages, three unrelated mechanisms, each private to its owner — nothing to centralize.

Postgres removes that isolation. Once events, draws and the pile all live in the same server, they share one thing that none of them individually owns: a connection pool, a migration set, a transaction boundary, and (for the graph layer) a per-connection AGE session setup (`LOAD 'age'`, `SET search_path`) that has to run identically on every connection regardless of which domain acquired it. If each package opened its own pool and ran its own migrations, you get three copies of pool configuration and three uncoordinated migration histories against one schema — and a real risk that `pile.Put` and `events.Emit` silently share a transaction they shouldn't, or fail to, because nothing arbitrates the boundary.

The precedent inside this codebase for "when a shared mechanism graduates to its own package" is `internal/config`: `config.Home()` / `config.Sandbox()` is the one seam every file-touching package calls through, precisely because the garden directory is a resource all of them share. Postgres is the same shape of resource, at a layer below the filesystem. `internal/store` is `internal/config` for the database: it owns *how you reach the resource* (pool, migrations, tx helpers, AGE session state), while `internal/events`, `internal/draws` and `internal/pile` keep owning *what the resource means for them* (event shape, draw shape, entry shape, identity/convergence rules, validation).

Concretely:
- `store.Open(ctx) (*store.DB, error)` — resolves DSN (parallel to `config.Home()`), builds the pgxpool with `AfterConnect` wired for AGE, runs pending migrations.
- `store.DB.Tx(ctx, func(pgx.Tx) error) error` — the one transaction helper every domain package calls through.
- Each domain package contributes its own migration file/embedded SQL to `store`'s migration set (events' event table, draws' table, pile's entry/version tables) but the migration *runner* lives in `store`.
- `events.Emit`, `draws.Append`/`Load`, `pile.Store.Put`/`Open`/`All` keep their existing signatures. Every one of the 9+ production call sites in `tender`, `gate`, `dispatch`, `cli/*` is untouched by this seam decision — the call sites already only know about `events.Emit(...)`, `draws.Append(...)`, `pile.Store.Put(...)`, and that stays true.

This is a MODIFIED (not rewritten) contract for the three domain packages, and a NEW package for the mechanism itself.

## 2. The Local Write Queue

**Verdict:** NEW package `internal/queue`, a peer of `internal/store` — domain packages write to `queue`, `queue` drains into `store`.

**Boundaries.** `queue` does not wrap `store` (that would make every write pay a network round-trip before returning, which is exactly what the "writes never block on the network" constraint forbids) and it does not sit inside `store` (that would make the durability guarantee an implementation detail of the database client, invisible to the domain packages that need to reason about it — e.g. `yield --health` needs to say whether the *queue* is backing up, not whether Postgres is down). It sits beside both, as its own thing:

```
events.Emit() ──┐
draws.Append() ─┼──> queue.Enqueue(op) ──(fsync, local, same mechanism   ──> queue.Drain(ctx) ──> store.DB.Tx(...)
pile.Put() ─────┘                          phase 01 built for events)          (on next invocation,
                                                                                 or explicit drain cmd)
```

`queue.Enqueue` takes a small serializable "intent" (an operation the store package knows how to apply — e.g. `{kind: "event.append", payload: ...}`), appends it to a local fsynced log, and returns. This is a direct generalization of phase 01's own mechanism in `internal/events`: append, fsync before returning, first-failure marker, health-reportable. That work was explicitly called out in PROJECT.md as "what makes the v0.2 local write queue trustworthy" — it is the prior art `queue` is built from, not something built alongside it from scratch.

**Read-your-own-writes.** The queue is local-only; it says nothing about consistency across machines, which matches the "shared topology, single gardener first" decision (team use is a configuration change layered on *later*, not solved by this queue). Within one machine, a read immediately after a write must not silently miss it — a `hugel yield --health` run right after a `hugel gate` on the same box has to see that gate's event. The read path in `events.HealthOf`, `draws.Load`, `pile.Store.All` therefore becomes: query `store` for the durable rows, then overlay whatever is still sitting undrained in the local `queue` for this garden. This overlay is cheap at the current volumes (single digits of pending writes between drains, not thousands) and it is the same "derived index, disposable" posture the pile's own README already states for reads — nothing new is asserted as truth by the overlay, it is a merge view over two logs that will converge once drained.

**Where drain runs.** The existing architecture is explicit that hugel's CLI paths run single-threaded, with no goroutines — tender and gate delegate long-running work to *external* tmux sessions, not to goroutines inside the hugel binary. A goroutine started inside a one-shot CLI invocation does not outlive that invocation, so "background goroutine" is not free the way it would be in a long-running server. Three real options, and they are not mutually exclusive:

1. **Opportunistic drain on next invocation (default).** Any `hugel` command that touches `store` makes one bounded, best-effort attempt to drain the local queue before doing its own work — bounded by a short timeout, and *never* allowed to fail or slow the command that triggered it (a failed drain attempt just leaves the backlog for the next invocation). This is the natural default because it needs nothing new to run: hugel is invoked constantly (gate, tender, yield, soil) and each invocation is a free opportunity to shrink the backlog.
2. **Explicit drain command** (`hugel queue drain` or folded into an existing `hugel sync`), for the case where the gardener has been offline long enough that they want to force a drain, or where a cron job wants to drain on a schedule without waiting for organic invocations.
3. **Background goroutine inside the resident garden TUI.** v0.2 also plans to make `hugel garden` session-persistent (it currently renders once and exits). Once that lands, garden is a genuinely long-running process and can host a drain loop the way a server would. This is available later, not on day one, and should not be assumed as the primary drain mechanism because it depends on a different, independently-scoped requirement ("session-persistent garden") landing first.

Recommend (1) as the mechanism that ships with the queue itself, (2) as a companion command built in the same phase since it is nearly free once (1) exists, and (3) as a later enhancement once the resident garden exists — not a blocker to the queue shipping.

## 3. Migration and Coexistence

**Verdict:** one-shot import, not dual-write, not read-through. NEW command (e.g. `hugel migrate import`, `internal/cli`), reading MODIFIED domain packages' old file formats directly.

At 13KB of events, 22 draw records and a few MB of pile entries, dual-write and read-through both solve a problem this migration doesn't have: they exist to avoid moving all the data at once because moving it all at once is too slow, too big, or too risky to do in one pass. None of that is true here — the whole existing garden fits in memory and can be inserted into Postgres in well under a second. Dual-write would instead *add* a real cost: two writers that can drift, a reconciliation story, and an ambiguous window where it's unclear which store is authoritative — exactly the ambiguity the "no derived structure may be the only record" constraint exists to prevent. Read-through is a mechanism for staged cutover of large datasets; there is no large dataset here to stage.

**Concrete sequence:**
1. `internal/store` exists, migrated, reachable (may be empty).
2. `hugel migrate import` reads `events.jsonl`, `draws.jsonl`, and the pile's git-tracked entry files directly (it does not need `events`/`draws`/`pile` to have been converted to write through `store` yet — it only needs read access to the old formats and write access to `store`), and inserts everything in one pass. Idempotent by construction: events/draws import can dedupe on (time, bead, name) or just refuse to double-import an already-imported log; pile import reuses the identity/convergence logic `pile.Store.Put` already has (bed + type + normalised title), so re-running the importer against an already-imported pile converges rather than duplicates.
3. A single switch (env var or config flag, e.g. `HUGEL_STORE=postgres`) changes which backend `Emit`/`Append`/`Put`/`Load` target. Both backends can be compiled in simultaneously, so the "flag day" is one flag flip after one import command — not a deploy, not a long coexistence window.

**Rollback.** Because the importer only *reads* the old files and never deletes them, rolling back before any new Postgres-only writes have accumulated is just flipping the flag back. The harder case is rollback *after* new work has landed in Postgres — those writes have no file-format equivalent unless something captured them on the way in. Rather than adding a second dual-write path purely for rollback safety, reuse the component already being built for a different reason: the local write queue (§2) durably logs every write before it reaches Postgres. That queue's full history is sufficient to replay every post-cutover write back into the JSONL formats if Postgres turns out wrong, which is the same discipline the milestone already commits to for the AGE graph itself ("droppable and rebuildable... so a migration has nothing irreplaceable to lose") — applied one layer down, to the Postgres-vs-files decision rather than only to the graph-vs-tables decision. This means the rollback story should be validated as part of building the queue, not deferred to "if we ever need it": prove the queue's log can rebuild a JSONL file before calling the queue done.

## 4. Testing Architecture

**Verdict:** `config.Sandbox`'s guarantee is replaced by a schema-per-test-run guard (NEW, likely `internal/store` or a sibling test-support package), backed by testcontainers in CI. Per-test transactional rollback is the wrong default here — call that out explicitly, since it is the obvious-looking answer.

`config.Sandbox` panics when a test resolves the garden outside a temp directory. It exists because every leak into the gardener's real garden happened by omission — a test that didn't know it touched the garden at all — and per-test `t.Setenv` discipline was tried and failed for exactly that reason. The database equivalent needs the same "refuse by construction" property, not a documentation convention.

The tempting database analogue — begin a transaction per test, roll it back after — has a real limitation directly relevant here: it only isolates code that does all its work inside the one transaction the test holds open. `internal/store`'s pool is built around `pgxpool` with an `AfterConnect` hook that runs `LOAD 'age'` and sets `search_path` on *every new connection* it hands out — that's the mechanism AGE requires, since both are per-connection session state, not per-transaction. A pool of N connections handing out a different physical connection per acquisition is precisely the case where the rollback-per-test pattern silently stops isolating: code under test that acquires a second connection from the pool (which `store.DB.Tx` legitimately might, under contention) writes and commits outside the test's held transaction, and the "rollback" cleans up nothing for it.

Recommend instead: **schema-per-test-run** (or per-test-package) against a single long-lived Postgres+AGE instance — `CREATE SCHEMA hugel_test_<random>`, run store's migrations into it, point the pool at it via `search_path`, run the tests, `DROP SCHEMA ... CASCADE` at teardown. This composes correctly with a real pool and real AGE session state, because it isolates at the same level AGE's session state already lives at (the connection's search_path), rather than at the transaction level AGE doesn't operate at. For CI without a standing Postgres, wrap that in testcontainers (an AGE-enabled Postgres image, e.g. `apache/age`), started once per test run/package rather than once per test, to keep container-startup cost off the per-test critical path.

**A new guard, mirroring Sandbox's instinct:** `config.Sandbox` panics on a real path; the equivalent here should refuse to run store-backed tests against a DSN that doesn't look like a test schema/container (host not localhost/testcontainer network, or database/schema name not prefixed `hugel_test_`). This is the same "the hazard is omission, not ignorance" reasoning Sandbox's own comment gives, applied to a connection string instead of a directory.

**Which packages actually need a database.** The current dependency graph already answers this. Packages that take data in and hand data back out — `internal/soil` (ranking/scoring), `internal/pile`'s entry validation and identity logic, `internal/survival` (fact/verdict grading), `internal/config`'s `Kin` logic, `internal/redact` — don't talk to storage today and won't need to; their tests stay exactly as hermetic as they are now. Only `internal/store`, `internal/events`, `internal/draws`, and `internal/pile`'s `Store` type genuinely need a real (or testcontainer) database in their own unit tests, because those are the packages whose job *is* the storage mechanism. Everything that merely *calls* `events.Emit` / `draws.Append` / `pile.Put` incidentally — `tender`, `gate`, `dispatch`, `compost` — should keep testing against a fake/interface double for `store`, exactly as they presumably already avoid needing a real git remote to test pile-adjacent behavior today. Reserve the real database for the four packages that own the seam.

## 5. Build Order

**Sequential spine:**

1. **`internal/store` (NEW).** Pool + `AfterConnect` (AGE `LOAD`/`search_path`) + migration runner + `Tx` helper + initial schema (events, draws, pile tables; AGE graph init can be a no-op migration for now — the graph itself is out of this milestone's critical path). Nothing downstream can be built against a moving target here, so this is first.
2. **`internal/queue` (NEW), largely in parallel with #1.** Its durable local append/fsync/failure-marker mechanism is a direct generalization of phase 01's `internal/events` work and needs no dependency on `store` to be written and unit-tested — it just needs an agreed shape for the "op" it queues (an interface or small tagged struct `store` will later know how to apply). Its `Drain()` implementation is the one piece that is strictly sequential after #1 exists.
3. **Testing infrastructure — schema-per-test harness + testcontainers wiring + the DSN guard (§4) — should land as effectively "step 0," before step 4, not after.** Converting `internal/events` against a live ad hoc database with no repeatable test story would repeat the exact hazard `config.Sandbox` was built to close, one layer down. Build this alongside #1, using #1's schema as soon as it's roughed in.
4. **`internal/events` MODIFIED — convert first among the three domain packages.** Smallest surface (one write path, one reader — `HealthOf` — plus the failure-marker/health semantics phase 01 already hardened), fewest call sites relative to its importance, and the domain whose durability story is already the most rigorously tested in this codebase. Converting it first extends phase 01's investment directly rather than shelving it while draws/pile get attention.
5. **`internal/draws` MODIFIED — convert second.** Four read call sites (`tend.go`, `garden.go`, `yield.go` ×2) but a flat, simple record shape and no git entanglement. Does not depend on `internal/events`'s conversion being done — the two can be parallelized across two workstreams if desired, since neither reads the other.
6. **`internal/pile` MODIFIED — convert last, and scope it as its own phase.** This is the hardest of the three: git today supplies both the write mechanism *and* the pile's versioning/history (`Store.Commit`, an entry's lineage via git log). Moving to Postgres forces a decision this milestone hasn't made yet — whether Postgres rows get their own history/audit mechanism (a superseded-chain or version table) or whether git versioning is deliberately kept in parallel for a while. That decision has materially more surface area than events or draws and should not be bundled into the same phase as either.
7. **`hugel migrate import` (NEW), parallel to 4–6, gated only on #1.** It reads the *old* file formats directly and writes through `store`; it does not need `events`/`draws`/`pile` to have been switched over to the new backend yet, so it can be developed alongside their conversions rather than waiting on them.

**Parallelizable:** `store` and `queue` (once the op-shape contract is fixed); the test harness and `store`'s schema; the migration importer against the domain-package conversions (steps 4–6); events vs. draws conversion against each other.

**Strictly sequential:** `store` before `queue.Drain()` can be implemented; `store` before any domain-package conversion; `queue` before any domain package is converted — the "writes never block on the network" constraint means `Emit`/`Append`/`Put` must write to `queue` from the first day of conversion, not to `store` directly with a queue retrofitted later, since that would mean redoing the write path a second time.

**Smallest useful first slice:** steps 1–4 end to end — `store` + `queue` + `internal/events` converted, `yield --health` reporting through the new path, `hugel migrate import` handling just `events.jsonl`, and the schema-per-test harness in place. This is genuinely shippable on its own: it proves the entire substrate (pool, AGE session plumbing even though nothing queries the graph yet, migration runner, durable local queue, opportunistic drain, queue-replay rollback, and the new test story) on the smallest, best-already-tested, lowest-risk domain, before either `draws`' four read call sites or `pile`'s git-vs-Postgres-history decision are in scope. It is also the most direct continuation of what phase 01 already built, rather than a detour from it.

## Anti-Patterns to Avoid

### Letting each domain package own its own pool/migrations
**What people do:** give `internal/events`, `internal/draws`, `internal/pile` each their own `pgxpool.Pool` and migration file, mirroring how each currently owns its own file mechanism.
**Why it's wrong:** three uncoordinated pools against one server, three migration histories that can race or diverge, and AGE's per-connection session setup (`LOAD 'age'`, `search_path`) reimplemented three times with three chances to get it subtly wrong.
**Instead:** one `internal/store`, three domain packages contributing schema/queries to it.

### Writing straight to the database on the hot path, queue added later
**What people do:** convert `events.Emit` to call `store.DB.Tx` directly first, planning to insert the queue "once the store works."
**Why it's wrong:** the write path has to be rebuilt anyway to add the queue in front, and in the meantime every gate/tender run depends on network reachability — the exact failure mode the milestone rules out ("losing work when the database is unreachable").
**Instead:** the queue exists before any domain package is converted; `Emit`/`Append`/`Put` write to `queue` from their first Postgres-backed commit.

### Per-test transaction rollback as the database test guarantee
**What people do:** reach for `BEGIN`/`ROLLBACK` per test as the direct analogue of `t.TempDir()`.
**Why it's wrong:** it silently stops isolating as soon as code under test acquires a second pooled connection — which a pool built for AGE's per-connection session state will do routinely.
**Instead:** schema-per-test-run against a real pool, backed by testcontainers in CI.

### Long-lived dual-write as the migration strategy
**What people do:** write to both files and Postgres for a probation period "to be safe."
**Why it's wrong:** at 13KB/22 records/a few MB, the safety dual-write buys costs more (drift, reconciliation, an ambiguous authority window) than the one-shot import it's avoiding.
**Instead:** one-shot import + flag flip; rollback safety comes from the queue's own durable log, not from a second write path.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Postgres (server) | `pgx`/`pgxpool` over a DSN resolved by `internal/store` (parallel to `config.Home()`) | Pure-Go driver keeps the hugel binary cgo-free per the stack constraint; the server itself is now a runtime requirement, which the milestone explicitly accepts. |
| Apache AGE (extension) | Per-connection `LOAD 'age'; SET search_path = ag_catalog, "$user", public;` via `pgxpool.Config.AfterConnect` | Session-scoped, not transaction-scoped — this is exactly why per-test transaction rollback doesn't compose with it (§4). AGE pins the server to PG 11–18, below mainline's current release. |
| bd / Dolt | Unchanged — bd stays authoritative for beads; Postgres only carries bead ids | No new integration; out of scope for this seam. |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `internal/events` / `internal/draws` / `internal/pile` ↔ `internal/queue` | Direct call (`queue.Enqueue(op)`) on every write | Domain packages never call `store` directly on the write path. |
| `internal/queue` ↔ `internal/store` | Direct call (`queue.Drain(ctx)` invokes `store.DB.Tx`) | Drain is the only path from queue into store; queue never bypasses store on drain. |
| `internal/events` / `internal/draws` / `internal/pile` ↔ `internal/store` (reads) | Direct call, overlaid with undrained `queue` entries | Read-your-own-writes on one machine; no promise across machines yet. |
| `internal/cli`, `internal/tender`, `internal/gate`, `internal/cli/dispatch` ↔ domain packages | Unchanged call-site APIs (`events.Emit`, `draws.Append`, `pile.Store.Put`) | This seam decision is designed to leave every existing call site untouched. |
| `hugel migrate import` ↔ old file formats + `internal/store` | Direct file read, direct `store` write | No dependency on whether `events`/`draws`/`pile` have been converted to the new backend yet. |

## Sources

- Direct reads of `internal/events/events.go`, `internal/events/convention.go`, `internal/draws/draws.go`, `internal/config/config.go`, `internal/config/sandbox.go`, `internal/pile/store.go`, `internal/gate/run.go`, `internal/tender/tender.go`, `internal/tender/start.go`, `go.mod` — HIGH confidence, ground truth for current architecture.
- `.planning/PROJECT.md` (Current Milestone, Constraints, Key Decisions) — HIGH confidence, this milestone's own stated decisions and reasoning.
- [pgxpool AfterConnect pattern for per-connection AGE session setup](https://github.com/jackc/pgx/discussions/1989) — MEDIUM confidence, community pattern, not yet verified against this codebase.
- [pgx / pgxpool documentation](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool) — MEDIUM-HIGH confidence, official package docs.
- [Testcontainers per-test-database / rollback-pattern tradeoffs](https://qaskills.sh/blog/testcontainers-postgres-per-test-database) — MEDIUM confidence, cross-language pattern discussion (Go-specific examples were not found directly; the general tradeoff — rollback fails once code opens a second connection — is language-agnostic and directly applicable to a pgxpool-based store).
- [Local durable outbox / write-ahead queue pattern in Go](https://threedots.tech/post/sqlite-durable-execution/) — MEDIUM confidence, corroborates the enqueue-locally/drain-later shape recommended for `internal/queue`.
