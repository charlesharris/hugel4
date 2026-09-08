---
phase: 01-trustworthy-event-writes
plan: 01
subsystem: infra
tags: [go, events, observability, error-handling]

# Dependency graph
requires: []
provides:
  - "events.Emit(e Event) error — every silent write failure now returns a wrapped error"
  - "the report-and-continue idiom at internal/gate/run.go's step and finishAs closures"
  - "the agreed stderr notice shape: hugel: event %q not recorded: %v"
affects: [01-02, 01-03, 01-04, 01-05]

# Actuals (#2632)
actuals:
  tokens: 2477
  tasks: 2
  commits: 2
plan_head_before: c9f5a27

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Report-and-continue on a failed side-write: caller checks the error, prints hugel: event %q not recorded: %v to stderr, and never returns it or alters control flow"

key-files:
  created: []
  modified:
    - internal/events/events.go
    - internal/events/events_test.go
    - internal/gate/run.go
    - internal/gate/gate_test.go

key-decisions:
  - "Emit's doc comment rewritten in full: the caller, not the package, now decides whether its own work can tolerate a warning"
  - "Only the two closures (step, finishAs) in internal/gate/run.go were wrapped in this plan; the four direct events.Emit call sites there (gate.test x2, gate.review, gate.land) are left for plan 02, per the plan's explicit scope"
  - "Timer.Done left unchanged (still discards Emit's return) — plan 03 owns its signature change"
  - "Task 2's TDD cycle collapsed to a single test(...) commit: the behavior under test (Emit returning an error) was already implemented by task 1's tracer commit, so there was no production code left to write in a GREEN phase — see TDD Gate Compliance below"

patterns-established:
  - "Report-and-continue on a failed side-write (matches internal/cli/soil.go:recordDraw and internal/draws/draws.go's fmt.Errorf(\"verb noun: %w\", err) wording)"

requirements-completed: [SUB-01]

coverage:
  - id: D1
    description: "events.Emit returns a non-nil, descriptive error on every failure path instead of swallowing it"
    requirement: "SUB-01"
    verification:
      - kind: unit
        ref: "internal/events/events_test.go#TestEmitReturnsAnErrorWhenTheLogCannotBeWritten"
        status: pass
      - kind: unit
        ref: "go vet ./..."
        status: pass
    human_judgment: false
  - id: D2
    description: "A gate whose event writes all fail still reaches its verdict and returns it to its caller unchanged (nil error, correct Report)"
    requirement: "SUB-01"
    verification:
      - kind: integration
        ref: "internal/gate/gate_test.go#TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten"
        status: pass
    human_judgment: false
  - id: D3
    description: "The gardener sees a hugel: event %q not recorded: %v line on stderr for each event the gate could not write, without leaking Event.Fields"
    requirement: "SUB-01"
    verification:
      - kind: integration
        ref: "internal/gate/gate_test.go#TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten"
        status: pass
      - kind: unit
        ref: "grep -c 'e.Fields' internal/gate/run.go (must be 0)"
        status: pass
    human_judgment: false
  - id: D4
    description: "No test in the events package still asserts that a dropped event is silent"
    requirement: "SUB-01"
    verification:
      - kind: unit
        ref: "go test -run TestEmit ./internal/events/..."
        status: pass
    human_judgment: false

duration: 16min
completed: 2026-09-08
status: complete
---

# Phase 01 Plan 01: Emit returns its failure, and a gate says so without failing Summary

**`events.Emit` now returns a wrapped error on every write-failure path, and `internal/gate/run.go`'s two emission closures report that error on stderr without changing gate control flow — proven end-to-end by a gate run against a wholly unwritable event log.**

## Performance

- **Duration:** 16 min (commit-timestamp proxy)
- **Started:** 2026-09-08T21:59:00Z (approx.)
- **Completed:** 2026-09-08T22:01:39Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- `events.Emit` changed from `func Emit(e Event)` to `func Emit(e Event) error`, wrapping each of its five former silent-exit points (`marshal event`, `resolve event log path`, `create garden dir`, `open event log`, `write event`) with `fmt.Errorf("...: %w", err)`
- Rewrote `Emit`'s doc comment to state the new contract and why the caller, not the package, decides whether a warning is tolerable
- `internal/gate/run.go`'s `step` and `finishAs` closures now check `Emit`'s error and print `hugel: event %q not recorded: %v` to stderr, touching zero control-flow paths (`rep.Reached`/`rep.Stages` in `step`, `return r` in `finishAs` unchanged) — fixing 7 emission points across the file without duplicating the check per call
- Added `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten`: a real `gate.Run` against a tender whose `HUGEL_HOME` resolves through a regular file (`ENOTDIR` on `MkdirAll`) still returns a nil error and a `Report{Passed: false}`, with the captured stderr containing `event "gate.run" not recorded` and at least two `not recorded` occurrences
- Rewrote `internal/events/events_test.go`'s test suite so no happy-path test discards `Emit`'s return value, and the former "cannot fail" test is renamed and now asserts a descriptive, non-nil error

## Task Commits

Each task was committed atomically:

1. **Task 1: Emit returns its failure, and a gate says so without failing** - `4334fab` (feat, tracer)
2. **Task 2: The events package asserts the new contract, not the old one** - `1885fde` (test)

**Plan metadata:** committed separately after this SUMMARY (see final commit).

_Note: Task 2 is a single `test(...)` commit rather than the usual RED→GREEN pair — see TDD Gate Compliance below._

## Files Created/Modified
- `internal/events/events.go` - `Emit` signature changed to return `error`; five silent-exit points now wrapped; doc comment rewritten
- `internal/events/events_test.go` - happy-path tests bind and check `Emit`'s result; the unwritable-garden test renamed and asserts a descriptive error instead of only "no panic"
- `internal/gate/run.go` - `step` and `finishAs` closures check `Emit`'s error and print the agreed stderr notice; the four direct call sites are untouched (plan 02)
- `internal/gate/gate_test.go` - new end-to-end test proving a gate with a wholly unwritable log still reaches and returns its verdict while reporting the loss on stderr

## Decisions Made
- Kept the four direct `events.Emit(...)` calls in `internal/gate/run.go` (gate.test x2, gate.review, gate.land) exactly as written — Go permits discarding a call's return value, so they compile unchanged, and plan 02 owns sweeping them per the plan's own scope note
- Did not add `f.Sync()` to `Emit` — durability is plan 03's responsibility and stays a separately revertable commit
- `Timer.Done` left with its current no-return signature; plan 03 changes it

## Deviations from Plan

None - plan executed exactly as written. One clarification on TDD mechanics is documented below (not a deviation from the plan's instructions, which specified exactly this test-file-only action for task 2).

## TDD Gate Compliance

Task 2 carried `tdd="true"`, but its `<action>` was a pure test-file rewrite (`<files>internal/events/events_test.go</files>`, no production code) asserting a contract that task 1's tracer commit had already implemented (`events.Emit` already returned wrapped errors before task 2 began). Per the canonical TDD reference's own `error_handling` guidance ("Test doesn't fail in RED phase: Feature may already exist - investigate"), forcing an artificial RED commit against already-passing production code would not have produced a real failing-then-fixed cycle — the test would have gone straight to GREEN with the pre-existing implementation.

Given that, this task produced one `test(01-01): ...` commit (`1885fde`) rather than a `test(...)` → `feat(...)` pair. This is a **documented, intentional deviation from the standard 2-3-commit TDD pattern**, not a missing GREEN commit: there was no new production code for a GREEN commit to add. All of task 2's acceptance criteria (test renamed, old name gone, ≥4 bound `Emit` calls, `go test -run TestEmit` green) were verified before commit.

| Gate | Status | Commit |
|------|--------|--------|
| RED (test exists, was already green given task 1's implementation) | N/A — no artificial RED possible | — |
| GREEN (implementation) | Already satisfied by task 1 (`4334fab`) | `4334fab` |
| Test assertions updated | ✓ | `1885fde` |
| REFACTOR | Not needed | — |

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plan 01-02 (the remaining eight call sites: gate, tender, dispatch) can proceed directly — the report-and-continue idiom and stderr notice shape are now proven end-to-end and ready to replicate
- `events.Emit`'s new signature is the stable contract every remaining plan in this phase builds on
- No blockers or concerns

---
*Phase: 01-trustworthy-event-writes*
*Completed: 2026-09-08*

## Self-Check: PASSED

- FOUND: internal/events/events.go
- FOUND: internal/events/events_test.go
- FOUND: internal/gate/run.go
- FOUND: internal/gate/gate_test.go
- FOUND commit: 4334fab
- FOUND commit: 1885fde
- `go test ./...` — all packages ok, no FAIL
- `go vet ./...` — silent
- `git diff --exit-code go.mod go.sum` — exits 0, no dependency added
- `grep -rn "events\.Emit(" --include="*.go" internal | grep -v _test.go` — 10 call sites, matching plan verification
