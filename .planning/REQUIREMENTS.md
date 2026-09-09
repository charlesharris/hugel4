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

- [ ] **GRAPH-01**: A SQLite projection is built from the event log, the pile and git
- [ ] **GRAPH-02**: The projection holds code↔code, code↔ticket, ticket↔ticket and entry↔entry relations
- [ ] **GRAPH-03**: The projection can be deleted and rebuilt by replay, and no fact exists solely inside it
- [ ] **GRAPH-04**: `go build` still produces one binary with no cgo and no server process
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
| Cassandra, Kafka or Postgres as the log | ~5 events/day against tooling for six-figure writes/sec; a live cluster would become a runtime dependency of a single-binary CLI |
| Hugel writing bd content | Lifecycle transitions only — `Close`, `HandBack`, `Release` |
| Multi-gardener / shared garden | Built for one gardener across many projects; concurrency model assumes it |

## Traceability

Every v1 requirement maps to exactly one phase. Filled during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SUB-01 | Phase 1 | Complete |
| SUB-02 | Phase 1 | Complete |
| SUB-03 | Phase 1 | Complete |
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
