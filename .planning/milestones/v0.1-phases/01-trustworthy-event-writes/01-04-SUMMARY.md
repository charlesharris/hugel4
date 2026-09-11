---
phase: 01-trustworthy-event-writes
plan: 04
subsystem: infra
tags: [go, events, observability, filesystem]

# Dependency graph
requires:
  - phase: 01-03
    provides: "events.Emit(e Event) error with a checked f.Sync() before it returns nil"
provides:
  - "the sticky first-failure marker: failMarkPath, markFailing, clearFailing at internal/events/events.go"
  - "type Health and func HealthOf() (Health, error) — the two-fact health reader"
affects: [01-05]

# Actuals (#2632)
actuals:
  tokens: 3823
  tasks: 2
  commits: 4
plan_head_before: 138a993

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Sticky first-failure marker via O_CREATE|O_EXCL (internal/cli/dispatch.go's lockGarden idiom, inverted from compostMark's most-recent-write-wins to first-write-wins)"
    - "Health as a field-carrying struct (Reachable, Home) rather than an inferred boolean, so an unreachable garden reports that it cannot answer instead of guessing healthy"

key-files:
  created: []
  modified:
    - internal/events/events.go
    - internal/events/events_test.go

key-decisions:
  - "TDD RED phase used stubbed markFailing/clearFailing (no-ops) and a stubbed HealthOf (fixed Health{}) rather than omitting the identifiers entirely — Go's whole-package compilation means a test calling a not-yet-defined function is a build failure, not a genuine RED; stubbing the signature and giving it a real body only in GREEN keeps the RED failure on the test's assertion instead of on the build"
  - "TestAnUnreachableGardenCannotBeMarked (task 1) passed vacuously against the RED stub, since a no-op marker function trivially satisfies 'no marker exists' for the wrong reason — documented in the RED commit message rather than treated as a plan or test defect; it genuinely re-verified after GREEN wiring"
  - "HealthOf demotes Reachable to false if the log itself cannot be stat'd for a reason other than not-exist (e.g. permission-denied), rather than reporting a directory-only reachability signal, so a partially-unreadable garden does not present as 'reachable but never written'"

patterns-established:
  - "Field-shaped health/status types over derived booleans in this package: Reachable and Home carry independent facts so a caller (and this SUMMARY's own tests) can distinguish 'cannot be answered' from 'answered as healthy'"

requirements-completed: [SUB-03]

coverage:
  - id: D1
    description: "A failed event write leaves a marker (events.failing-since) dated to the first failure of the streak; later failures in the same streak do not move that date; the next successful write clears it"
    requirement: "SUB-03"
    verification:
      - kind: unit
        ref: "internal/events/events_test.go#TestAFailedWriteMarksTheGardenAsFailing"
        status: pass
      - kind: unit
        ref: "internal/events/events_test.go#TestTheFailureMarkKeepsTheFirstFailuresTime"
        status: pass
      - kind: unit
        ref: "internal/events/events_test.go#TestASuccessfulWriteClearsTheFailureMark"
        status: pass
      - kind: unit
        ref: "internal/events/events_test.go#TestAnUnreachableGardenCannotBeMarked"
        status: pass
    human_judgment: false
  - id: D2
    description: "events.HealthOf reports the log's last-write time and the open failure streak's start as two independent, nilable facts"
    requirement: "SUB-03"
    verification:
      - kind: unit
        ref: "internal/events/events_test.go#TestHealthOfReportsAWrittenLogAsHealthy"
        status: pass
      - kind: unit
        ref: "internal/events/events_test.go#TestHealthOfSaysNothingHasRunInAFreshGarden"
        status: pass
      - kind: unit
        ref: "internal/events/events_test.go#TestHealthOfReportsAFailingStreak"
        status: pass
    human_judgment: false
  - id: D3
    description: "When the garden itself cannot be read, HealthOf says so (Reachable/Healthy both false, Home named, nil error) rather than reporting healthy from two absences"
    requirement: "SUB-03"
    verification:
      - kind: unit
        ref: "internal/events/events_test.go#TestHealthOfRefusesToGuessWhenTheGardenCannotBeRead"
        status: pass
    human_judgment: false

# Metrics
duration: 3min
completed: 2026-09-09
status: complete
---

# Phase 01 Plan 04: A sticky first-failure marker and a two-fact health reader Summary

**`events.failing-since` pins the first failure of a streak via `O_CREATE|O_EXCL` and clears on the next success; `events.HealthOf` reads it alongside the log's own mtime and a garden-reachability check, so a gardener can now tell "nothing has run" from "writes have been failing" — and from "I can't tell" — without reading code.**

## Performance

- **Duration:** 3 min (commit-timestamp proxy, `a85b5de`→`66563ba`)
- **Started:** 2026-09-09T02:07:51Z (approx.)
- **Completed:** 2026-09-09T02:09:48Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- `failMarkPath`, `markFailing`, `clearFailing` added beside `Path` in `internal/events/events.go`: `markFailing` creates `$HUGEL_HOME/events.failing-since` with `os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)` so an already-open streak's exists-error leaves the mtime alone; `clearFailing` unconditionally `os.Remove`s it, ignoring a not-exist error
- `Emit` wired to call `markFailing()` before each of its four failure returns inside the locked write section (create garden dir, open log, write, sync) and `clearFailing()` before its success return — the marshal and path-resolution failures above the lock do not mark, matching the plan's "a bad event is not a broken log" rule
- `type Health` (`Healthy`, `Reachable`, `Home`, `LastWrite *time.Time`, `FailingSince *time.Time`) and `func HealthOf() (Health, error)` added beside `Load`: stats the garden directory for reachability, the log for `LastWrite`, and the marker for `FailingSince`, returning a non-nil error only when `Path()` itself cannot resolve
- All four health states covered by test: a written log (healthy), a fresh garden (healthy, both times nil), an open failure streak (unhealthy, `FailingSince` set), and an unreachable garden (`Reachable`/`Healthy` both false, `Home` named, nil error)

## Task Commits

Each task followed a genuine RED-GREEN TDD cycle (no REFACTOR commit needed — first-pass implementation matched the plan's design):

1. **Task 1 RED:** `a85b5de` — `test(01-04): add failing tests for the sticky failure marker`
2. **Task 1 GREEN:** `6a02b1f` — `feat(01-04): a first failure leaves a mark that later failures do not move`
3. **Task 2 RED:** `bac1646` — `test(01-04): add failing tests for events.HealthOf`
4. **Task 2 GREEN:** `66563ba` — `feat(01-04): events.HealthOf answers the gardener's question in one call`

**Plan metadata:** committed separately after this SUMMARY (see final commit).

## Files Created/Modified
- `internal/events/events.go` — `failMarkPath`/`markFailing`/`clearFailing` added; `Emit` wired to mark/clear; `type Health` and `func HealthOf` added
- `internal/events/events_test.go` — 8 new tests: 4 for the marker's create/pin/clear cycle, 4 for `HealthOf`'s states

## Decisions Made
- RED phases used stubbed (no-op / fixed-zero-value) production identifiers rather than omitting them, since Go's whole-package build makes a test referencing an undefined identifier a build failure rather than a genuine assertion-level RED; the stub's body was only filled in during GREEN — see TDD Gate Compliance below
- `HealthOf` treats a log `os.Stat` error other than not-exist (e.g. permission-denied) as demoting the whole answer to `Reachable: false`, rather than a directory-only reachability check that could report "reachable" over a garden whose contents cannot actually be read

## Deviations from Plan

None - plan executed exactly as written. One TDD mechanics note is documented below (not a deviation from the plan's own instructions).

## TDD Gate Compliance

Both tasks carried `tdd="true"`. Go's package-level compilation means a test that calls a function not yet declared anywhere in the package fails the *build*, not the target assertion — which the canonical TDD reference's fail-fast rules would classify as `INVALID_RED` if a strict RED-evidence check were run. To keep RED genuine (a failing assertion on the *planned behavior*, not a compile error), each task's RED commit declared the new identifiers with their final signatures but a stubbed body (`markFailing`/`clearFailing` as no-ops, `HealthOf` returning a fixed `Health{}, nil`), then gave them their real implementation in the GREEN commit.

| Task | RED | GREEN | REFACTOR |
|------|-----|-------|----------|
| 1 (marker) | `a85b5de` — 3 of 4 tests fail genuinely on assertions; `TestAnUnreachableGardenCannotBeMarked` passes vacuously against the no-op stub (documented in the commit message) | `6a02b1f` — all 4 pass, including a re-verified `TestAnUnreachableGardenCannotBeMarked` | Not needed — GREEN implementation matched the plan's design on first pass |
| 2 (HealthOf) | `bac1646` — all 4 tests fail genuinely on assertions against the fixed-zero-value stub | `66563ba` — all 4 pass | Not needed |

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plan 01-05 (`hugel yield --health`) can proceed directly — `events.HealthOf`'s signature and `Health` struct's JSON tags are the stable contract it renders
- SUB-03 is not yet marked complete in REQUIREMENTS.md: it is a shared requirement ID with 01-05, and the shared-ID gate holds it `Pending` until 01-05's SUMMARY also exists
- No blockers or concerns

---
*Phase: 01-trustworthy-event-writes*
*Completed: 2026-09-09*

## Self-Check: PASSED

- FOUND: internal/events/events.go
- FOUND: internal/events/events_test.go
- FOUND commit: a85b5de
- FOUND commit: 6a02b1f
- FOUND commit: bac1646
- FOUND commit: 66563ba
- `go test ./...` — all packages ok, no FAIL
- `go vet ./...` — silent
- `git diff --exit-code go.mod go.sum` — exits 0, no dependency added
- `grep -n "markFailing\|clearFailing" internal/events/events.go` — 4 marking calls inside the locked section, 1 clearing call on the success path, matching the plan's `<verification>` block
