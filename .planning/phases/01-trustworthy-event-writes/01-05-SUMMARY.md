---
phase: 01-trustworthy-event-writes
plan: 05
subsystem: cli
tags: [go, cli, events, observability, testing]

# Dependency graph
requires:
  - phase: 01-04
    provides: "type Health and func HealthOf() (Health, error) — the two-fact health reader this plan renders"
provides:
  - "hugel yield --health — a fifth boolean view on the existing yield command, plain text or JSON"
  - "func showHealth(asJSON bool) error — the render layer over events.HealthOf"
  - "package cli's first tests, and the TestMain garden that keeps them off the gardener's real one"
affects: []

# Actuals (#2632)
actuals:
  tokens: 2272
  tasks: 2
  commits: 2
plan_head_before: 699c68d138c5d1884f7c4bfbe9da7f076a7c4317

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Flag-selected report view dispatched before the shared transcript load, for the one yield view that answers from the garden rather than from a session"
    - "TestMain-scoped garden for a CLI package's first test, mirroring internal/gate/main_test.go"

key-files:
  created:
    - internal/cli/main_test.go
    - internal/cli/yield_test.go
  modified:
    - internal/cli/yield.go
    - internal/complete/spec.go
    - README.md

key-decisions:
  - "--health dispatches immediately after fs.Parse, above the transcript-root resolution and transcript.LoadAll call, so the answer never depends on a transcript directory and never triggers the 'no sessions with recorded usage' early return"
  - "The unreachable-garden branch prints Health.Home on both report lines rather than a shared preamble, so grepping the output for the path always finds it regardless of which line a caller inspects"
  - "Task 2's TDD cycle collapsed to a single test(01-05) commit: showHealth was already implemented by task 1, so the four tests could not fail genuinely on first run — same situation and same resolution as 01-01 and 01-04's collapsed cycles"

patterns-established:
  - "internal/cli package tests set HUGEL_HOME via t.Setenv(t.TempDir()) per test, layered over the package-wide TestMain garden, matching internal/events and internal/gate conventions"

requirements-completed: [SUB-03]

coverage:
  - id: D1
    description: "hugel yield --health reports the log's state and last write in one command, in plain text or JSON, without reading a transcript directory"
    requirement: "SUB-03"
    verification:
      - kind: unit
        ref: "internal/cli/yield_test.go#TestHealthShowsNothingHasRunInAFreshGarden"
        status: pass
      - kind: command
        ref: "HUGEL_HOME=$(mktemp -d) go run ./cmd/hugel yield --health --root $(mktemp -d)"
        status: pass
      - kind: command
        ref: "HUGEL_HOME=$(mktemp -d) go run ./cmd/hugel yield --health --json --root $(mktemp -d)"
        status: pass
    human_judgment: false
  - id: D2
    description: "The report distinguishes a garden where nothing has run from one where writes have been failing, and names the date of each"
    requirement: "SUB-03"
    verification:
      - kind: unit
        ref: "internal/cli/yield_test.go#TestHealthShowsNothingHasRunInAFreshGarden"
        status: pass
      - kind: unit
        ref: "internal/cli/yield_test.go#TestHealthShowsWhenWritesHaveBeenFailing"
        status: pass
      - kind: unit
        ref: "internal/cli/yield_test.go#TestHealthAsJSONCarriesBothFacts"
        status: pass
    human_judgment: false
  - id: D3
    description: "When the garden cannot be read, the report says so instead of reporting healthy"
    requirement: "SUB-03"
    verification:
      - kind: unit
        ref: "internal/cli/yield_test.go#TestHealthSaysSoWhenTheGardenCannotBeRead"
        status: pass
    human_judgment: false
  - id: D4
    description: "The flag is offered by shell completion, documented in the README, and reachable without a transcript directory"
    requirement: "SUB-03"
    verification:
      - kind: unit
        ref: "internal/complete/spec_test.go#TestSpecMatchesTheFlagsTheCLIRegisters"
        status: pass
      - kind: command
        ref: "go run ./cmd/hugel completion zsh | grep -c -- --health"
        status: pass
    human_judgment: true
    rationale: "The completion table entry and README line are asserted by grep/test; whether a gardener actually finds and uses the flag from a real shell session is not something an automated check can confirm."

# Metrics
duration: 4min
completed: 2026-09-09
status: complete
---

# Phase 01 Plan 05: `hugel yield --health` and package `cli`'s first tests Summary

**`hugel yield --health` renders `events.HealthOf` as a two-line plain-text or JSON report, dispatched before any transcript is touched, and lands with the four tests — and the `TestMain` garden — that are package `cli`'s first ever test file.**

## Performance

- **Duration:** 4 min (commit-timestamp proxy, `699c68d`→`0a47c98`)
- **Started:** 2026-09-09T02:11:53Z (approx.)
- **Completed:** 2026-09-09T02:15:51Z
- **Tasks:** 2
- **Files modified:** 5 (2 created, 3 modified)

## Accomplishments
- `--health` added to `hugel yield`'s flag block and usage text, dispatched immediately after `fs.Parse` and before the transcript root is resolved — the only `yield` view that never reads `~/.claude/projects` and never triggers the "no sessions with recorded usage" early return
- `showHealth(asJSON bool) error` added beside `showSoil`: calls `events.HealthOf()` and returns its error unchanged (the only failure this view has); JSON form encodes `Health` directly with `SetIndent`, matching `showSoil`'s convention; plain-text form is two width-aligned lines — the log's state (healthy / `FAILING since <date> (<age>)` / unknown-and-path-named) and the last write (`never` / `<date> (<age> ago)` / unknown-and-path-named)
- `internal/complete/spec.go`'s `yield` command gained a `{Name: "health", ...}` row, keeping `internal/complete/spec_test.go`'s CLI-drift check green
- `README.md`'s `hugel yield` command block documents `--health`
- `internal/cli/main_test.go` gives package `cli` a `TestMain`-scoped temp garden, its first ever guard against writing to the gardener's real garden — copied from `internal/gate/main_test.go`'s pattern
- `internal/cli/yield_test.go` adds a non-parallel stdout-capture helper and four tests covering every state `showHealth` can render: a fresh garden (healthy, "never"), a backdated `events.failing-since` (FAILING, exact date), an unreadable garden (path named, never "healthy"), and the JSON form of a failing garden (`healthy: false`, `failing_since` present)

## Task Commits

1. **Task 1:** `0d4a66d` — `feat(01-05): hugel yield --health, wired end to end`
2. **Task 2:** `0a47c98` — `test(01-05): the four health reports, and cli's own garden`

**Plan metadata:** committed separately after this SUMMARY (see final commit).

## Files Created/Modified
- `internal/cli/yield.go` — `--health` flag, usage line, early dispatch, `showHealth`
- `internal/complete/spec.go` — completion row for `--health` under `yield`
- `README.md` — `hugel yield --health` documented in the command block
- `internal/cli/main_test.go` — new: `TestMain` garden for package `cli`
- `internal/cli/yield_test.go` — new: stdout-capture helper and four `showHealth` tests

## Decisions Made
- `--health` dispatches before `transcript.LoadAll` is called at all, not merely before the other three view branches — the plan's acceptance criteria assert dispatch happens at a lower line number than the transcript load, which this satisfies by construction
- The unreachable-garden branch repeats `h.Home` on both the "event log" and "last write" lines rather than printing the path once in a shared preamble, so the path is findable regardless of which line is inspected and neither line has to borrow the other's context
- Kept the TDD task's tests as a single `test(01-05)` commit rather than staging artificial RED — see TDD Gate Compliance below

## Deviations from Plan

None — plan executed exactly as written. One TDD mechanics note is documented below (not a deviation from the plan's own instructions).

## TDD Gate Compliance

Task 2 carried `tdd="true"`, but `showHealth` — the function under test — was already fully implemented by task 1 (a `type="auto"` task, not TDD) in the same plan. Writing the four tests therefore could not produce a genuine RED: all four passed on first run against the already-correct implementation. This is the same situation 01-01 and 01-04 documented (a RED phase that "could not fail genuinely" because the target behavior was already implemented), and it is resolved the same way — one `test(01-05)` commit rather than a stubbed-then-filled-in RED/GREEN pair, since there was no production code left to stub.

| Task | RED | GREEN | REFACTOR |
|------|-----|-------|----------|
| 2 (health report tests) | `0a47c98` — all 4 tests pass immediately; no genuine failing assertion was possible since `showHealth` (task 1, `0d4a66d`) already implemented the exact behavior under test | Not applicable — nothing to implement | Not needed |

## Issues Encountered
None.

## User Setup Required
None — no external service configuration required.

## Requirements Traceability

SUB-03 ("A gardener can distinguish 'nothing has run since <date>' from 'writes have been failing since <date>' without reading the code") is a requirement shared between 01-04 and this plan, held `Pending` by the shared-ID gate until both plans' SUMMARYs exist. Both now exist — SUB-03 should close on this plan's `update_requirements` step. Confirmed in git: `.planning/REQUIREMENTS.md` line 15/80 read `[ ]`/`Pending` as of this SUMMARY's authoring; the executor's `requirements mark-complete` step (below) is expected to flip both.

## Next Phase Readiness
- Phase 01 (Trustworthy Event Writes) is now complete: all five plans have SUMMARYs, and `go test ./...` is green with no dependency added across the phase
- `internal/cli` has its first tests and its own `TestMain` garden — any future test added to package `cli` inherits the guard for free
- Dogfood spot-check (partial — read-only half only): `go run ./cmd/hugel yield --health` with no `HUGEL_HOME` override on this machine reports `event log: healthy` and `last write: 2026-09-01 16:36 (7d ago)`, consistent with PROJECT.md's own note that the real event log was "last written 2026-09-01." The plan's full human-check (rerun after a real gate or tender emits, to confirm the last-write time moves) was intentionally not executed by this automated run — it invokes real side effects (a live gate or tender session against the gardener's actual work) that an unattended executor should not trigger, and the plan's own `<verify>` block marks it `<human-check>` rather than `<automated>` for exactly that reason. Left for human verification.
- No blockers or concerns

---
*Phase: 01-trustworthy-event-writes*
*Completed: 2026-09-09*

## Self-Check: PASSED

- FOUND: internal/cli/yield.go
- FOUND: internal/complete/spec.go
- FOUND: README.md
- FOUND: internal/cli/main_test.go
- FOUND: internal/cli/yield_test.go
- FOUND commit: 0d4a66d
- FOUND commit: 0a47c98
- `go test ./...` — all packages ok, `internal/cli` reports `ok` (was `[no test files]`), no FAIL
- `go vet ./...` — silent
- `git diff --exit-code go.mod go.sum` — exits 0, no dependency added
- `go build -o /tmp/hugel-health ./cmd/hugel && HUGEL_HOME=$(mktemp -d) /tmp/hugel-health yield --health` — prints a two-line report from a real binary
- `go run ./cmd/hugel completion zsh | grep -c -- --health` — 1, the generated completion script offers the flag
