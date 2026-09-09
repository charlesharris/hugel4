---
phase: 01-trustworthy-event-writes
verified: 2026-09-09T02:28:40Z
status: gaps_found
score: 3/4 must-haves verified
covered_files:
  - ".planning/REQUIREMENTS.md"
  - ".planning/phases/01-trustworthy-event-writes/01-01-PLAN.md"
  - ".planning/phases/01-trustworthy-event-writes/01-01-SUMMARY.md"
  - ".planning/phases/01-trustworthy-event-writes/01-02-PLAN.md"
  - ".planning/phases/01-trustworthy-event-writes/01-02-SUMMARY.md"
  - ".planning/phases/01-trustworthy-event-writes/01-03-PLAN.md"
  - ".planning/phases/01-trustworthy-event-writes/01-03-SUMMARY.md"
  - ".planning/phases/01-trustworthy-event-writes/01-04-PLAN.md"
  - ".planning/phases/01-trustworthy-event-writes/01-04-SUMMARY.md"
  - ".planning/phases/01-trustworthy-event-writes/01-05-PLAN.md"
  - ".planning/phases/01-trustworthy-event-writes/01-05-SUMMARY.md"
  - "README.md"
  - "internal/cli/dispatch.go"
  - "internal/cli/main_test.go"
  - "internal/cli/yield.go"
  - "internal/cli/yield_test.go"
  - "internal/complete/spec.go"
  - "internal/config/sandbox.go"
  - "internal/events/events.go"
  - "internal/events/events_test.go"
  - "internal/gate/gate_test.go"
  - "internal/gate/run.go"
  - "internal/tender/start.go"
  - "internal/tender/tender_test.go"
covered_digest: "v1:sha256:19a108771df91df469faccb96eb89b0f6e8538bc33f22084776e83e54378c571"
behavior_unverified: 0
overrides_applied: 0
gaps:
  - truth: "A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing \"nothing has run since 2026-09-01\" from \"writes have been failing since 2026-09-01\", without reading the code"
    status: failed
    reason: "events.HealthOf (internal/events/events.go, os.Stat(p) at line 363) trusts a successful stat of the event log path as evidence of a real write without checking the path names a regular file. When $HUGEL_HOME/events.jsonl exists as a directory -- a log that has never been written and cannot be -- HealthOf returns Healthy=true, Reachable=true, FailingSince=nil, and a fabricated LastWrite taken from the directory's own mtime. Independently reproduced against this tree (see Behavioral Spot-Checks): Health = {Healthy:true Reachable:true LastWrite:<just now> FailingSince:<nil>} for a log that has never been written and is currently unwritable. hugel yield --health (internal/cli/yield.go:345-376) renders this straight through as \"event log:  healthy\" / \"last write: <a few seconds ago>\" -- exactly the \"writes have been failing unseen\" scenario this phase exists to prevent, reported as its opposite, on one command, without reading the code. Emit correctly fails in the identical filesystem state (also independently reproduced: \"open event log: ... is a directory\"), so the asymmetry is real: the write path is honest and the health-reporting path that exists specifically to catch this class of failure is not. No test in events_test.go constructs this state; TestHealthOfRefusesToGuessWhenTheGardenCannotBeRead covers a different scenario (HUGEL_HOME itself is a plain file), not events.jsonl being a directory under a valid HUGEL_HOME."
    artifacts:
      - path: "internal/events/events.go"
        issue: "os.Stat(p) at line 363 (inside HealthOf) does not check st.Mode().IsRegular() before trusting ModTime() as LastWrite; the same gap is repeated for the failure marker stat at line 375 (IN-01 in the code review)"
    missing:
      - "A regular-file check on the stat of the event log path (and the failure-marker path) before trusting its mtime as evidence of a write; report Reachable=false rather than a fabricated LastWrite when the path is occupied by something other than a file Emit could have written"
      - "A regression test that constructs the log path as a directory (mirroring TestAFailedWriteMarksTheGardenAsFailing's directory-path construction) and asserts HealthOf() reports Healthy=false / LastWrite=nil before any Emit call, not a fabricated recent write"
---

# Phase 1: Trustworthy Event Writes Verification Report

**Phase Goal:** A gardener can trust the event log — silence means nothing ran, not that writes have been failing unseen
**Verified:** 2026-09-09T02:28:40Z
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Truths are the four ROADMAP.md Success Criteria for this phase (Option A: roadmap contract).

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | With the event file made unwritable, the gardener sees the failure reported by the command that emitted the event, instead of the command reporting success | ✓ VERIFIED | `internal/events/events.go:215-262` — `Emit` returns a wrapped error on every write-path failure (`create garden dir`, `open event log`, `write event`, `sync event log`). All 10 production call sites (`internal/gate/run.go` ×6, `internal/tender/start.go` ×3, `internal/cli/dispatch.go` ×1) check the error and print `hugel: event %q not recorded: %v` to stderr — confirmed by direct grep, zero unchecked sites. End-to-end proof: `internal/gate/gate_test.go#TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` and `internal/tender/tender_test.go#TestATenderStopsEvenWhenTheLogCannotBeWritten` both construct an unwritable `HUGEL_HOME` (a plain file where a directory is expected) and assert the operation still returns its real result while stderr names the lost event. Both pass (`go test ./internal/gate/... ./internal/tender/...`). |
| 2 | An event written by a command that has returned survives a crash — the tail of the log is durable, not sitting in a buffer | ✓ VERIFIED | `internal/events/events.go:255-258` — `Emit` calls `f.Sync()` as a checked, non-deferred statement before its success return; a `Sync` failure returns `fmt.Errorf("sync event log: %w", err)` rather than a silent success. `TestEmitIsDurableBeforeItReturns` (`internal/events/events_test.go:175`) reads the just-written event back through a fresh `os.ReadFile` handle. Doc comment at events.go:209-214 correctly scopes the guarantee to what `fsync(2)` actually provides (notes the macOS `F_FULLFSYNC` gap, golang/go#26650) rather than overclaiming. |
| 3 | A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing "nothing has run since 2026-09-01" from "writes have been failing since 2026-09-01", without reading the code | ✗ FAILED | `hugel yield --health` exists and is wired (`internal/cli/yield.go:345-376` calls `events.HealthOf`, registered in `internal/complete/spec.go:85`, documented in README.md:34), and it correctly handles three of four filesystem states (healthy/written, fresh/never-written, failing-streak, and HUGEL_HOME-itself-unreadable). But `events.HealthOf` (`internal/events/events.go:363`) treats any successful `os.Stat` of the log path as proof of a write, without checking it names a regular file. Independently reproduced: with `$HUGEL_HOME/events.jsonl` created as a directory (never written, currently unwritable), `HealthOf()` returns `Healthy:true Reachable:true LastWrite:<just now> FailingSince:<nil>` — the command would print "event log: healthy" / "last write: a few seconds ago" for a log that has never recorded one event and cannot record one now. See Behavioral Spot-Checks and Gaps Summary below. |
| 4 | A failing event write is visible but never destroys the work it was instrumenting — gate and tender still complete, and the gardener is told | ✓ VERIFIED | Traced all 10 production `events.Emit` call sites: every one uses the `if err := events.Emit(...); err != nil { fmt.Fprintf(os.Stderr, ...) }` report-and-continue shape, and the surrounding function's return value/control flow is computed from variables bound before the `Emit` call (confirmed the `outcomeOf(err == nil)` shadowing pattern in `gate/run.go:109,197` and `tender/start.go:119` evaluates against the pre-existing outer `err`, per Go short-var-decl scoping — verified by reading, matches the code review's independent trace). `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` and `TestATenderStopsEvenWhenTheLogCannotBeWritten` both assert the real outcome is unchanged and the gardener sees the specific event name lost on stderr. |

**Score:** 3/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/events/events.go` (`Emit`) | returns `error`; checked `f.Sync()` before success | ✓ VERIFIED | Confirmed at lines 215-262 |
| `internal/events/events.go` (`HealthOf`) | two-fact health reader that never fabricates a state it cannot establish | ⚠️ STUB-LIKE (present, wired, behaviorally wrong) | Present and wired into the CLI, but produces a false-positive `Healthy:true` for a directory-shaped log path — see Truth 3 |
| `internal/gate/run.go` | all 6 `events.Emit` sites checked | ✓ VERIFIED | grep confirms `if err := events.Emit(` at every site, zero unchecked |
| `internal/tender/start.go` | 3 `events.Emit` sites checked | ✓ VERIFIED | Confirmed at lines 117, 128, 343 |
| `internal/cli/dispatch.go` (`handBack`) | checks and reports before telling bd | ✓ VERIFIED | Confirmed at line 174; `beads.HandBack` called unconditionally afterward |
| `internal/cli/yield.go` (`showHealth`) | renders `events.HealthOf`, plain or JSON | ✓ VERIFIED and WIRED | `--health` flag dispatches before transcript loading (yield.go:56-57); renders all documented states, but inherits `HealthOf`'s false-positive bug |
| `internal/complete/spec.go` | `--health` completion entry | ✓ VERIFIED | `Name: "health"` present at line 85 |
| `README.md` | events/health contract documented | ✓ VERIFIED | `hugel yield --health` documented at line 34; sandbox.go's doc comment no longer cites `Emit` as an error-discarding precedent |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `internal/gate/run.go` | `internal/events/events.go` | checked `events.Emit`, stderr report, control flow unchanged | ✓ WIRED | All 6 sites; verified by test and by reading |
| `internal/tender/start.go` | `internal/events/events.go` | same pattern, 3 sites | ✓ WIRED | Verified by test (`TestATenderStopsEvenWhenTheLogCannotBeWritten`) and by reading |
| `internal/cli/dispatch.go` (`handBack`) | `internal/beads/beads.go` (`HandBack`) | `beads.HandBack` still called after a failed event write | ✓ WIRED | Confirmed at dispatch.go:174-183; no dedicated test (documented plan decision in 01-02-SUMMARY.md — `beads.HandBack` shells to `bd`, non-deterministic in a unit test), held by source assertion + phase-wide grep instead |
| `internal/cli/yield.go` (`showHealth`) | `internal/events/events.go` (`HealthOf`) | direct call, no transcript dependency | ✓ WIRED | Confirmed at yield.go:346; also functionally FAILED per Truth 3 — the link works, the data it surfaces can be wrong |
| `internal/cli/yield.go` | `internal/complete/spec.go` | `--health` flag registered in completion table | ✓ WIRED | Both declare `Name: "health"` / `fs.Bool("health", ...)`; `internal/complete/spec_test.go` enforces this by walking source (passing per `go test ./...`) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `go build ./...` | full build | exit 0 | ✓ PASS |
| `go vet ./...` | static check | exit 0, no findings | ✓ PASS |
| `go test ./...` | full workspace suite (run once) | all 18 packages ok | ✓ PASS |
| CR-01 repro: `HealthOf()` on a directory-shaped, never-written log path | ad hoc test, `os.MkdirAll(p, 0o755)` then `HealthOf()`, run in-process via `go test -run` | `Health = {Healthy:true Reachable:true Home:... LastWrite:2026-09-08 21:27:35 ... FailingSince:<nil>}` | ✗ FAIL — confirms CR-01: reports healthy for a log that has never been written and cannot be |
| Asymmetry check: `Emit()` on the identical directory-shaped path | ad hoc test, same fixture, calls `Emit(Event{Name:"x"})` | `Emit err = open event log: ...: is a directory` | ✓ PASS (Emit is honest); confirms the asymmetry between the write path and the health-reporting path |

Both ad hoc reproduction tests were written to a scratch `_test.go` file under `internal/events/`, run once via `go test -run <name> -v`, and deleted immediately after — no state left in the tree.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|--------------|-------------|--------------|--------|----------|
| SUB-01 | 01-01, 01-02, 01-03 | `events.Record` (`Emit`) returns an error instead of swallowing it | ✓ SATISFIED | `Emit` returns `error`; all 10 production call sites check it; `TestEmitReturnsAnErrorWhenTheLogCannotBeWritten` passes |
| SUB-02 | 01-03 | Event writes are flushed durably | ✓ SATISFIED | Checked `f.Sync()` before success return; `TestEmitIsDurableBeforeItReturns` passes; `Timer.Done` carries the same contract |
| SUB-03 | 01-04, 01-05 | A gardener can distinguish "nothing has run" from "writes have been failing" without reading the code | ✗ BLOCKED | `hugel yield --health` exists, is wired, and correctly distinguishes 3 of 4 filesystem states — but it can silently report the opposite of the truth in the fourth (directory-shaped log path), which is precisely the failure this requirement exists to prevent. Not satisfied as stated. |

No orphaned requirements: REQUIREMENTS.md maps exactly SUB-01/02/03 to Phase 1, and all three appear across the five plans' `requirements` frontmatter.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/events/events.go` | 363 | `os.Stat(p)` trusted as proof of a write without an `IsRegular()` check | 🛑 Blocker | Causes Truth 3 / SUB-03 to fail (CR-01 in code review, independently reproduced above) |
| `internal/events/events.go` | 374-379 | Same non-regular-file assumption repeated for the `events.failing-since` marker stat | ℹ️ Info | Lower risk (marker is only ever created by `O_CREATE\|O_EXCL` via `markFailing`), but same shape as CR-01 and should be fixed alongside it (IN-01 in code review) |
| `internal/gate/run.go:109,197`; `internal/tender/start.go:119` | — | `outcomeOf(err == nil)` / recorded error message computed inside the initializer of the very `if err := events.Emit(...); err != nil` statement that shadows `err` — correct today only because of Go short-var-decl evaluation order, with no compiler diagnostic if a future refactor breaks it | ⚠️ Warning | Not a present failure (verified correct today, both by reading and by the passing gate/tender tests), but fragile — a future edit (splitting the literal out of the `if`, reordering statements) could silently start reporting the event-write's own failure in place of the operation being measured (WR-01 in code review) |

No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` markers found in any file modified by this phase.

### Gaps Summary

One blocker: **`events.HealthOf` can report a broken, never-written garden as healthy.**

The phase's stated goal is "silence means nothing ran, not that writes have been failing
unseen." Three of its four success criteria hold solidly — the write path (`Emit`) is honest
about failure, the tail of the log is durably synced, and a failing write never changes what
gate or tender actually did. But the fourth criterion is the trust surface itself:
`hugel yield --health`, the one command a gardener runs specifically to answer "can I trust
this log," and it inherits a real bug from `events.HealthOf`.

`HealthOf` derives `LastWrite` from `os.Stat(p).ModTime()` without checking that `p` names a
regular file. This is not a hypothetical: it is exactly the class of failure Success Criterion
1 names — "bad `HUGEL_HOME`" — realized as `$HUGEL_HOME/events.jsonl` occupied by a directory
(e.g. a `mkdir -p` typo in setup, or a botched migration). In that state, `Emit` correctly
fails (independently reproduced: `open event log: ...: is a directory`), but `HealthOf` — the
mechanism built specifically to surface exactly this kind of silent failure — reports
`Healthy: true` with a plausible, recent `LastWrite` fabricated from the directory's own mtime,
because until the first `Emit` attempt against that path, `events.failing-since` was never
created and `FailingSince` stays `nil`. `hugel yield --health` renders this straight through as
"event log: healthy" / "last write: a few seconds ago."

This is not a documentation gap or a low-risk edge case: it is the health surface confidently
asserting the opposite of the truth in the exact scenario the phase exists to make visible. No
test in `events_test.go` constructs this state — the existing `TestHealthOfRefusesToGuessWhenTheGardenCannotBeRead`
covers a different failure (`HUGEL_HOME` itself is a plain file), not `events.jsonl` being a
directory under an otherwise-valid `HUGEL_HOME`.

**Fix, per the code review (CR-01), independently confirmed correct by this verification:**
only trust the stat as evidence of a real write when `st.Mode().IsRegular()`; otherwise report
`Reachable: false` rather than fabricating a timestamp. Apply the same guard to the failure
marker's stat (IN-01) and add a regression test constructing the log path as a directory,
asserting `HealthOf()` reports `Healthy: false` / `LastWrite: nil` before any `Emit` call.

The WR-01 shadowing fragility is a real but lower-priority finding — it does not currently
misbehave (confirmed by reading and by passing tests), so it is recorded as a Warning rather
than blocking this phase, but should not be left indefinitely given how easily a future edit
could invert it silently.

---

_Verified: 2026-09-09T02:28:40Z_
_Verifier: Claude (gsd-verifier)_
