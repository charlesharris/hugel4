---
gsd_state_version: "1.0"
milestone: v0.2
current_phase: 7
current_phase_name: A Queue The Network Cannot Stop
status: planning
stopped_at: v0.2 roadmap created — Phases 7–11 written, 25/25 requirements mapped
last_updated: "2026-09-11T20:50:41.717Z"
last_activity: 2026-09-11
last_activity_desc: v0.2 roadmap created (Phases 7–11, 25 requirements mapped)
state_head: 14d8c6fab536b3805d68018470e676511e934845
progress:
  total_phases: 5
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
milestone_name: The Shared Garden
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-11)

**Core value:** Work done by agents leaves behind why it was done that way — and that record is cheap enough to deliver back into the next session that it actually gets used.
**Current focus:** Milestone v0.2 The Shared Garden — roadmap written, Phases 7–11. Next: discuss/plan Phase 7.

## Current Position

Phase: 7 — A Queue The Network Cannot Stop (not started)
Plan: —
Status: Roadmap complete, phase not yet planned
Last activity: 2026-09-11 — v0.2 roadmap created (Phases 7–11, 25 requirements mapped)

## Performance Metrics

**Velocity:**

- Total plans completed (v0.2): 0
- Average duration: —
- Total execution time: —

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 7 | 0/TBD | - | - |
| 8 | 0/TBD | - | - |
| 9 | 0/TBD | - | - |
| 10 | 0/TBD | - | - |
| 11 | 0/TBD | - | - |

**Recent Trend:**

- Last 5 plans: v0.1 Phase 01 — 16 min, 13 min, 4 min, 3 min, 4 min
- Trend: plans got faster as the phase's mechanism settled

*Updated after each plan completion*
**Per-Plan Metrics (carried from v0.1):**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 01 P01 | 16 min | 2 tasks | 4 files |
| Phase 01 P02 | 13 min | 2 tasks | 5 files |
| Phase 01 P03 | 4min | 2 tasks | 4 files |
| Phase 01 P04 | 3min | 2 tasks | 2 files |
| Phase 01 P05 | 4min | 2 tasks | 5 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap v0.2: phase numbering continues at 7 — v0.1 defined Phases 1–6 and only Phase 1 executed; the rest are recorded as carried forward or superseded rather than renumbered
- Roadmap v0.2: research's 7-phase build order compressed to 5 — the storage seam and test harness fold into the queue phase (a gate, landing first within it, not a phase of its own), and the `draws` conversion folds in with `events` (a flat record shape and one requirement does not earn a phase)
- Roadmap v0.2: `internal/pile` keeps its own phase, as research insisted — content moves, git-diffable history needs a replacement shipped alongside, and converging-write keys can collide on import
- Roadmap v0.2: STORE-08 and STORE-09 ride with Phase 8, the phase that removes the file-backed events they protect — they are preservation guarantees, and deferring one means accepting the loss rather than scheduling it
- Roadmap v0.2: STORE-02 (UUIDv7 idempotency) and STORE-03 (actor + machine) sit in Phase 7 rather than with the conversions — retrofitting either onto rows already written is a second migration
- Roadmap v0.2: STORE-07 (whole-garden migration with a readable proof) lands in Phase 9, the phase after which the sentence is true of the whole garden; Phase 8 claims only the two logs it moves
- Roadmap v0.2: survival (Phase 10) precedes the graph (Phase 11) — AGE's `pg_dump`/`pg_restore` can succeed while leaving the graph unqueryable, so the drill's live Cypher check must already be running before the graph holds anything
- Roadmap v0.2: GRAPH-06 is scheduled with the graph and flagged as a known risk — its payoff is gated on SUB-04..08, which are deferred out of this milestone
- [Phase 01]: Task 2's TDD cycle collapsed to a single test(...) commit since Emit's error-return behavior was already implemented by task 1's tracer commit
- [Phase 01]: Sync is a plain checked statement in Emit, never deferred/discarded; Timer.Done given the same error contract as Emit ahead of the next phase adopting it
- [Phase 01]: HealthOf demotes Reachable to false when the log itself cannot be stat'd for any reason other than not-exist, so a partially-unreadable garden cannot present as reachable-but-never-written
- [Phase 01]: `--health` dispatches before `transcript.LoadAll` is even called, so the answer never depends on a transcript directory

### Pending Todos

- Re-verify the `pg_upgrade`/AGE incompatibility claim against `github.com/apache/age`'s own docs before Phase 10 builds an upgrade refusal (MEDIUM confidence, partly vendor-derived)
- Re-check AGE's openCypher coverage against the version actually pinned before Phase 11 commits to query shapes (dated snapshot of `apache/age#2323`)
- Record the importer's cross-machine merge rule and its accepted clock-skew failure mode before Phase 8's importer is planned
- `.planning/.continue-here.md` was removed automatically when the v0.2 roadmap landed — its stale "do not trust ROADMAP.md" warning no longer applies

### Blockers/Concerns

- Brownfield: every subsystem in PROJECT.md's Validated list already exists and works. v0.2 moves the ground under a working Go codebase; nothing here rebuilds shipped functionality
- STORE-10 gates the whole milestone: there is no CI in this project, and `config.Sandbox()` panics if a test resolves the garden outside a temp dir. Until its database-era equivalent exists, nothing after it can be honestly tested. It lands first inside Phase 7 (bead `hugel4-d1u`)
- The recurring failure mode on this project is a guarantee that reports success while doing nothing — a test that passed with `f.Sync()` deleted, a health check that called a directory-shaped log healthy. v0.2 adds three new health signals (queue drain, restore freshness, Postgres reachability). Every one needs a "break it and confirm the check turns red" test, not a happy-path test
- AGE supports PG 11–18, pinning the server below the current Postgres release (bead `hugel4-jq7`)
- `err` shadowing at `gate/run.go:109,197` and `tender/start.go:117` must not survive into the queue work (bead `hugel4-k65`)
- Phase 01 is still at `human_needed` pending a real-gate dogfood of `--health` (bead `hugel4-vvd`); SUB-01..03 are marked *Gaps Found* until it clears
- Work tracking is bd (see `.agents/skills/beads`) — hugel writes only `Close`, `HandBack`, `Release` to bd, never content. Phase 9 (STORE-06) is the phase most likely to be tempted across that line

## Deferred Items

Items acknowledged and deferred at milestone close, most recent first:

| Category | Item | Status | Deferred At | Milestone |
|----------|------|--------|-------------|-----------|
| Substrate | SUB-04 … SUB-08 — widen event emission, carry ids not counts | Carried forward, unscheduled | 2026-09-11 | v0.1 |
| Substrate | SUB-09 … SUB-11 — bd's dependency edges, durable bead context, the ephemeral middle | Carried forward, unscheduled | 2026-09-11 | v0.1 |
| Surface | SURF-01 … SURF-03 — the resident garden | Carried forward, unscheduled | 2026-09-11 | v0.1 |
| Loop | LOOP-01 … LOOP-03 — GSD drives the work loop; the human → tender return leg | Carried forward, unscheduled | 2026-09-11 | v0.1 |

## Session Continuity

Last session: 2026-09-11T20:50:41.706Z
Stopped at: v0.2 roadmap created — Phases 7–11 written, 25/25 requirements mapped
Resume file: None
