# Hugel

## What This Is

Hugel is a single-binary Go CLI and TUI for running agentic development work and
keeping what it learns. Sessions are composted into a shared pile of typed
knowledge entries; that pile is drawn back out, token-budgeted, as context for
the next piece of work. The garden is the surface you sit in front of: the state
of your projects (beds), the state of accumulated knowledge (the pile), and
whatever is waiting on a human decision.

It is built for one gardener working across many projects, and it is instrumented
so the cost of the context it delivers is always visible against the work that
context produced.

## Core Value

Work done by agents leaves behind why it was done that way — and that record is
cheap enough to deliver back into the next session that it actually gets used.

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

**Substrate — record what is currently ephemeral**

- [ ] **Widen event emission** — one wide event per unit of work from every subsystem, not two. Today `~/.hugel/events.jsonl` holds 32 events, all from `gate.*` and `tender.start`, last written 2026-09-01. Nothing from compost, soil, spike, dispatch, review, land or handback.
- [ ] **Stop discarding bd's dependency graph** — `beads.Bead` collapses dependencies, defer dates and gates into `Ready bool`. bd knows the ticket↔ticket edges; hugel drops them at the boundary. Carry them without recomputing readiness.
- [ ] **Enrich what a bead carries** — the structural context a graph needs must live somewhere durable and re-readable, in bd or beside it.
- [ ] **Record the ephemeral middle** — coordinator decisions, tender progress, spike findings and gate refusals are lost when the tmux session dies. Whatever the graph should know about them has to be written at the boundary that knows it.

**Graph — projected over the substrate**

- [ ] **Stored relation graph** — a queryable graph of code↔code, code↔ticket, ticket↔ticket and entry↔entry relations. Stored, on the explicit understanding that it is a projection: droppable and rebuildable by replaying events, entries and git, so a migration has nothing irreplaceable to lose.
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
- **A server, or any external database** — single binary, local files, git for versioning.

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
- **The graph is rebuildable or it is not kept**: Storing it is conditional on being able to drop and replay it from events, entries and git.
- **Tech stack**: Go 1.26, Charmbracelet (Bubbletea/Lipgloss), no ORM, no config library, no server.
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
| Substrate widening precedes the graph | 32 events from two subsystems and a discarded bd dependency graph would derive nothing | — Pending |
| GSD drives the work loop; hugel supplies context and captures findings | Avoids hugel growing a second planner alongside the one already in use | — Pending |

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
*Last updated: 2026-09-08 after initialization*
