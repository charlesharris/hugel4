# Hugel

## What This Is

Hugel is a single-binary Go CLI and TUI for running agentic development work and
keeping what it learns. Sessions are composted into a shared pile of typed
knowledge entries; that pile is drawn back out, token-budgeted, as context for
the next piece of work. The garden is the surface you sit in front of: the state
of your projects (beds), the state of accumulated knowledge (the pile), and
whatever is waiting on a human decision.

It is built for one gardener working across many projects and, as of v0.2, across
many machines: the garden lives in a shared Postgres database rather than in files
under one home directory. It is instrumented so the cost of the context it
delivers is always visible against the work that context produced.

## Core Value

Work done by agents leaves behind why it was done that way — and that record is
cheap enough to deliver back into the next session that it actually gets used.

## Current Milestone: v0.2 The Shared Garden

**Goal:** Move the garden's substrate from files under one home directory into a
shared Postgres database — and engineer, rather than avoid, the guarantee that it
cannot be lost.

**Target features:**
- Events, draws and the pile live in Postgres; bd stays authoritative for beads,
  with bead ids carried in Postgres so relations reach everything else
- Relations queried as a property graph (Apache AGE) rather than hand-rolled joins
- One garden reachable from several machines, with an actor id on every row so
  team use is a configuration change and not a migration
- Writes never block on the network: a durable local queue accepts them and drains
  when the database is reachable
- Losing the garden is engineered against and *proven* against — a restore that
  has never been performed is not a backup

**The reframe this milestone rests on.** Every graph hugel kept previously died
with the store it lived in, and the conclusion drawn at the time was to avoid
stores that can die. That was the wrong lesson to draw. The right one is that a
durability guarantee nobody exercises is not a guarantee — which is the same thing
phase 01 learned about tests that pass whether or not the code is there. So this
milestone keeps the structure and engineers the survival: export, backup, replay,
and a restore drill that actually runs.

## Requirements

### Validated

<!-- Inferred from the committed codebase map; shipped and relied upon. -->

- ✓ Pile store — typed entries (Decision, Pattern, Discovery, Failure, Constraint), git-backed, converging writes keyed on scope + type + normalised title — existing
- ✓ Compost / digest — transcript → entries via a free heuristic extractor — existing
- ✓ Soil — per-query lexical ranking with an exactly-enforced token budget and per-entry cap — existing
- ✓ `hugel pile review` — a human setting standing on an entry; soil ranks vouched entries 1.4x — existing
- ✓ `hugel tend` — Bubbletea surface for judging entries, bounded by time with per-group caps — existing
- ✓ Tender — detached tmux sessions in per-bead git worktrees, brief generated from soil — existing
- ✓ Gate — work, test, review, test, commit, merge, push, close — existing
- ✓ Yield / survival / draws — cost accounting, landing grades, revert detection — existing
- ✓ Beads integration — bd is the source of truth for work; hugel writes only three lifecycle transitions (`Close`, `HandBack`, `Release`), never content — existing
- ✓ `hugel garden` — one screen across every bed (in flight / ready / blocked), Tab to the knowledge side; same surface, same sitting — existing
- ✓ Attention routing — `HandBack` releases a claim, labels `needs-attention` and appends what the tender learned, in one bd invocation; `Queue` then refuses that bead to any tender — existing
- ✓ Cochange — code↔code coupling derived from git on demand, computed and never stored — existing
- ✓ Entry metadata — every entry carries `Paths` and `Beads`, populated at extraction; code↔entry and ticket↔entry joins already exist — existing
- ✓ Events / draws — append-only wide-event log and draw log, both storing ids rather than counts — existing, barely populated
- ✓ Redact — credential filtering before material reaches the pile — existing

### Active

Ordered by dependency. The substrate has to carry the facts before anything can
be derived from it, and almost nothing is recorded today.

**Substrate — move the garden into Postgres (v0.2)**

- [ ] **Events, draws and the pile become tables** — including pile *content*, not only its metadata. Decided 2026-09-11: entries become rows so ranking and joins are SQL rather than a boundary crossing. Git-diffable knowledge history does not survive that by default, and its replacement ships in the same phase as the migration rather than being regretted after it. The draw log's shape is the thing to preserve: it stores the ids a draw delivered rather than a count, which is the only reason draw precision is computable at all.
- [ ] **UUIDv7 identity, minted at emit** — one id per record, generated on the machine that emits it, carrying creation order and doubling as the idempotency key. Ordering survives the write queue: a backlog written offline keeps its creation position, where a server-assigned sequence would stamp it with the drain time. A server sequence runs alongside for ingest order, and the actor id carries provenance, since UUIDv7 has no node field by design.
- [ ] **A durable local write queue** — writes are accepted, fsynced and acknowledged locally, then drained to Postgres when it is reachable. `hugel gate` and `hugel tender` never block on a network, and a stalled drain is visible rather than silent. Built on phase 01's `Emit`/failure-marker/`HealthOf` mechanism rather than a second one invented beside it.
- [ ] **Bead ids in Postgres, bd still authoritative** — `bd` keeps owning issues in Dolt; Postgres carries bead ids so relations between beads, events, entries and code live in one queryable place. Hugel does not become a second issue tracker.
- [ ] **An actor id on every row** — one gardener today, several machines. Attribution is carried from the first migration so multi-gardener use is configuration rather than a schema rewrite, and a machine with a drifting clock is detectable rather than merely disruptive.
- [ ] **Export, backup and a restore that is actually run** — the garden is exportable to a portable format, restorable from it, and the restore exercised on a schedule. The engineered answer to the regret that every graph hugel kept died with its store.
- [ ] **Migrate what exists** — the current events log, 22 draw records and the pile's entries move across without loss, with a way to verify the import was faithful. The old files are retained as an archive rather than deleted.
- [ ] **A test harness that can hold a database** — `config.Sandbox()` panics if a test resolves the garden outside a temp dir, and that guarantee has no database equivalent yet. Nothing above can be honestly tested until it does.

**Substrate — record what is currently ephemeral**

- [x] **Make event writes fail loudly** — done in phase 01 (v0.1). `Emit` returns its error at all ten production call sites, fsyncs before returning, and `yield --health` distinguishes "nothing has run" from "writes have been failing" across every filesystem state including a full disk. Carried forward rather than retired: this is what makes the v0.2 local write queue trustworthy.
- [ ] **Widen event emission** — one wide event per unit of work from every subsystem, not two. Today `~/.hugel/events.jsonl` holds 32 events, all from `gate.*` and `tender.start`, last written 2026-09-01. Nothing from compost, soil, spike, dispatch, review, land or handback.
- [ ] **Stop discarding bd's dependency graph** — `beads.Bead` collapses dependencies, defer dates and gates into `Ready bool`. bd knows the ticket↔ticket edges; hugel drops them at the boundary. Carry them without recomputing readiness.
- [ ] **Enrich what a bead carries** — the structural context a graph needs must live somewhere durable and re-readable, in bd or beside it.
- [ ] **Record the ephemeral middle** — coordinator decisions, tender progress, spike findings and gate refusals are lost when the tmux session dies. Whatever the graph should know about them has to be written at the boundary that knows it.

**Graph — projected over the substrate**

- [ ] **Stored relation graph, in Postgres + Apache AGE** — a queryable graph of code↔code, code↔ticket, ticket↔ticket and entry↔entry relations, projected from the JSONL logs, the pile and git. Droppable and rebuildable by replay, so a migration has nothing irreplaceable to lose. Queried as a property graph in Cypher rather than as recursive CTEs over an edges table, which is what variable-depth traversal actually wants. `pgx` is pure Go, so hugel's own binary stays cgo-free — but a Postgres server with the AGE extension becomes a runtime requirement for graph queries. Decided 2026-09-11; supersedes the SQLite projection. See the decision log.
- [ ] **Graph in the draw path** — soil ranking consults structure, not only wording. `cochange` already proves the shape of this.

**Surface**

- [ ] **Session-persistent garden** — `hugel garden` renders once and exits today. It should stay resident: refresh as tenders progress, land and hand back, and be the thing that is logged into and left open.

**Loop**

- [ ] **GSD drives the work loop** — discuss → plan → execute; hugel supplies context in and captures findings out, rather than growing a second planner.
- [ ] **Findings written as work proceeds** — the pile updated during a session, not only at compost time afterwards.
- [ ] **Human → tender return leg** — the tender → human handback is built (`HandBack` + `Queue` refusal + attention-first ordering). The answer currently returns as bd notes picked up by a *fresh* tender. Relay it back into the session that asked.

### Out of Scope

- **An unbounded review queue** — a backlog of hundreds is not judged, it is abandoned. Recorded twice as a deliberate refusal (`aa4d8fb7`, `2a841ffd`).
- **Non-bead sources in the attention list** — anything needing the gardener becomes a bead carrying `needs-attention`. One source of truth, and the no-inbox refusal is preserved structurally rather than by discipline.
- **Edges asserted by a human** — measured at zero across 289 entries (`2b9d936f`). An edge that waits on a person to notice a relationship gets no edges.
- **LLM edge inference at extraction time** — built once already and left nothing behind (`680d9417`).
- **A graph that cannot be rebuilt** — every graph hugel has kept died with the store it lived in (`internal/cochange/cochange.go`). Storing one is only acceptable while it remains a projection over durable sources.
- **Hugel writing bd content** — lifecycle transitions only (`Close`, `HandBack`, `Release`).
- **A clustered log (Cassandra, Kafka)** — write volume is ~5 events/day against tooling built for six-figure writes/sec. Postgres was moved *out* of this bullet on 2026-09-11 and is now the substrate; the objection to clusters was never the SQL, it was operating a cluster for a handful of writes a day.
- **Losing work when the database is unreachable** — a remote substrate must not make `hugel gate` depend on a network. Writes land in a local fsynced queue and drain later; an unreachable database degrades reads, never recording.
- **Treating a backup as durable before it has been restored** — the explicit reversal of this project's own recorded regret. Every graph hugel kept died with its store, and the answer adopted here is to exercise the restore rather than to avoid the store.

## Context

**Current state.** Far more of the vision is built than the vision assumed.
`hugel garden` already renders every bed's work with Tab to the knowledge side.
The attention concept already exists, sourced from bd's `needs-attention` label,
sorted first, and withheld from tenders. `cochange` already derives code↔code
coupling from git. Entries already carry `Paths` and `Beads`. The engine is not
the gap.

**The gap is that almost nothing is recorded.** The pile holds 316 entries and
zero edges. The event log holds 32 events from two subsystems and has not been
written to in a week. The draw log holds 20 draws. A graph derived from this
substrate today would be very nearly empty — which is the finding that reorders
the roadmap: substrate first, graph second.

**What the pile already knows about this vision.** Drawn from hugel's own pile:

1. The pile has 289 entries and **zero edges of any kind** (`a61ed4f8`).
   `Link{Rel,ID}` has existed since the pile was built with declared rel types
   and not one entry carries a link. Any edge design starts from nothing written.
2. The only edge ever made to work was made to work *mechanically*: a revert
   links to the decision it falsifies because git writes the original subject
   into the revert's own subject — the join is the title (`6b8b27ac`).
3. `internal/cochange` refuses to store its graph on the grounds that every
   graph hugel kept died with its store, and the entries that survived did so
   because they were flat files. Storing a graph is a deliberate reversal of
   that position, taken with the replay requirement attached.

**Why the reversal is defensible.** Derivable-only fails today not because
derivation is the wrong model, but because the sources derive almost nothing. If
the substrate is widened so events carry the ids, and bd's dependency edges stop
being discarded at the boundary, then a stored graph is a cache over durable
append-only sources rather than an authored artifact — and a lost cache is
replayed, not mourned.

## Constraints

- **Token economy**: Every token of soil that enters a session is re-sent on every later turn — cost is set by how much enters and how early, not by how much the pile holds. A resident TUI must answer to this.
- **Substrate before projection**: No derived structure may be the only record of a fact. Events store ids rather than counts, and the draw log already proves why — store the count and the measurement does not exist.
- **The graph is rebuildable or it is not kept**: Storing it is conditional on being able to drop and replay it from the base tables — events, draws, entries, bead ids — and git. Unchanged by v0.2 except in where those facts live.
- **Postgres is the substrate; the graph is still a projection over it**: as of v0.2 the durable record is Postgres — events, draws and pile entries are tables, not files. Within it the old distinction survives intact one level down: the AGE graph is derived from those tables and nothing may exist solely in it, so it can be dropped and replayed. What changed is which layer is the floor, not the discipline.
- **The local queue is a write-ahead buffer, not a second source of truth**: writes land locally, fsynced, before the database sees them, so a gate on a dead network still records. It drains and clears; it is not a parallel log to be reconciled. Phase 01's durable append, failure marker and health surface are exactly what makes it trustworthy.
- **Nothing is kept that cannot be gotten back**: the garden must be exportable to a portable format, restorable from that export, and the restore must be exercised on a schedule rather than assumed. A backup that has never been restored is not a backup, and the previous generation of this project lost every graph it built by assuming otherwise.
- **Tech stack**: Go 1.26, Charmbracelet (Bubbletea/Lipgloss), `pgx`, Postgres with Apache AGE, no ORM, no config library. `pgx` is pure Go so the binary stays cgo-free, but a reachable database is now part of the product rather than an optional accelerator.
- **External tools**: `git` and `tmux` required; `bd` and `claude` optional at the boundaries.
- **Irreversibility**: Landing is the one step that cannot be undone by deleting a directory — every stage before it is built to refuse.
- **Provenance**: Entries, sessions and landings are immutable facts; review and status are mutable judgement wrapped around them. Nothing may collapse the two.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| `hugel garden` becomes session-persistent rather than a one-shot render | It is the thing logged into and left open; today it surveys, draws and exits | — Pending |
| The attention list stays sourced solely from bd's `needs-attention` label | One source of truth; preserves the no-inbox refusal structurally rather than by discipline | — Pending |
| The relation graph is stored, not computed on demand | Queryable persistent structure is worth the staleness cost that `cochange` refused | — Pending |
| A stored graph must be a droppable, replayable projection | Every graph hugel kept died with its store; a cache over append-only sources cannot die the same way | — Pending |
| ~~JSONL stays the log; SQLite is the projection over it~~ | Superseded 2026-09-11 by the AGE decision below. The first half stands; only the projection's store changed | — Superseded |
| JSONL stays the log; Postgres + Apache AGE is the projection over it | Property-graph queries in Cypher express code↔code, code↔ticket, ticket↔ticket and entry↔entry directly, where recursive CTEs over an edges table get worst exactly at the variable-depth traversal the graph exists for. Evaluated against SQL/PGQ in mainline Postgres first: that shipped in PG 19 beta 1 and was reverted 2026-09-07 (47 commits, "multiple design issues too late to address in this release cycle"), earliest return PG 20 ~Sept 2027, so AGE is the route available now. Accepted costs, recorded rather than discovered later: a Postgres server becomes a runtime requirement for graph queries, AGE supports PG 11–18 so the server pins below the current release, and the projection cannot be tested without CI that does not yet exist | — Pending |
| Cassandra and server-backed *logs* rejected | ~5 events/day against tooling for six-figure writes/sec, and a live cluster would become a runtime dependency of a single-binary CLI. Unchanged by the AGE decision, which touches the projection only: the log stays JSONL precisely so the server can be down or deleted without losing a fact | — Pending |
| Substrate widening precedes the graph | 32 events from two subsystems and a discarded bd dependency graph would derive nothing | — Pending |
| GSD drives the work loop; hugel supplies context and captures findings | Avoids hugel growing a second planner alongside the one already in use | — Pending |
| v0.2: Postgres becomes the substrate, not just the projection | Supersedes "JSONL stays the log". One store for events, draws, the pile and the relations between them, queryable as a property graph instead of by scanning files. Accepts that a reachable database is now part of the product | — Pending |
| The regret is answered by engineering survival, not by avoiding structure | Every graph hugel kept died with its store, and the conclusion drawn then was to avoid such stores. Restated: a durability guarantee nobody exercises is not a guarantee — the same lesson phase 01 learned about tests that pass with the code deleted. Export, backup, replay and a restore drill that runs | — Pending |
| Writes never block on the network | A remote substrate must not make a gate depend on connectivity. Local fsynced queue, drained later; phase 01's durable-append work becomes its foundation rather than being discarded | — Pending |
| bd keeps owning beads; Postgres carries bead ids only | Relations reach everything without replacing bd's Dolt storage and git-backed sync, which already works. Much smaller blast radius than migrating the tracker | — Pending |
| Shared topology, single gardener first | One garden reachable from several machines, with an actor id from the first migration so team use is configuration rather than a rewrite | — Pending |
| Pile content moves into Postgres, not just its metadata | Entries become rows, so ranking and joins are SQL rather than a boundary crossing. Accepts that git-diffable knowledge history does not survive by default and must be deliberately replaced in the same phase as the migration, not regretted after it | — Pending |
| UUIDv7 for identity, creation order and idempotency | Minted at emit time on the machine that emits. Survives the write queue: a backlog written offline keeps its creation position, where a server-assigned sequence would stamp it with the drain time instead. One id doing the job the research wanted an idempotency key for | — Pending |
| A server sequence alongside it, for ingest order | UUIDv7 answers "when was this made", which clock skew can distort; the sequence answers "when did the database learn it", which it cannot. Incremental export and sync read the sequence. Two orderings because there are genuinely two questions | — Pending |
| Provenance is a column, not a field in the id | UUIDv7 carries no node id by design — that was UUIDv1's MAC field and it was dropped deliberately. The actor id already required by the shared topology carries it, and is also what makes a machine with a drifting clock detectable rather than merely disruptive | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-09-11 after starting milestone v0.2 The Shared Garden*
