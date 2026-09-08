# Roadmap: Hugel

## Overview

Hugel already has the engine: the pile, soil, tender, gate, garden, yield and
cochange all work. What it does not have is a record. The event log holds 32
events from two subsystems and stopped being written a week ago, the pile holds
316 entries and zero edges, and `events.Record` swallows every error so a stale
log cannot be told apart from a failing one. This milestone works outward from
that fact: first make writes trustworthy, then widen what is written, then carry
the structural context events alone cannot hold — and only then project a graph
over the substrate, because a graph built before the widening would be very
nearly empty. The surface and the loop follow, resting on the same substrate but
independent of the graph.

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Trustworthy Event Writes** - `events.Record` fails loudly and durably, so silence in the log means nothing ran
- [ ] **Phase 2: Wide Events From Every Subsystem** - One wide event per unit of work, carrying ids rather than counts
- [ ] **Phase 3: Relations The Event Log Cannot Carry** - bd's dependency edges kept, and the ephemeral middle written at the boundary that knows it
- [ ] **Phase 4: The Stored Relation Graph** - A droppable, replayable SQLite projection that is queryable and reaches the draw path
- [ ] **Phase 5: The Resident Garden** - `hugel garden` becomes the surface that is logged into and left open
- [ ] **Phase 6: The Work Loop Closes** - GSD drives discuss → plan → execute with hugel on both ends, and a gardener's answer returns to the session that asked

## Phase Details

### Phase 1: Trustworthy Event Writes

**Goal**: A gardener can trust the event log — silence means nothing ran, not that writes have been failing unseen
**Depends on**: Nothing (first phase)
**Requirements**: SUB-01, SUB-02, SUB-03
**Success Criteria** (what must be TRUE):

  1. With the event file made unwritable (read-only directory, bad `HUGEL_HOME`, full disk), the gardener sees the failure reported by the command that emitted the event, instead of the command reporting success
  2. An event written by a command that has returned survives a crash — the tail of the log is durable, not sitting in a buffer
  3. A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing "nothing has run since 2026-09-01" from "writes have been failing since 2026-09-01", without reading the code
  4. A failing event write is visible but never destroys the work it was instrumenting — gate and tender still complete, and the gardener is told

**Plans**: 5 plans

Plans:
**Wave 1**

- [ ] 01-01-PLAN.md — `events.Emit` returns its failure, and a gate reports it without failing (tracer)

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 01-02-PLAN.md — the remaining eight call sites check and report: gate, tender, handback

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 01-03-PLAN.md — durable writes: a checked `Sync`, `Timer.Done`, and the docs that described the old contract

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 01-04-PLAN.md — the sticky first-failure marker and `events.HealthOf`

**Wave 5** *(blocked on Wave 4 completion)*

- [ ] 01-05-PLAN.md — `hugel yield --health`, its completion entry, and package `cli`'s first tests

### Phase 2: Wide Events From Every Subsystem

**Goal**: Every subsystem that does a unit of work leaves one wide event behind, carrying the ids a relation can later be derived from
**Depends on**: Phase 1 (a widened log is only worth writing once writes are trustworthy)
**Requirements**: SUB-04, SUB-05, SUB-06, SUB-07, SUB-08
**Success Criteria** (what must be TRUE):

  1. After a day of ordinary use, the gardener finds events from compost, soil, spike, dispatch, pile review, tender and gate in `events.jsonl` — one per unit of work, not the two subsystems recorded today
  2. A soil draw's event names the entry ids delivered, so the context a session was given can be recovered from the log alone
  3. A handback event names the bead, the reason and the session that gave up
  4. A gardener can pick any event out of the log and follow its ids — bead, bed, session, sha, entry ids, paths — to the things it refers to, rather than finding a count that cannot be followed anywhere

**Plans**: TBD

### Phase 3: Relations The Event Log Cannot Carry

**Goal**: The structural context that outlives a session — bd's ticket edges and what a working tender knew — is durable and re-readable after the tmux session is gone
**Depends on**: Phase 2
**Requirements**: SUB-09, SUB-10, SUB-11
**Success Criteria** (what must be TRUE):

  1. A bead read through hugel exposes the same ticket↔ticket dependency edges bd knows, instead of collapsing them into `Ready bool`, and readiness is still bd's answer rather than one hugel recomputed
  2. A bead's relations are still derivable a week after the session that worked it has ended and its worktree is gone
  3. Tender progress, coordinator decisions, spike findings and gate refusals are readable after the tmux session dies, because they were written at the boundary that knew them rather than reconstructed afterwards
  4. Everything hugel writes back to bd is still one of the three lifecycle transitions — `Close`, `HandBack`, `Release` — and never content

**Plans**: TBD

### Phase 4: The Stored Relation Graph

**Goal**: The structure implicit in the logs, the pile and git becomes queryable — and reaches the draw path, so a session gets context chosen by structure as well as by wording
**Depends on**: Phase 3 (the substrate must carry the facts before anything is projected from it)
**Requirements**: GRAPH-01, GRAPH-02, GRAPH-03, GRAPH-04, GRAPH-05, GRAPH-06
**Success Criteria** (what must be TRUE):

  1. A gardener can ask which beads touched a directory, and which entries relate to a bead, and get the answer without a full scan of the logs
  2. The projection holds code↔code, code↔ticket, ticket↔ticket and entry↔entry relations, and each edge can be traced back to the durable source line it was derived from
  3. A gardener can delete the SQLite file outright, rebuild it by replay from events, entries and git, and lose nothing — no fact exists solely inside the projection
  4. `go build` still produces one binary, with no cgo and no server process to run
  5. A draw surfaces entries reached through structure that wording alone would have missed, with the token budget still exactly enforced

**Plans**: TBD

### Phase 5: The Resident Garden

**Goal**: `hugel garden` stops being a one-shot render and becomes the screen that is logged into and left open all day
**Depends on**: Phase 3 (a resident surface can only reflect progress that is being written)
**Requirements**: SURF-01, SURF-02, SURF-03
**Success Criteria** (what must be TRUE):

  1. `hugel garden` stays on screen until the gardener leaves it, rather than rendering once and exiting
  2. A tender starting, progressing, landing or handing back appears in an already-open garden without restarting it
  3. The attention list still shows exactly the beads bd labels `needs-attention`, and nothing else has become a source for it
  4. Leaving the garden open all day costs no soil tokens beyond what the one-shot render already cost — refreshing the surface does not re-send context

**Plans**: TBD
**UI hint**: yes

### Phase 6: The Work Loop Closes

**Goal**: GSD drives discuss → plan → execute with hugel supplying context in and capturing findings out, and a gardener's answer reaches the session that asked for it
**Depends on**: Phase 5 (the open garden is where a handback is seen and answered)
**Requirements**: LOOP-01, LOOP-02, LOOP-03
**Success Criteria** (what must be TRUE):

  1. A unit of work runs discuss → plan → execute under GSD with hugel supplying the soil at the start and capturing the findings at the end, and no second planner has grown inside hugel
  2. A finding recorded partway through a session is in the pile before that session ends, not only after compost runs later
  3. A gardener answering a handed-back bead sees that answer arrive in the tender session that asked, rather than a fresh tender picking it up from bd notes afterwards

**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Trustworthy Event Writes | 0/5 | Not started | - |
| 2. Wide Events From Every Subsystem | 0/TBD | Not started | - |
| 3. Relations The Event Log Cannot Carry | 0/TBD | Not started | - |
| 4. The Stored Relation Graph | 0/TBD | Not started | - |
| 5. The Resident Garden | 0/TBD | Not started | - |
| 6. The Work Loop Closes | 0/TBD | Not started | - |
