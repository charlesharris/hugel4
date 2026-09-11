# Roadmap: Hugel

## Overview

**Active milestone: v0.2 The Shared Garden — Phases 7 through 11.**

v0.1 proved one thing and left the rest standing: `events.Emit` now fails loudly
and durably, so silence in the log means nothing ran rather than that writes have
been failing unseen. v0.2 takes that guarantee and moves the ground underneath it.
The garden stops being files under one home directory and becomes a shared Postgres
database — events, draws and the pile, content included — reachable from several
machines, with an actor id on every row from the first migration.

Two things are engineered rather than hoped for. Writes never block on the network:
they land fsynced on the gardener's own disk and drain when the database is
reachable, so a gate on a plane still records. And losing the garden is engineered
*against and proven against* — exported, restored into an empty database, and the
restore actually run, because a backup that has never been restored is not a backup.
That is the reversal this milestone rests on: every graph hugel ever kept died with
its store, and the lesson drawn at the time was to avoid stores that can die. The
right lesson was that a durability guarantee nobody exercises is not a guarantee —
the same thing phase 1 learned about tests that pass with the code deleted.

The relation graph lands last, on Apache AGE over the same Postgres. It is a
projection: droppable, replayable from the tables and git, holding no fact that
exists nowhere else.

## Phases

**Phase Numbering:**

- Integer phases (7, 8, 9): Planned milestone work
- Decimal phases (7.1, 7.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.
Phases 1–6 belong to milestone v0.1 and are recorded under *Prior Milestone* below.

- [ ] **Phase 7: A Queue The Network Cannot Stop** - Writes land fsynced on local disk and drain when Postgres is reachable, and a test can never reach a real garden
- [ ] **Phase 8: The Event Log And The Draw Log Move Into Postgres** - Rows that refuse to be edited, without giving up `grep` or one-bad-line-costs-one-line
- [ ] **Phase 9: The Pile Moves, And Its History Is Replaced Rather Than Lost** - Entries become rows that rank without a file, bead ids join everything, and the migration is provable
- [ ] **Phase 10: A Restore That Has Actually Been Run** - Export, rebuild into an empty database, and a drill that is exercised rather than assumed
- [ ] **Phase 11: The Relation Graph, On Apache AGE** - A droppable, replayable property graph over the tables and git, reaching the draw path

## Phase Details

### Phase 7: A Queue The Network Cannot Stop

**Goal**: A gardener's writes are accepted, fsynced and acknowledged on their own
disk before any database hears about them, and drain when it is reachable — with a
stall that is named rather than silent, and none of it testable against a real garden

**Depends on**: Phase 1 (v0.1) — the durable append, sticky failure marker and
`HealthOf` surface this queue generalises rather than reinvents

**Requirements**: STORE-02, STORE-03, STORE-10, QUEUE-01, QUEUE-02, QUEUE-03, QUEUE-04, QUEUE-05

**Success Criteria** (what must be TRUE):

  1. With Postgres unreachable — wrong host, refused connection, a machine with no network at all — `hugel gate` and `hugel tender` run to completion at the speed they run today, and the writes they made are fsynced to local disk before the command returned
  2. Once the database is reachable the backlog lands, and a record delivered twice is stored once; ids are minted on the machine that emitted them, so a backlog written offline keeps the position it was created in rather than being stamped with the drain time, and every row names the gardener and the machine that wrote it
  3. A drain that has stopped is named in `hugel yield --health` — when it stopped, and how much is waiting behind it — and breaking the drain on purpose turns that answer red instead of leaving it confidently green
  4. A gardener reading state on the machine that just wrote sees their own writes, drained or not; one machine's reads never appear to go backwards relative to its own writes, and cross-machine order is not claimed anywhere
  5. `go test ./...` passes on a machine with no Postgres and no garden, and a test that reaches for the gardener's real garden or a shared database is refused by construction — the way `config.Sandbox()` already refuses — rather than trusted not to

**Notes**: The `internal/store` seam (pool, `goose` migrations, the AGE
`AfterConnect` session plumbing, a `Tx` helper) and the schema-per-run test harness
are enabling work inside this phase, not a phase of their own. STORE-10 is the
hard gate: there is no CI here, and until the database-era equivalent of
`config.Sandbox()` exists nothing after this can be honestly tested. It lands
first *within* this phase. STORE-02 and STORE-03 are here rather than with the
conversions because an idempotency key and an actor id retrofitted onto rows
already written is a second migration.

**Plans**: TBD

### Phase 8: The Event Log And The Draw Log Move Into Postgres

**Goal**: Events and draws stop being files under one home directory and become rows
the database itself refuses to edit — without giving up the two things the files
were quietly good at: one bad line costing one line, and `grep` answering "what
happened Tuesday" with everything else broken

**Depends on**: Phase 7 (nothing converts before the queue exists, or the write path
gets built twice and every gate is network-dependent in the meantime)

**Requirements**: STORE-01, STORE-04, STORE-08, STORE-09

**Success Criteria** (what must be TRUE):

  1. A gardener who tries to update or delete an event is refused by the database rather than by convention, and every event carries an ingest sequence answering "when did the database learn this", distinct from the emit-time id answering "when was this made"
  2. A soil draw is a row naming the entry ids it delivered, not a count — so draw precision stays computable from what was stored, which is the one property the draw log already exists to protect
  3. A malformed record costs that record and nothing else — in the one-shot import and in the ongoing drain alike — and every skipped record is logged with enough of itself for a gardener to reconstruct it by hand
  4. A gardener can read and search recent events with no database client and no reachable database, and what they see includes the writes still sitting undrained in the local queue
  5. Nothing writes `events.jsonl` or `draws.jsonl` any more; the history that was in them is in Postgres, reconciled against the files it came from, and the files are kept as an archive rather than deleted

**Notes**: STORE-08 and STORE-09 are here, in the same phase that removes the
file-backed version they replace, because they are preservation guarantees — one
bad record costs one record; the log stays readable without a database client — and
deferring either means accepting the loss rather than scheduling it. A bulk
`INSERT` in one transaction does not preserve either. The importer's cross-machine
merge rule (file position is not an ordering authority once there is more than one
source file) must be written down and its clock-skew failure mode accepted
explicitly, not assumed by concatenation order. Whole-garden migration proof is
Phase 9's STORE-07; this phase only claims the two logs it moves.

**Plans**: TBD

### Phase 9: The Pile Moves, And Its History Is Replaced Rather Than Lost

**Goal**: Pile entries — content included — become rows that rank without touching a
file, bead ids join them to everything else, and the whole file-backed garden's
migration is provable in one report a gardener can read

**Depends on**: Phase 8

**Requirements**: STORE-05, STORE-06, STORE-07

**Success Criteria** (what must be TRUE):

  1. A soil draw ranks and returns entries without opening a file, with the token budget still exactly enforced and the per-entry cap unchanged
  2. The review history a gardener could read as a `git diff` is still readable after the move — who changed what, when, and from what to what — because its replacement shipped with the migration rather than being regretted after it
  3. A gardener can ask what a bead touched and reach its events, entries and paths in one query, while `bd` still owns the beads themselves and hugel still writes it nothing but `Close`, `HandBack` and `Release`
  4. The whole file-backed garden — every event, every draw and all 316 entries — is in Postgres, and the gardener reads one report proving what landed matches what was there, rather than being told the import exited zero
  5. Before any converging write runs, the gardener sees a collision report: which entries from different machines' pile histories would land on the same scope + type + normalised title, and what the import intends to do about each

**Notes**: The hardest of the three conversions, and the one research insisted
should not be bundled with the others. Decision already taken and recorded in
PROJECT.md: pile *content* moves into Postgres, so the append-only audit/history
table that replaces git-diffable review history must exist from day one of this
phase — bolting provenance on later leaves everything before that point
unrecoverable, which is exactly the class of loss this milestone exists to prevent.
Converging-write keys designed for one serialized store can collide once several
machines' histories arrive in one table, hence criterion 5 as a hard pre-condition.
An optimistic version check belongs on the pile — it is the one mutable store and
therefore the only real cross-machine write hazard.

**Plans**: TBD

### Phase 10: A Restore That Has Actually Been Run

**Goal**: The garden can be exported, rebuilt from that export into an empty
database, and the rebuild verified by asking the restored garden questions — on a
cadence, so the answer ages out rather than staying green forever

**Depends on**: Phase 9 (there is nothing worth drilling a restore of while half the
garden is still in files)

**Requirements**: SURV-01, SURV-02, SURV-03, SURV-04

**Success Criteria** (what must be TRUE):

  1. One command writes the whole garden — events, draws, entries and the relations between them — to a portable file the gardener can carry off the machine
  2. That file rebuilds the garden into an empty database, and the rebuilt garden answers the same questions with the same answers as the one it came from
  3. The restore is judged by querying what came back — row counts, contents, and a live Cypher query that actually returns rows — rather than by the restore command exiting zero and the dump file looking a plausible size
  4. `hugel yield --health` says when the restore was last exercised, says plainly that it never has been when it never has, and goes stale past a maximum age instead of reporting an answer from six months ago as current
  5. Breaking the restore on purpose turns that health answer red — the check is proven able to fail before it is trusted, the same way phase 1's health check had to be

**Notes**: Sequenced before the graph deliberately. AGE carries its own restore
hazards — a `pg_dump`/`pg_restore` can succeed while leaving the graph unqueryable
— so the drill needs to already be running by the time the graph holds anything,
and criterion 3's live Cypher check is what catches that class of silent success.
Two things to decide and record here: whether the AGE graph is restored via
`pg_dump` or rebuilt by drop-and-replay from the base tables (replay is the
recommendation, since it is a projection by design), and whether tooling should
refuse `pg_upgrade` outright when AGE is present. The `pg_upgrade`/AGE
incompatibility is a MEDIUM-confidence, partly vendor-derived claim — re-verify it
against `github.com/apache/age`'s own documentation before building the refusal.

**Plans**: TBD

### Phase 11: The Relation Graph, On Apache AGE

**Goal**: The structure implicit in the event, draw and entry tables and in git
becomes a property graph a gardener can question in Cypher — droppable, replayable,
holding no fact that exists nowhere else — and it reaches the draw path

**Depends on**: Phase 9 (the tables it is projected from) and Phase 10 (a restore
drill already exercising AGE's hazards before the graph holds anything)

**Requirements**: GRAPH-01, GRAPH-02, GRAPH-03, GRAPH-04, GRAPH-05, GRAPH-06

**Success Criteria** (what must be TRUE):

  1. A gardener asks which beads touched a directory, and which entries relate to a bead, and gets the answer from a graph query rather than a full scan of the log
  2. The graph holds code↔code, code↔ticket, ticket↔ticket and entry↔entry edges, and every edge can be traced back to the durable row or commit it was derived from
  3. A gardener can drop the whole graph — or the database — rebuild it by replay from the event, draw and entry tables and git, and lose nothing; no fact exists solely inside it
  4. `go build` still produces one binary with no cgo, and with the Postgres + AGE server down or deleted every write path still records; only graph queries are unavailable, and nothing else degrades
  5. A draw surfaces an entry reached through structure that wording alone would have missed, measured against the edges that actually exist rather than against a hoped-for population, with the token budget still exactly enforced

**Known risk — GRAPH-06's payoff is gated on work this milestone does not contain.**
Structure-aware soil ranking is the reason for building AGE at all, and its value
depends on non-trivial edges existing. The substrate-widening requirements that
would produce them are deferred out of v0.2: SUB-04 through SUB-08 (one wide event
per unit of work from every subsystem, carrying ids rather than counts) and SUB-09
(stop discarding bd's ticket↔ticket dependency edges at the `beads.Bead`
boundary). So the edges available to this phase are only the ones already derivable
today — code↔code from git via `cochange`, code↔ticket and entry↔bead from the
`Paths` and `Beads` that entries and events already carry, ticket↔ticket read from
`bd` at projection time rather than through `beads.Bead`, and entry↔entry from
shared beads and paths plus the one mechanical join this project has ever made work
(a revert's own subject naming the decision it falsifies). GRAPH-06 is built and
proven against those. It will not be *worth much* until the substrate widens, and
the honest outcome of criterion 5 may be a thin result. Record that as a
measurement of the current substrate, not as a failure of the approach — and do not
let it become an argument for human-asserted or LLM-inferred edges, both of which
are measured failures already on this project's record.

**Also carried here**: benchmark any Cypher query before it ships into the draw
path — a query tested against a near-empty graph hides a cliff that appears once
edges exist. AGE's openCypher coverage is a dated snapshot; re-check it against the
version actually pinned, and pin the server to Postgres 18 (AGE supports PG 11–18)
rather than following Postgres to 19.

**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 7 → 8 → 9 → 10 → 11

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 7. A Queue The Network Cannot Stop | 0/TBD | Not started | - |
| 8. The Event Log And The Draw Log Move Into Postgres | 0/TBD | Not started | - |
| 9. The Pile Moves, And Its History Is Replaced Rather Than Lost | 0/TBD | Not started | - |
| 10. A Restore That Has Actually Been Run | 0/TBD | Not started | - |
| 11. The Relation Graph, On Apache AGE | 0/TBD | Not started | - |

## Prior Milestone: v0.1

Phases 1–6 were defined for milestone v0.1. Only Phase 1 was executed. The full
v0.1 roadmap text is preserved verbatim at
`.planning/milestones/v0.1-phases/ROADMAP-v0.1.md`; Phase 1's plans, summaries,
review, UAT and verification are at
`.planning/milestones/v0.1-phases/01-trustworthy-event-writes/`.

| Phase | Requirements | Disposition |
|-------|--------------|-------------|
| 1. Trustworthy Event Writes | SUB-01, SUB-02, SUB-03 | Executed — 5/5 plans, verified 4/4 must-haves at `human_needed`; requirements reverted to *Gaps Found* pending a real-gate dogfood of `--health` (`hugel4-vvd`) |
| 2. Wide Events From Every Subsystem | SUB-04 … SUB-08 | Not executed — carried forward, unscheduled. Blocks GRAPH-06's payoff (see Phase 11's known risk) |
| 3. Relations The Event Log Cannot Carry | SUB-09, SUB-10, SUB-11 | Not executed — carried forward, unscheduled |
| 4. The Stored Relation Graph | GRAPH-01 … GRAPH-06 | **Superseded by Phase 11.** The projection moved from SQLite to Postgres + Apache AGE (decided 2026-09-11) and the work moved into v0.2 |
| 5. The Resident Garden | SURF-01, SURF-02, SURF-03 | Not executed — carried forward, unscheduled |
| 6. The Work Loop Closes | LOOP-01, LOOP-02, LOOP-03 | Not executed — carried forward, unscheduled |
