# Requirements: Hugel

**Defined:** 2026-09-08
**Core Value:** Work done by agents leaves behind why it was done that way — and that record is cheap enough to deliver back into the next session that it actually gets used.

## v1 Requirements

Ordered by dependency: the substrate must carry the facts before anything can be
projected from it.

### Substrate

- [x] **SUB-01**: `events.Record` returns an error instead of swallowing it, so a caller can tell a written event from a dropped one
- [x] **SUB-02**: Event writes are flushed durably, so a crash does not silently lose the tail of the log
- [x] **SUB-03**: A gardener can distinguish "nothing has run since <date>" from "writes have been failing since <date>" without reading the code
- [ ] **SUB-04**: Compost emits a wide event per digested session
- [ ] **SUB-05**: Soil emits a wide event per draw, carrying the entry ids delivered
- [ ] **SUB-06**: Spike, dispatch and pile review each emit a wide event per unit of work
- [ ] **SUB-07**: `HandBack` emits a wide event carrying the bead, the reason and the session that gave up
- [ ] **SUB-08**: Every event carries the ids a relation can be derived from — bead, bed, session, sha, entry ids and paths — rather than counts
- [ ] **SUB-09**: bd's ticket↔ticket dependency edges are read into `beads.Bead` without hugel recomputing readiness
- [ ] **SUB-10**: A bead carries enough durable context that its relations remain derivable after the session that worked it has ended
- [ ] **SUB-11**: Tender progress and gate refusals are written at the boundary that knows them, before the tmux session dies

### Graph

- [ ] **GRAPH-01**: A Postgres + Apache AGE projection is built from the event, draw and entry tables and git (amended 2026-09-11; was SQLite)
- [ ] **GRAPH-02**: The projection holds code↔code, code↔ticket, ticket↔ticket and entry↔entry relations
- [ ] **GRAPH-03**: The projection can be deleted and rebuilt by replay, and no fact exists solely inside it
- [ ] **GRAPH-04**: `go build` still produces one binary with no cgo (`pgx` is pure Go); the server is required for graph queries, and every write path still works with it unreachable (amended 2026-09-11; was "no server process")
- [ ] **GRAPH-05**: A gardener can ask which beads touched a directory, and which entries relate to a bead, without a full log scan
- [ ] **GRAPH-06**: Soil ranking consults structural relations in addition to wording

### Surface

- [ ] **SURF-01**: `hugel garden` stays resident instead of rendering once and exiting
- [ ] **SURF-02**: The resident garden reflects tenders starting, progressing, landing and handing back without being restarted
- [ ] **SURF-03**: The attention list remains sourced solely from bd's `needs-attention` label

### Loop

- [ ] **LOOP-01**: GSD's discuss → plan → execute cycle drives the work loop, with hugel supplying soil in and capturing findings out
- [ ] **LOOP-02**: Findings reach the pile during a session, not only at compost time afterwards
- [ ] **LOOP-03**: A gardener's answer to a handed-back bead is relayed into the tender session that asked, rather than starting a fresh tender

## Milestone v0.2 Requirements — The Shared Garden

Scoped 2026-09-11. The substrate moves into shared Postgres, writes stop depending
on the network, the garden is engineered against being lost, and the relation graph
lands on Apache AGE.

### Store

- [ ] **STORE-01**: Events are written to a Postgres table that refuses updates and deletes, and each event carries a monotonic ingest sequence
- [ ] **STORE-02**: Every record carries a UUIDv7 minted on the machine that emitted it, and the same record delivered twice is stored once
- [ ] **STORE-03**: Every row names the actor and the machine that wrote it
- [ ] **STORE-04**: A soil draw is recorded in Postgres carrying the ids of the entries delivered, not a count
- [ ] **STORE-05**: Pile entries, content included, are stored in Postgres and rank without reading files
- [ ] **STORE-06**: Bead ids are carried in Postgres so a relation can join a bead to events, entries and paths, while `bd` remains authoritative for the beads themselves
- [ ] **STORE-07**: An existing file-backed garden migrates into Postgres, with a report a gardener can read proving what landed matches the source
- [ ] **STORE-08**: A malformed source record costs that record and nothing else, in both the one-shot import and the ongoing drain, and every skipped record is logged with enough content to reconstruct it
- [ ] **STORE-09**: A gardener can read and search recent events without a database client, including writes still sitting undrained in the local queue
- [ ] **STORE-10**: A test run cannot read or write a gardener's real garden or a shared database, and the default `go test ./...` needs no database at all

### Queue

- [ ] **QUEUE-01**: A write is accepted, fsynced and acknowledged locally before Postgres sees it, so `hugel gate` and `hugel tender` never block on the network
- [ ] **QUEUE-02**: Queued writes reach Postgres once it is reachable, and a write delivered more than once is stored once
- [ ] **QUEUE-03**: A single machine's writes keep the order they were made in; cross-machine order is not claimed
- [ ] **QUEUE-04**: A drain that has stopped is visible in `hugel yield --health`, naming when it stopped — never silent
- [ ] **QUEUE-05**: A gardener reading state on the machine that just wrote sees their own writes, drained or not

### Survival

- [ ] **SURV-01**: A gardener can export the whole garden to a portable format in one command
- [ ] **SURV-02**: A garden can be rebuilt from an export into an empty database, losing nothing
- [ ] **SURV-03**: `hugel yield --health` reports when the restore was last exercised, and says plainly when it never has been
- [ ] **SURV-04**: A restore is verified by querying the restored garden — including a live graph query — rather than by the restore command exiting zero

## v2 Requirements

Deferred. Tracked but not in the current roadmap.

### Graph

- **GRAPH-07**: Dispatch consults the graph when choosing what a tender should pick up next
- **GRAPH-08**: The graph surfaces in the garden as a navigable view rather than a query

### Substrate

- **SUB-12**: Events ship to an external collector via the OTLP mapping already anticipated in `internal/events`

## Out of Scope

| Feature | Reason |
|---------|--------|
| An unbounded review queue | A backlog of hundreds is not judged, it is abandoned. Refused twice on the record (`aa4d8fb7`, `2a841ffd`) |
| Non-bead sources in the attention list | One source of truth; the no-inbox refusal is preserved structurally rather than by discipline |
| Edges asserted by a human | Measured at zero across 289 entries (`2b9d936f`) |
| LLM edge inference at extraction time | Built once already and left nothing behind (`680d9417`) |
| A graph that cannot be rebuilt | Every graph hugel kept died with the store it lived in (`internal/cochange/cochange.go`) |
| Cassandra or Kafka as the log | ~5 events/day against tooling for six-figure writes/sec. Postgres left this row on 2026-09-11 and is now the substrate; the objection was operating a cluster, never the SQL |
| Table partitioning, retention policy, snapshotting on the event table | ~18K rows/year never approaches a size where any of it matters, and keeping everything is the point |
| Schema-versioning frameworks and concurrency versions on the event log | One codebase, one deploy, no aggregate contention |
| Exactly-once delivery via distributed transactions | At-least-once plus an idempotency key is observably identical and vastly simpler |
| A message-broker queue, priority queues, a queue-status UI | The queue is a disk-backed outbox for ~50 writes a day, not infrastructure |
| Vector clocks, CRDTs, multi-master topology, merge UI | All solve independent replicas reconciling later; centralising on one Postgres is precisely how that problem is avoided |
| A dedicated graph database, or a graph visualisation UI | AGE rides the substrate already chosen; the two edge-inference approaches are measured failures already on this project's record |
| Treating a backup as durable before it has been restored | The explicit reversal of this project's own regret — exercise the restore rather than avoid the store |
| Hugel writing bd content | Lifecycle transitions only — `Close`, `HandBack`, `Release` |
| Multi-gardener today | v0.2 makes the garden shared across a single gardener's machines and carries an actor id from the first migration, so team use becomes configuration rather than a rewrite. Multiple people is still not built or tested |

## Traceability

Every v1 requirement maps to exactly one phase. Filled during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SUB-01 | Phase 1 | Gaps Found |
| SUB-02 | Phase 1 | Gaps Found |
| SUB-03 | Phase 1 | Gaps Found |
| SUB-04 | Phase 2 | Pending |
| SUB-05 | Phase 2 | Pending |
| SUB-06 | Phase 2 | Pending |
| SUB-07 | Phase 2 | Pending |
| SUB-08 | Phase 2 | Pending |
| SUB-09 | Phase 3 | Pending |
| SUB-10 | Phase 3 | Pending |
| SUB-11 | Phase 3 | Pending |
| GRAPH-01 | Phase 4 | Pending |
| GRAPH-02 | Phase 4 | Pending |
| GRAPH-03 | Phase 4 | Pending |
| GRAPH-04 | Phase 4 | Pending |
| GRAPH-05 | Phase 4 | Pending |
| GRAPH-06 | Phase 4 | Pending |
| SURF-01 | Phase 5 | Pending |
| SURF-02 | Phase 5 | Pending |
| SURF-03 | Phase 5 | Pending |
| LOOP-01 | Phase 6 | Pending |
| LOOP-02 | Phase 6 | Pending |
| LOOP-03 | Phase 6 | Pending |

**Coverage:**

- v1 requirements: 23 total
- Mapped to phases: 23 ✓
- Unmapped: 0

**By phase:**

| Phase | Requirements |
|-------|--------------|
| 1. Trustworthy Event Writes | SUB-01, SUB-02, SUB-03 |
| 2. Wide Events From Every Subsystem | SUB-04, SUB-05, SUB-06, SUB-07, SUB-08 |
| 3. Relations The Event Log Cannot Carry | SUB-09, SUB-10, SUB-11 |
| 4. The Stored Relation Graph | GRAPH-01, GRAPH-02, GRAPH-03, GRAPH-04, GRAPH-05, GRAPH-06 |
| 5. The Resident Garden | SURF-01, SURF-02, SURF-03 |
| 6. The Work Loop Closes | LOOP-01, LOOP-02, LOOP-03 |

---
*Requirements defined: 2026-09-08*
*Last updated: 2026-09-08 after roadmap creation (traceability filled)*
