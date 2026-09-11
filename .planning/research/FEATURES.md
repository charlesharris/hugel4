# Feature Research

**Domain:** Event-sourced relational store + local write queue + multi-machine single-user sync + property graph, for a personal dev-tool substrate (hugel v0.2 "The Shared Garden")
**Researched:** 2026-09-11
**Confidence:** MEDIUM-HIGH (patterns are well-established industry practice; sizing recommendations are opinionated and calibrated specifically to hugel's stated ~50 events/day, one gardener)

## Sizing Note (read this before the tables)

Every recommendation below is sized to **~50 events/day, one gardener, occasionally two machines**. That is roughly 18,000 rows/year in the event table and a write queue that is empty 99% of the time. Nearly every "best practice" article about event sourcing, outboxes, and graph databases is written for a system running orders of magnitude hotter (thousands of writes/sec, multi-tenant, many concurrent writers). This document explicitly flags where that gap turns standard practice into waste, per PROJECT.md's own established pattern of recording deliberate refusals.

---

## 1. Event Sourcing in a Relational Store

### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Monotonic append-only sequence (`bigserial`/`identity` PK, or a Postgres sequence) | Total ordering of events is the entire point of a wide-event log; without it, "what happened, in what order" is unanswerable. Phase 01 already relies on ordered append for the JSONL log — Postgres must preserve that guarantee. | LOW | A single `id bigserial primary key` plus `occurred_at timestamptz not null default now()` is sufficient. Do not build a custom sequencing scheme. |
| Immutability enforcement | The pile's own constraint (PROJECT.md: "Entries, sessions and landings are immutable facts") already states events must not be mutated after write. A relational store makes `UPDATE`/`DELETE` *possible* in a way a flat JSONL file never was — that possibility has to be closed off explicitly, or it will eventually be used. | LOW | A `REVOKE UPDATE, DELETE` on the table role, or a `BEFORE UPDATE OR DELETE` trigger that raises, is enough. This is the single highest-leverage table-stakes item: it converts a JSONL append-only *habit* into a store-enforced *guarantee*. |
| Idempotency key on every row | The local write queue (outbox) guarantees at-least-once delivery, not exactly-once (see §2). Retried drains **will** occasionally attempt to insert the same event twice. Without a dedupe key, a network blip during drain silently duplicates rows in the wide-event log — corrupting exactly the token-cost and draw-precision measurements the whole system exists to produce. | LOW | A UUID generated client-side at emit time (not server-side at insert time), stored as a unique column. `ON CONFLICT (idempotency_key) DO NOTHING` on insert. This is a direct dependency: the existing `Emit` call sites (phase 01) need to generate this key at the point of emission, before the event ever touches the queue. |
| Actor/machine id on every row | PROJECT.md requires "an actor id on every row from the first migration" so team use is a config change, not a migration. For v0.2 specifically (one gardener, several machines), this is also the *only* way to answer "which machine wrote this" (see §3). | LOW | `actor_id text not null` (or a small `actors` lookup table if you want referential integrity — not required at this scale). Populate with a machine-derived identifier (hostname + a persisted install id), not just a username, since the same gardener is the actor on every machine. |
| Ids-not-counts, preserved through the migration | Already a hard constraint (PROJECT.md: "store the count and the measurement does not exist"). Moving to Postgres must not regress this — it would be trivially easy to add a `count` column "for convenience" and start letting the ids atrophy. | LOW | No new work — this is a *do-not-regress* item, not a new feature. Column types should be array/jsonb of ids, not integers. |
| Basic indexing (time, actor, kind) | Table stakes for *querying* the log at all — `yield --health`, draw reconstruction, and any dashboard need to filter by time range and by kind without a sequential scan. | LOW | `(occurred_at)`, `(actor_id)`, `(kind)` — three plain btree indexes. At 50 events/day even a sequential scan over a year of data (~18K rows) is sub-millisecond, so this is about query ergonomics, not performance. |

### Anti-Features (over-engineering at ~50 events/day)

| Feature | Why Requested | Why Problematic Here | Alternative |
|---------|---------------|-----------------------|-------------|
| Time-range partitioning of the event table | Standard advice for "audit logging at scale" (every source consulted recommends it) — it's the first thing anyone suggests for an append-only event table. | At 18K rows/year, partitioning adds DDL maintenance (creating new partitions, attaching/detaching, backfill scripts) to solve a problem — query performance on a huge table — that does not exist. A single unpartitioned table with a time index will out-perform the *human cost* of maintaining partitions for years. | One table. Revisit only if row count crosses ~10M (would take ~200,000 years at this rate) or if a specific slow query is actually observed. |
| Retention/deletion policy | Compliance-driven practice ("keep 90 days, archive/delete the rest") is reflexive in event-sourcing writeups. | hugel's core value is *keeping* what work leaves behind — deleting events destroys the exact record the product exists to preserve. There is no storage-cost pressure at this volume (18K rows/year is kilobytes). | Keep everything, forever. If storage ever becomes a real concern, cold-archive to object storage — never delete. |
| Snapshotting / materialized aggregate state | Classic event-sourcing pattern for systems that replay millions of events to reconstruct current state on every read. | hugel doesn't replay the event log to reconstruct state on the hot path — the pile, draws, and beads are already the current-state stores. The event log is a wide-event *record*, not a source you replay to answer "what is true right now." Snapshotting solves a problem hugel doesn't have. | None needed. If a future "replay to rebuild the graph" use case needs this, revisit then — it's explicitly out of scope for this milestone's substrate work. |
| Schema versioning / event envelope framework (e.g., a formal event schema registry, protobuf/avro versioning) | Common in systems where many independent producers write differently-shaped events and consumers must handle version skew. | One gardener, one codebase, one deploy. There is no producer/consumer version skew to manage — a schema migration is just a Go struct change plus a Postgres migration, reviewed by the same person who wrote it. | Plain Go structs marshaled to `jsonb`, migrated with ordinary `ALTER TABLE`/backfill scripts as needed. |
| Optimistic concurrency version numbers per aggregate | Standard in CQRS/event-sourcing where multiple writers race to append to the same aggregate stream and must detect conflicting appends. | hugel's events are append-only wide events describing *completed* units of work (a gate, a tender start) — there is no "aggregate" being concurrently mutated by two writers racing on the same stream. Two machines can write **different** events concurrently; they never contend for the same row. | None needed. This would matter if v0.3 introduced concurrent edits to a *single mutable* record (see §3 — that's the pile's problem, not the event log's). |

**Dependency note:** the idempotency key and actor id columns are the two pieces of new surface the *existing* `Emit` call sites (phase 01, all ten production sites) must grow before the migration — everything else is schema/DDL work independent of application code.

---

## 2. Outbox / Store-and-Forward from a Client

This is the newest and highest-risk piece of the milestone: writes must never block `hugel gate` on network reachability, and a local queue is now a durability-critical component that didn't exist before.

### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Local durable queue (fsync-before-return) | PROJECT.md is explicit: "a gate on a dead network still records." Phase 01 already built exactly this discipline for the JSONL log (`Emit` fsyncs before returning, fails loudly). The queue must inherit that guarantee, not weaken it. | MEDIUM | A local append-only file (or embedded store like BoltDB/SQLite) that fsyncs each enqueued write before the calling goroutine returns. This is a direct reuse/extension of phase 01's durable-append work — not a new design. |
| Drain-on-reconnect, not orchestration | Users expect the queue to *notice* connectivity returning and flush automatically — they should never have to manually "sync" or "retry." | LOW-MEDIUM | Drain triggers: on process start, on a timer/backoff while the queue is non-empty, and opportunistically before any read that needs fresh data. The queue's job is strictly store-and-forward — it must not encode business logic about *when* a write is valid, only *that* it eventually lands. |
| At-least-once delivery + idempotent server-side apply | Universal property of every store-and-forward design (transactional outbox pattern): a dispatcher can crash between "sent" and "marked sent," so duplicates are a known, accepted trade-off, not a bug to chase out. Exactly-once is not achievable here without distributed transactions, which are explicitly the wrong tool for this scale. | LOW (given §1's idempotency key already exists) | Because every event already carries a client-generated idempotency key (§1), "duplicate suppression" is just `ON CONFLICT DO NOTHING` at insert time. Do not build anything more elaborate — the key that already exists for a different reason (queue-retry dedup) is sufficient. |
| Per-source ordering, not global ordering | Users expect that events written in sequence *on one machine* land in that sequence. They do **not** expect strict global ordering across two machines writing concurrently — that's not achievable without a coordination protocol, and nobody actually needs it for a wide-event log. | LOW | Preserve local (per-actor, per-queue) FIFO ordering on drain. Global ordering falls out naturally from `occurred_at` timestamps for display purposes, with the caveat in §3 about clock skew — it does not need to be *guaranteed*, only *approximately true*. |
| Queue depth / health surfaced to the gardener | Phase 01 already built exactly this pattern for the JSONL log: `yield --health` distinguishes "nothing has run" from "writes have been failing," across every filesystem state. The queue needs the equivalent: is it empty, how many items are backed up, how long has the oldest one been waiting, and is drain actually happening. | LOW-MEDIUM | Direct extension of the existing health surface (`yield --health`) rather than a new concept — add "queue depth" and "oldest unsent item age" as fields alongside what already exists. This is a hard dependency: this feature does not make sense to design from scratch; it's an addition to code that already exists. |
| Defined behavior when the queue fills | Users need to know what happens if the network is down for a very long time and local disk fills, or the queue grows large. Silence here is the single worst outcome — it's exactly the "durability guarantee nobody exercises" failure mode PROJECT.md calls out. | LOW-MEDIUM | At 50 events/day, "the queue fills" in practice means "the disk fills," which is already covered by phase 01's full-disk handling for the JSONL log. The behavior should be identical: fail loudly, never silently drop, surface via health. No separate "queue capacity" concept is needed — disk space *is* the capacity. |

### Anti-Features (over-engineering at this scale)

| Feature | Why Requested | Why Problematic Here | Alternative |
|---------|---------------|------------------------|-------------|
| Exactly-once delivery (distributed transactions / 2PC between local queue and Postgres) | Sounds like the "correct" guarantee, and is a common ask in outbox-pattern writeups for financial/transactional systems. | At-least-once + idempotent apply (already required for retry-safety anyway) delivers the same observable outcome — no duplicate events ever visible — at a fraction of the complexity, with no distributed-transaction coordinator. Exactly-once delivery as a *transport* guarantee is a well-known trap: it's usually unnecessary because idempotent consumers make at-least-once behave identically from the outside. | At-least-once transport + idempotency key + `ON CONFLICT DO NOTHING` (already table stakes above). |
| A message-broker-style queue with topics, consumer groups, replay-from-offset, etc. (i.e., reaching for Kafka/NATS/etc. "since we need a queue anyway") | The word "queue" invites reaching for queueing infrastructure, and the milestone already introduces a new durable component, which can feel like the moment to "do it properly." | Explicitly out of scope already (PROJECT.md: "A clustered log (Cassandra, Kafka) — write volume is ~5 events/day against tooling built for six-figure writes/sec"). One gardener writing at human speed needs a local file with an fsync and a drain loop, not a broker. | A local append-only file/embedded store, exactly as phase 01 already built for the JSONL log. |
| Priority queues / reordering / selective retry of individual queued items | Feels useful once you imagine "what if one write keeps failing and blocks the rest." | At this volume, a stuck item is a one-time debugging event, not an operational pattern to design around. Building prioritization machinery for a queue that is empty 99% of the time is solving an imagined operations problem. | Strict FIFO drain. If one item is malformed and blocks drain, surface it loudly via health (already required) and let the gardener look at it directly — don't build automated skip-and-requeue logic. |
| A queue status dashboard / UI beyond CLI health output | Once health is surfaced, it's tempting to build a live view of queue state. | `hugel garden` becoming session-persistent (already an Active roadmap item) is the natural home for this later — building a separate queue-specific UI now duplicates that surface before it exists. | Fold "queue depth / oldest item age" into the existing `yield --health` output and, later, into the persistent garden view — don't build a third surface. |

**Dependency note:** this entire section is a direct extension of phase 01 ("Make event writes fail loudly" — done in v0.1). PROJECT.md itself states this explicitly: phase 01's durable append, failure marker, and health surface "are exactly what makes it trustworthy." The write queue is not a new subsystem to design from first principles — it's phase 01's discipline (fsync, fail loudly, distinguish "nothing ran" from "writes are failing") relocated in front of a network call instead of in front of a local file.

---

## 3. Multi-Machine, Single-User Systems

This is the domain area where PROJECT.md's stated model (one gardener, several machines, actor id on every row, "team later") most directly maps to well-known failure modes from personal multi-device sync (Dropbox, git across machines, browser sync, note-taking apps).

### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| "Which machine wrote this" is always answerable | The single most common real complaint in multi-device systems is not knowing which device produced a given record when something looks wrong (a stray entry, a duplicate, a gap). | LOW (falls out of §1's actor_id) | Already covered by the actor_id column — this is a benefit, not new work, as long as actor_id is populated with a *machine*-identifying value, not just "the gardener's username" (which is identical across machines and would answer the wrong question). |
| Append-only log tolerates concurrent writes from two machines with no coordination | Two machines each writing gate events at the "same" wall-clock moment is normal and expected — it must never require a lock, a leader election, or a "who goes first" negotiation. | LOW | This is naturally true for an append-only table with independent per-row idempotency keys and no foreign-key contention between machines. No design work needed beyond what §1 already requires — call this out explicitly as a *non-problem* so it isn't accidentally over-engineered (see anti-features below). |
| Timestamps used for display/ordering, not for correctness | Clock skew across two machines (a laptop with drifted NTP, a desktop that's a few seconds off) is normal and will produce timestamps that are not in true wall-clock order. | LOW | Never rely on `occurred_at` comparison for anything that must be correct (e.g., "which event happened first" in a way that affects behavior). The monotonic sequence id from §1 is the actual ordering authority; timestamps are for human display only. This is a one-line design rule, not a feature to build, but it must be stated so nobody later writes logic that compares timestamps across actors expecting them to be authoritative. |
| The pile (mutable store) needs a real conflict rule for cross-machine edits | Unlike the event log, the pile is *mutable* — review status changes, entries get superseded, links get added. If the same entry is reviewed/edited from two machines before either sync completes, "whoever wrote last, silently wins" is a real risk of quietly discarding a gardener's own judgment call. | MEDIUM | Because writes now go through a shared Postgres (not two independent local files being merged later), most of the classic "two independent local databases diverge and must be reconciled" problem disappears — Postgres is the single arbiter, and last-writer-wins is decided by whichever write actually commits, not by comparing client clocks. The only remaining risk is a *stale read*: gardener reviews an entry on machine A from data fetched before machine B's edit landed. Mitigate with an `updated_at`/version column checked at write time (`WHERE version = $1`), so a stale write fails loudly instead of silently clobbering. |

### Anti-Features (over-engineering at this scale)

| Feature | Why Requested | Why Problematic Here | Alternative |
|---------|---------------|------------------------|-------------|
| Vector clocks / Hybrid Logical Clocks for event ordering | The "correct" distributed-systems answer to clock skew, and comes up in every multi-device sync writeup. | This machinery exists to establish a causal order across *independent, disconnected* replicas that must later merge. hugel's writes go through one shared Postgres — there is a single arbiter of order (the monotonic sequence id), so there is no merge problem to solve with logical clocks. Vector clocks solve a problem this architecture doesn't have because it chose a shared database over independent replicas. | Monotonic sequence id (already table stakes, §1) for order; timestamps for display only. |
| CRDTs for the pile | CRDTs are the standard answer for "same logical record, edited concurrently from two offline devices, must merge without conflict." | The pile is not offline-replicated per machine — it lives in the shared Postgres from the moment this milestone lands. There is nothing to merge because there is only one copy. CRDTs solve the *previous* generation of this architecture's problem (independent files under separate home directories), which this milestone is explicitly retiring. | Optimistic locking (version/`updated_at` check) — sufficient for "did someone else change this since I read it," which is the only real risk once there's one authoritative store. |
| Multi-master / leader-election / active-active database topology | Sounds necessary for "reachable from several machines." | One gardener never writes from two machines *at the exact same instant* to the *same row* — the actual concurrency is "different machines writing different rows minutes or hours apart." A single Postgres instance handles this with zero special topology. Multi-master is solving for write throughput and availability at a scale this project doesn't have and isn't asking for (team use is explicitly deferred, not concurrent-team-use-today). | One Postgres instance (with ordinary backup/replica for durability — see PROJECT.md's restore-drill requirement, which is a *durability* concern, not a *concurrency* one). |
| Machine-specific merge/conflict UI ("machine A says X, machine B says Y, pick one") | Feels like the natural UX for "multi-machine" once you're thinking about sync conflicts. | Because Postgres is the single source of truth (not two local copies being reconciled), there is structurally no "two versions to pick between" — a write either lands or it's rejected by the version check above and the gardener just re-issues it against current state. Building a merge UI answers a question this architecture doesn't ask. | The optimistic-lock failure surfaces like any other write conflict: "this was already changed, re-read and retry" — no bespoke UI. |

**Note on append-only log vs. mutable pile:** this is the key distinction the question asks for, and it's worth stating directly. The event log's concurrency story is nearly free — append-only, independently-keyed rows from independent actors never contend. The pile's concurrency story requires one small, standard safeguard (version check on write) *because* it is mutable and reviewed by a human whose judgment must not be silently overwritten. Everything else in the classic "multi-device sync" literature (CRDTs, vector clocks, merge UIs) is solving for architectures with independent local replicas — which this milestone is deliberately moving *away from* by centralizing on shared Postgres.

---

## 4. Property Graph Modelling (code↔code, code↔ticket, ticket↔ticket, entry↔entry)

### Table Stakes

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Neighbor lookup ("what connects directly to X") | The most basic and most frequent graph query — "what beads touch this file," "what entries link to this decision." | LOW | This is a plain SQL join/index-lookup, and does **not** need a graph engine. `SELECT * FROM edges WHERE src = $1 OR dst = $1` (or the AGE equivalent `MATCH (n)-[]-(m) WHERE n.id = $1 RETURN m`) is trivial either way. Decide this on developer ergonomics (Cypher reads better once AGE is already in place for other queries), not on necessity. |
| Fixed-depth subgraph extraction ("everything within 2 hops of this bead") | Needed to answer "what's the context around this piece of work" for the draw path — directly serves the "Graph in the draw path" roadmap item. | LOW-MEDIUM | Still expressible as 2-3 chained joins in plain SQL at small graph sizes. Becomes meaningfully easier to *write and maintain* in Cypher once AGE exists, but is not, by itself, a justification for adopting a graph engine. |
| Variable-depth path queries (shortest path, "is X reachable from Y at all", arbitrary-depth traversal) | This is the one category that is a genuine table stakes requirement *for choosing a graph engine at all*. Ticket↔ticket dependency chains and code coupling chains are naturally variable-depth (a bead can depend on a bead that depends on a bead, arbitrarily deep), and this is exactly the case recursive CTEs handle awkwardly and Cypher handles directly. | MEDIUM | This is the query class PROJECT.md's own decision log already names as the reason for choosing AGE over recursive CTEs: "recursive CTEs over an edges table get worst exactly at the variable-depth traversal the graph exists for." Confirmed by external sources: recursive CTEs work but "get ugly fast and slower even faster" past a few hops; Cypher avoids the "recursive CTE gymnastics" for this specific shape. This is the one query type that actually earns AGE's presence in the stack rather than plain SQL. |
| Droppable/rebuildable graph (already an existing constraint, not new) | PROJECT.md: "the AGE graph is derived from those tables and nothing may exist solely in it, so it can be dropped and replayed." | MEDIUM | Direct dependency on the widened event emission and the bd dependency-edge capture (both Active roadmap items) — the graph cannot be built, let alone rebuilt, until those substrate items land. This ordering (substrate before graph) is already correctly reflected in PROJECT.md's roadmap and this research doesn't need to re-derive it, only confirm it. |

### Differentiators

| Feature | Value Proposition | Complexity | Notes |
|---------|--------------------|------------|-------|
| Structure-aware soil ranking (graph consulted in the draw path, not just lexical match) | This is the actual payoff of building the graph at all — ranking by "what's structurally connected to the current work" is not something a lexical ranker can do, and it's explicitly named as an Active roadmap item ("cochange already proves the shape of this"). | MEDIUM-HIGH | Direct dependency on cochange (existing, code↔code only) generalizing to the full graph (code↔ticket, ticket↔ticket, entry↔entry). This is where the graph earns its keep as a differentiator rather than a nice-to-have query convenience — it changes what soil delivers, not just what a gardener can ask for manually. |
| Cross-entity path queries spanning types (e.g., "what decisions are reachable from this file, through the beads that touched it") | Genuinely hard to express well in SQL once it spans three or more entity types with different join keys; natural in a labeled-property-graph model where node/edge types are first-class. | MEDIUM | This is a real differentiator *if* a concrete use case emerges (e.g., surfacing a decision during soil draw because it's two hops from the current file through a closed bead). Don't build generic "ask any path query" support speculatively — build the specific traversal soil actually needs. |

### Anti-Features (over-engineering / wrong tool)

| Feature | Why Requested | Why Problematic Here | Alternative |
|---------|---------------|------------------------|-------------|
| Migrating off Postgres to a dedicated graph database (Neo4j, etc.) | Once you're doing "real" graph work, it's tempting to reach for a purpose-built graph engine with more mature tooling. | Already explicitly evaluated and rejected in PROJECT.md's decision log — a standalone graph DB would add a second runtime dependency for a single-binary CLI, exactly the operational cost the AGE choice was made to avoid (`pgx` stays pure Go; the *server* gains an extension, the *client binary* does not gain a dependency). At this data volume (a few hundred entries, no edges yet), no graph engine's performance advantage is reachable, let alone needed. | Apache AGE on the existing Postgres instance — already the decided path. |
| Using recursive CTEs *and* AGE side-by-side "to be safe" / building both | Hedging instinct when adopting new technology with a documented risk (AGE pins Postgres to 11-18, below current release; no CI yet). | Maintaining two parallel query implementations for the same graph doubles the surface that must be droppable/rebuildable and doubles what breaks on a schema change. The risk PROJECT.md already accepted (server version pin, no CI yet) is a reason to *harden* the AGE path (get CI in place), not a reason to duplicate it in SQL as a fallback. | Commit to AGE for the variable-depth queries; use plain SQL only for the genuinely trivial single-hop lookups where a graph engine adds no value (see table stakes above) — that's a division of labor, not a hedge. |
| Human-asserted edges / a UI for manually drawing relationships | Feels like an obvious feature for a "knowledge graph" — let the gardener declare that two things are related. | Already recorded as a deliberate refusal in PROJECT.md, measured at zero uptake across 289 entries ("Edges asserted by a human"). Repeating this research finding: a graph that depends on a human noticing and recording a relationship will have the graph that already exists today — 289 entries, zero edges. | Every edge is derived from a durable source (git cochange, bd dependency links, entry `Paths`/`Beads` metadata, the widened event log) — never asserted. |
| LLM-based edge inference at extraction/compost time | Appealing because it could "find" relationships a mechanical derivation would miss (semantic similarity, implied causality). | Already tried once and explicitly recorded as having "left nothing behind" (`680d9417`). Re-attempting this without a different mechanism or evaluation is repeating a documented failure. | Mechanical derivation only (git history, bd's own dependency graph, path/bead metadata already carried on entries) — the same substrate-first principle applied to edges as to everything else in this milestone. |
| A generic graph visualization / explorer UI | Once a property graph exists, building a visual explorer (force-directed layout, click-to-expand) is an easy scope trap — it *looks* like the natural next feature. | No stated need for this in PROJECT.md, and it competes directly with `hugel garden` becoming the single persistent surface (an Active roadmap item) rather than spawning a second one. Graph value in this milestone is consumed by soil's ranking, not by a gardener browsing a node-link diagram. | Expose graph queries through soil's draw output and through targeted CLI queries (`hugel graph neighbors <id>`, etc., if useful for debugging) — not a standalone visual tool. |

**Dependency note (critical for roadmap ordering):** the graph section as a whole has a hard dependency on the substrate-widening items already listed as Active in PROJECT.md — "Widen event emission," "Stop discarding bd's dependency graph," and "Enrich what a bead carries." Building the AGE projection before those land would produce, in PROJECT.md's own words, "a graph derived from this substrate today would be very nearly empty." This research confirms rather than revises that ordering.

---

## 5. Anti-Features (Consolidated — What to Explicitly Not Build)

The four sections above each carry domain-specific anti-features inline; this table consolidates the ones that are cross-cutting or most likely to be independently proposed during roadmapping, so they can be cited directly against a specific proposed phase.

| Anti-Feature | Surface Appeal | Why It's Waste at This Scale | Alternative |
|---------|---------------|-----------------------------|-------------|
| Clustered/distributed log or broker (Kafka, Cassandra, NATS JetStream, etc.) | "We're building durable event infrastructure, this is what production systems use" | Already refused in PROJECT.md at ~5 events/day; this research's ~50/day sizing doesn't change the conclusion (still 3-4 orders of magnitude under what such tooling is built for). Operating a cluster becomes the single biggest new runtime liability in a single-binary CLI. | Postgres table + local durable queue file, both already decided. |
| Table partitioning, retention/archival policy, snapshotting on the event table | Reflexive advice in every event-sourcing/audit-log guide consulted | 18K rows/year never approaches a size where partitioning helps, and hugel's core value is keeping records forever, not aging them out. | One unpartitioned table, kept indefinitely. |
| Exactly-once delivery via distributed transactions between queue and DB | Sounds like the "correct" guarantee for a durability-critical write path | At-least-once + idempotency key (needed anyway for other reasons) is observably identical and enormously simpler; distributed transactions are a well-known trap even at scales where they'd be technically justified. | At-least-once transport, idempotent apply. |
| Vector clocks / CRDTs for cross-machine conflict resolution | Standard answer to "multi-device sync," appears in nearly every source on the topic | Solves a problem specific to *independent replicated copies* reconciling later. This architecture centralizes on one shared Postgres precisely to avoid having independent copies to reconcile. | Monotonic sequence id + optimistic version check on the one mutable store (the pile). |
| Migrating to a dedicated graph database | "Real" graph workloads deserve a "real" graph engine | Already evaluated and rejected — adds a second runtime dependency for marginal-to-zero benefit at current graph size (a few hundred entries, presently zero edges). | Apache AGE on the existing Postgres instance. |
| Human-asserted graph edges / manual relationship UI | Obvious feature for a "knowledge graph" product | Measured failure already on record (zero uptake across 289 entries). Depends on a human noticing and acting, which this project has repeatedly found does not happen. | Mechanically derived edges only. |
| LLM edge inference at extraction time | Could catch relationships mechanical derivation misses | Already tried, already recorded as having produced nothing durable. | Mechanical derivation from git, bd, and entry metadata. |
| A separate queue-status or graph-visualization UI | Each new subsystem invites its own dashboard | Fragments the single surface (`hugel garden` becoming session-persistent is the one Active surface item) and adds maintenance burden disproportionate to a single gardener's need to glance at status. | Fold status into existing/planned surfaces: `yield --health` for queue, soil/draw output and targeted CLI for graph. |
| Schema/event versioning framework, aggregate optimistic-concurrency versioning on the event log | Standard CQRS/event-sourcing machinery | No independent producers/consumers to version against (one codebase, one deploy), and no concurrent writers contending on the same event-log row (append-only, no aggregate stream to protect). | Plain Go structs + ordinary migrations; version-check only where mutation actually happens (the pile). |

---

## Feature Dependencies

```
[Widened event emission (Active, existing roadmap item)]
    └──requires──> [Idempotency key generated at Emit call sites]
                       └──requires──> [Postgres event table with unique idempotency_key column]

[Local write queue / outbox]
    └──requires──> [Phase 01: durable append, fsync, fail-loudly, health surface]  (existing — direct reuse)
    └──requires──> [Idempotency key on events]  (dedup on drain retry)
    └──enhances──> [yield --health]  (existing — extend, don't replace)

[Actor id on every row]
    └──requires──> [Migration touching every existing table]  (events, draws, pile)
    └──enables──> ["which machine wrote this" queries]
    └──enables──> [Future team-use configuration change (out of scope this milestone)]

[Apache AGE property graph]
    └──requires──> [Widened event emission]  (substrate must carry the facts first)
    └──requires──> [bd dependency graph preserved, not collapsed to Ready bool]
    └──requires──> [Enriched bead structural metadata]
    └──enhances──> [Soil ranking / draw path]  (differentiator: graph-aware ranking)
    └──conflicts──> [Human-asserted edges]  (mechanical-only is a hard constraint, not a preference)
    └──conflicts──> [LLM edge inference at extraction time]  (previously tried, previously failed)

[Optimistic version check on the pile]
    └──requires──> [Pile now lives in shared Postgres, not per-machine files]  (this milestone's centralization)
    └──enhances──> [Multi-machine safety for the one mutable store]

[Restore drill / backup verification]
    └──requires──> [Postgres as substrate]  (this milestone)
    └──conflicts──> [Treating an unexercised backup as durable]  (explicit prior regret being corrected)
```

### Dependency Notes

- **Local write queue requires phase 01's durable-append work:** this is the single most important dependency in the whole milestone. The queue is not a new design problem — it's phase 01's fsync/fail-loudly/health discipline relocated from "in front of a local file" to "in front of a network call." Roadmapping should treat this as an extension task, not a from-scratch subsystem.
- **AGE graph requires substrate widening first:** already correctly sequenced in PROJECT.md's Active roadmap ("substrate before graph"). This research confirms it: building the graph against 32 events from two subsystems and a discarded dependency graph produces an empty graph, as PROJECT.md's own Context section states.
- **Idempotency key serves two masters:** it exists for both queue-retry dedup (§2) and general event-log integrity (§1). Design it once, at the point of emission, and let both consumers rely on the same key — don't build two separate dedup mechanisms.
- **Optimistic version check conflicts with (replaces the need for) CRDTs/vector clocks:** because this milestone centralizes on shared Postgres rather than keeping independent per-machine copies, the entire class of merge-conflict tooling that multi-device literature recommends is not needed. This is worth stating as an explicit non-decision in the roadmap so it isn't quietly reintroduced by a future contributor following generic multi-device-sync advice.

---

## MVP Definition (for this milestone's substrate + graph work)

### Launch With (v0.2 core)

- [ ] Postgres event table: monotonic id, actor_id, idempotency_key (unique), immutability enforced via trigger/permissions — this is the floor everything else stands on
- [ ] Local durable write queue extending phase 01's fsync/fail-loudly pattern, draining on reconnect with at-least-once + idempotent apply
- [ ] Queue depth / health folded into existing `yield --health`
- [ ] Actor id populated on every row across events, draws, and pile — the one-line change that makes team-use later a config change
- [ ] Optimistic version check on pile writes (the one place mutation across machines is a real risk)
- [ ] Apache AGE installed and wired, but only after substrate-widening items land — building it earlier produces an empty graph
- [ ] Basic graph queries needed by soil: neighbors and fixed/variable-depth path traversal for the entities that already carry relationships (code↔code via cochange, code↔ticket, ticket↔ticket via bd's preserved dependency graph)
- [ ] Export/backup + a restore drill that actually runs (already an explicit PROJECT.md requirement, load-bearing for the "regret answered by engineering" reframe)

### Add After Validation (v0.2.x)

- [ ] Structure-aware soil ranking consulting the graph, not just lexical match (differentiator — depends on the graph actually holding non-trivial edges, which depends on substrate widening having run for a while)
- [ ] Cross-entity path queries spanning three-plus node types, once a concrete soil use case names the specific traversal needed

### Future Consideration (v0.3+, explicitly not now)

- [ ] Team/multi-gardener use — actor_id already present, but the design work of "who can see/edit what" is genuinely deferred, not merely unbuilt
- [ ] Anything requiring independent per-machine replicas reconciling later (this milestone deliberately moves away from that shape) — should not resurface as a "feature" without first revisiting why it was avoided

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Immutable, idempotent, actor-tagged event table | HIGH | LOW | P1 |
| Local durable write queue (extends phase 01) | HIGH | MEDIUM | P1 |
| Queue health surfaced via `yield --health` | HIGH | LOW | P1 |
| Optimistic version check on pile | MEDIUM | LOW | P1 |
| Restore drill (export/backup/replay proven) | HIGH | MEDIUM | P1 |
| AGE graph: neighbor + variable-depth path queries | MEDIUM | MEDIUM | P1 (blocked on substrate items) |
| Graph-aware soil ranking | HIGH | MEDIUM-HIGH | P2 |
| Cross-entity multi-hop path queries for soil | MEDIUM | MEDIUM | P2 |
| Table partitioning / retention on event log | LOW | MEDIUM | P3 (do not build — see anti-features) |
| Graph visualization UI | LOW | HIGH | P3 (do not build — see anti-features) |
| Team/multi-gardener access control | LOW (this milestone) | HIGH | P3 (explicitly deferred) |

**Priority key:**
- P1: Must have — this is the substrate the milestone is named for
- P2: Should have once P1 substrate is proven to carry real facts
- P3: Explicitly deferred or refused — listed to close the door on scope creep during roadmapping, not because it might get done later

---

## Sources

- [Event Sourcing with Postgres: Building Audit-Proof Backends Without Kafka](https://thebackenddevelopers.substack.com/p/event-sourcing-with-postgres-building) — MEDIUM confidence, corroborates monotonic-sequence + immutability + idempotency as core pattern
- [PostgreSQL partitioning for event tables in audit logging | AppMaster](https://appmaster.io/blog/postgresql-partitioning-event-audit-tables) — MEDIUM confidence, used as the source of the "standard advice" this doc explicitly argues against at this scale
- [Postgres FM | Transcript: Append-only tables](https://postgres.fm/episodes/append-only-tables/transcript) — MEDIUM confidence
- [The Outbox Pattern: Reliable Event Publishing Without Distributed Transactions](https://wilburhimself.github.io/blog/38-the-outbox-pattern-reliable-event-publishing-without-distributed-transactions/) — MEDIUM confidence, corroborates at-least-once as the accepted/standard trade-off
- [Outbox Pattern Survival Guide — Thomas Pierrain](https://medium.com/@tpierrain/outbox-pattern-survival-guide-6ad4b57ef189) — MEDIUM confidence, corroborates per-aggregate ordering only, store-and-forward scope
- [Flutter Outbox Pattern - DEV Community](https://dev.to/guimg/flutter-outbox-pattern-16m3) — MEDIUM confidence, client-side durable queue framing ("process kill, dead battery, plane mode")
- [Cypher graph queries on PostgreSQL with Apache AGE - DEV Community](https://dev.to/franckpachot/cypher-graph-queries-on-postgresql-with-apache-age-3l62) — MEDIUM confidence
- [Postgres as a Graph Database: Four Approaches Compared | Evokoa](https://evokoa.com/blog/postgres-as-a-graph-database/) — MEDIUM confidence, corroborates "recursive CTEs get ugly fast" for variable-depth traversal specifically
- [How Fast Are Postgres 19 Graph Queries? Part 1 — exobench](https://exobench.ai/blog/pg19-graph-queries-part-1) — MEDIUM confidence, performance comparison context (recursive CTE fastest raw, AGE 1.5-2.5x for the ergonomic win)
- [Distributed Clocks and CRDTs – Adam Wulf](https://adamwulf.me/2021/05/distributed-clocks-and-crdts/) — MEDIUM confidence
- [Beyond Offline-First: The Nightmare of Data Synchronization & CRDTs](https://medium.com/@engin.bolat/beyond-offline-first-the-nightmare-of-data-synchronization-crdts-c69501a96c8d) — MEDIUM confidence, corroborates clock-skew-as-data-loss-mechanism framing for naive LWW
- `/Users/charris/src/hugel4/.planning/PROJECT.md` — HIGH confidence (primary source), used for existing-feature boundaries, constraints, and prior recorded refusals throughout this document

---
*Feature research for: hugel v0.2 "The Shared Garden" — Postgres substrate, local write queue, multi-machine single-gardener sync, Apache AGE property graph*
*Researched: 2026-09-11*
</content>
</invoke>
