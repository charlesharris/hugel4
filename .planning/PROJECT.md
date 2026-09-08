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
- ✓ Beads integration, read-only — bd is the source of truth for work; hugel never writes it — existing
- ✓ Redact — credential filtering before material reaches the pile — existing

### Active

- [ ] **`hugel garden`** — a single entry point that starts the TUI and is the thing you log in and sit in front of
- [ ] **Beds pane** — overview of every project: what is running, what landed, what is blocked
- [ ] **Pile pane** — the state of accumulated knowledge across all beds, not one project's slice
- [ ] **Attention pane** — what currently needs a human: tenders blocked on a decision, gate outcomes that refused, entries worth judging
- [ ] **Structural relations in the pile** — edges between parts of code, and between code and tickets, so the pile knows shape and not only prose
- [ ] **GSD as the work loop** — the discuss → plan → execute cycle drives the work; hugel supplies context in and captures findings out
- [ ] **Findings written as work proceeds** — the pile updated during a session, not only at compost time afterwards
- [ ] **Coordinator ↔ tender relay** — subagents spun off to do agreed work in isolation; the coordinator bubbles up only what genuinely needs the gardener and relays the answer back to the tender that asked

### Out of Scope

- **An unbounded review queue** — a backlog of hundreds is not judged, it is abandoned. Recorded twice as a deliberate refusal (`aa4d8fb7`, `2a841ffd`). The attention pane above must be bounded by live work, not by pile size.
- **Edges that wait on a human to notice a relationship** — measured at zero. `supersedes` is reachable only through `pile review --superseded-by` and has never fired once across 289 entries (`2b9d936f`).
- **LLM edge inference at extraction time** — built once already and left nothing behind; the entries describing it survive only as legacy-import markdown (`680d9417`).
- **Hugel writing to bd** — bd stays the source of truth for work; hugel reads it.
- **A server, or any external database** — single binary, local files, git for versioning.

## Context

**Current state.** Every subsystem in the Validated list above is built and in
use. The gap between what exists and the vision is not the engine — it is that
there is no single surface over it, the pile holds no structural relations, and
tenders run one-way.

**What the pile already knows about this vision.** Three findings bear on it
directly and were drawn from hugel's own pile:

1. `hugel tend` is already a working surface bounded by time, with per-group
   caps, built specifically so a scrolling list could not become an inbox. The
   garden TUI's pile pane is closer to an evolution of `tend` than a new thing.
2. The pile has 289 entries and **zero edges of any kind** (`a61ed4f8`).
   `Link{Rel,ID}` has existed since the pile was built with four declared rel
   types and not one entry carries a link. Any edge design starts from nothing
   written, not from a graph needing tidying.
3. The one edge that has ever been made to work was made to work *mechanically*:
   a revert links to the decision it falsifies because git writes the original
   subject, quoted, into the revert's own subject — the join is the title
   (`6b8b27ac`). Derivable edges get written. Edges needing a person or a model
   at extraction time do not.

**The distinction the attention pane turns on.** The refused inbox is a backlog
of accumulated pile entries — unbounded, stale, growing with the corpus. What
this vision calls an inbox is mostly *live*: tenders blocked right now, gate
refusals from this session. That set is bounded by concurrent work, not by pile
size, and does not obviously violate the recorded refusal. Keeping the two apart
is the design problem; letting them share a pane is how the refusal gets undone
by accident.

## Constraints

- **Token economy**: Every token of soil that enters a session is re-sent on every later turn — cost is set by how much enters and how early, not by how much the pile holds. Any always-on TUI context must answer to this.
- **Tech stack**: Go 1.26, Charmbracelet (Bubbletea/Lipgloss), no ORM, no config library, no server — token-efficiency and single-binary distribution are design goals, not incidental.
- **External tools**: `git` and `tmux` required; `bd` and `claude` optional at the boundaries.
- **Irreversibility**: Landing is the one step that cannot be undone by deleting a directory — every stage before it is built to refuse.
- **Provenance**: Entries, sessions and landings are immutable facts; review and status are mutable judgement wrapped around them. Nothing may collapse the two.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| `hugel garden` becomes the primary entry point | One surface to sit in front of; six commands that each do one thing cannot be sat in front of | — Pending |
| Attention pane scoped to live work, not accumulated backlog | Preserves the recorded refusal of an inbox while still surfacing what blocks the gardener now | — Pending |
| Edges must be mechanically derivable to be worth declaring | The only edge that ever worked joined on a title git already wrote; human- and LLM-authored edges are measured at zero | — Pending |
| GSD drives the work loop; hugel supplies context and captures findings | Avoids hugel growing a second planner alongside the one already in use | — Pending |
| Tender relay becomes bidirectional | Today tenders are detached and one-way; bubbling up a blocking question is what makes unattended work safe to leave unattended | — Pending |

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
