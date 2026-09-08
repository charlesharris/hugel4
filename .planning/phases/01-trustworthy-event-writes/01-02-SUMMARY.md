---
phase: 01-trustworthy-event-writes
plan: 02
subsystem: infra
tags: [go, events, error-handling, observability]

# Dependency graph
requires:
  - phase: 01-trustworthy-event-writes (plan 01)
    provides: "events.Emit(e Event) error — the changed signature and the seven already-converted step/finishAs closure sites in internal/gate/run.go"
provides:
  - "All ten production events.Emit call sites in the codebase check the returned error and report a named notice on stderr, without changing their caller's control flow"
  - "A tender test proving Stop completes and reports when the event log is unwritable"
  - "A strengthened gate test proving two distinct named events (gate.test, gate.run) are reported lost while the gate still returns its verdict"
affects: [01-trustworthy-event-writes (plan 03: f.Sync durability), 01-trustworthy-event-writes (plan 04: health marker)]

actuals:
  tokens: 2131
  tasks: 2
  commits: 2

tech-stack:
  added: []
  patterns:
    - "Report-and-continue on a failed side-write: if err := events.Emit(...); err != nil { fmt.Fprintf(os.Stderr, \"hugel: event %q not recorded: %v\\n\", \"<event.name>\", err) } — never propagated, never changes the caller's return value or control flow"
    - "Go short-var-decl scoping used deliberately: an inner if err := events.Emit(...); err != nil block can reuse the identifier err even while an outer err (e.g. a tmux failure) is still live in the enclosing block, because the RHS of the inner := evaluates against the outer err before the inner err's scope begins"

key-files:
  created: []
  modified:
    - internal/gate/run.go
    - internal/gate/gate_test.go
    - internal/tender/start.go
    - internal/tender/tender_test.go
    - internal/cli/dispatch.go

key-decisions:
  - "internal/cli/dispatch.go's handBack site gets no dedicated test (plan decision, stated up front) — handBack shells out to bd, which is not deterministic in a unit test, and package cli has no test harness until plan 05. Held by the source-assertion acceptance criteria and the phase-wide unchecked-site grep instead."
  - "Reused the identifier err (rather than a differently-named inner variable) for all three tender.start.go sites, including the tmux-failure branch, after verifying with a standalone Go program that short-var-decl scoping keeps the outer tmux err intact through the shadowing inner if — matches both the plan's acceptance-criteria grep (literal 'if err := events.Emit(') and its instruction not to let the tmux error get displaced."

requirements-completed: [SUB-01]

coverage:
  - id: D1
    description: "The four remaining direct events.Emit call sites in internal/gate/run.go (gate.test on branch, gate.review, gate.test on merged, gate.land) check the error and print a named not-recorded notice without altering gate control flow"
    requirement: "SUB-01"
    verification:
      - kind: unit
        ref: "internal/gate/gate_test.go#TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten"
        status: pass
      - kind: unit
        ref: "go test ./internal/gate/..."
        status: pass
    human_judgment: false
  - id: D2
    description: "All three tender.start.go events.Emit sites (tmux-failure tender.start, success tender.start, Stop's tender.stop) and the dispatch.go handBack tender.handback site check and report without changing return values; a tender still stops and a handback still reaches bd"
    requirement: "SUB-01"
    verification:
      - kind: unit
        ref: "internal/tender/tender_test.go#TestATenderStopsEvenWhenTheLogCannotBeWritten"
        status: pass
      - kind: unit
        ref: "go test ./..."
        status: pass
    human_judgment: false
  - id: D3
    description: "Ten of ten production emitters check the error phase-wide (zero unchecked events.Emit call sites left in internal/, excluding tests)"
    requirement: "SUB-01"
    verification:
      - kind: unit
        ref: "grep -rn 'events\\.Emit(' --include='*.go' internal | grep -v '_test.go' | grep -cv 'if err := events\\.Emit(' => 0"
        status: pass
    human_judgment: false
  - id: D4
    description: "internal/cli/dispatch.go's handBack site is held by source assertion and the phase-wide grep rather than a dedicated test (documented plan decision, not an oversight)"
    human_judgment: true
    rationale: "handBack shells out to bd via beads.HandBack, which is not deterministic in a unit test, and package cli has no test harness until plan 05. A human/future-plan reviewer should confirm this gap is acceptable and remains closed by plan 05's harness."

duration: 13min
completed: 2026-09-08
status: complete
---

# Phase 01 Plan 02: Sweep the Eight Remaining `events.Emit` Call Sites Summary

**Every production `events.Emit` call site in gate, tender, and dispatch now checks its error and reports a named `not recorded` notice on stderr — with a new tender test and a strengthened gate test proving neither the gate's verdict nor a tender's stop are ever displaced by a failed event write.**

## Performance

- **Duration:** 13 min
- **Started:** 2026-09-08T22:02:23Z
- **Completed:** 2026-09-08T22:15:23Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Wrapped the four remaining direct `events.Emit` calls in `internal/gate/run.go` (`gate.test` on branch, `gate.review`, `gate.test` on merged, `gate.land`) in the same `if err := events.Emit(...); err != nil { fmt.Fprintf(os.Stderr, ...) }` idiom the `step`/`finishAs` closures already used, bringing all six gate emission points to a checked state
- Strengthened `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` to assert the `gate.test` notice is also reported (raising the lost-notice floor from 2 to 3), proving the gate still returns a nil error and a refusing `Report` while now losing two distinct named events
- Wrapped all three `events.Emit` calls in `internal/tender/start.go` (tmux-failure `tender.start`, success `tender.start`, `Stop`'s `tender.stop`) — the tmux-failure branch's `return nil, err` is unchanged, confirmed by verifying Go's short-var-decl scoping keeps the outer tmux `err` intact through the shadowing inner `if`
- Wrapped the `tender.handback` `events.Emit` in `internal/cli/dispatch.go`'s `handBack`, matching the fall-through idiom already used for `beads.HandBack`'s own failure — `beads.HandBack` still runs unconditionally
- Added `TestATenderStopsEvenWhenTheLogCannotBeWritten` in `internal/tender/tender_test.go`, proving `Stop` on a nameless tender returns nil and reports `event "tender.stop" not recorded` when `HUGEL_HOME` resolves through a regular file
- Confirmed phase-wide: zero unchecked production `events.Emit` call sites remain (was 10 at phase start)

## Task Commits

Each task was committed atomically:

1. **Task 1: The gate's four direct emitters report too** - `19594b2` (feat)
2. **Task 2: The tender and the handback report too** - `df20214` (feat)

**Plan metadata:** pending (this SUMMARY + STATE/ROADMAP commit)

## Files Created/Modified
- `internal/gate/run.go` - four remaining direct `events.Emit` sites now check and report their error
- `internal/gate/gate_test.go` - strengthened `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` to assert the `gate.test` notice and a 3-notice floor
- `internal/tender/start.go` - all three `events.Emit` sites (tmux-failure, success, `Stop`) now check and report
- `internal/tender/tender_test.go` - new `TestATenderStopsEvenWhenTheLogCannotBeWritten`
- `internal/cli/dispatch.go` - `handBack`'s `events.Emit` now checks and reports; `beads.HandBack` still runs unconditionally

## Decisions Made
- Reused the `err` identifier at all three `tender/start.go` sites (including the tmux-failure branch) rather than renaming the inner variable, after confirming with a standalone Go program that the outer tmux `err` survives the inner shadowing `if err := events.Emit(...)` unmodified — this satisfies both the plan's literal acceptance-criteria grep and its underlying concern (the tmux error must reach `return nil, err` untouched).
- `internal/cli/dispatch.go`'s `handBack` site remains untested by design (plan decision 4): `bd` is not available/deterministic in a unit test and package `cli` has no test harness until plan 05. Held by the phase-wide unchecked-site grep and the `beads.HandBack(` count criterion instead.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Ten of ten production `events.Emit` call sites now check and report their error; SUB-01's phase-wide unchecked-site count is zero.
- No emission site propagates its error: the gate still returns its verdict, the tender still starts and stops, the handback still reaches bd.
- `go test ./...` and `go vet ./...` are green; `go.mod`/`go.sum` unchanged.
- Ready for plan 03 (`f.Sync()` durability inside `events.Emit` itself) and plan 04 (the `events.failing-since` health marker), both of which build on the same `Emit(e Event) error` signature this plan and plan 01 finished converting all call sites onto.

---
*Phase: 01-trustworthy-event-writes*
*Completed: 2026-09-08*

## Self-Check: PASSED

All modified files confirmed present on disk; both task commits (19594b2, df20214) confirmed in git log; `go test ./...` and `go vet ./...` green; all plan-level `<verification>` commands re-run and passing.
