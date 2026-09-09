---
phase: 01-trustworthy-event-writes
verified: 2026-09-09T20:52:00Z
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
  - ".planning/phases/01-trustworthy-event-writes/01-REVIEW.md"
  - ".planning/phases/01-trustworthy-event-writes/01-UAT.md"
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
covered_digest: "v1:sha256:beaf83edf34fea7b86b08f84e03f3036ccb1dfa4d1f9482c5f4a9091f985a893"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 3/4
  gaps_closed:
    - "CR-01/IN-01: events.HealthOf trusted a bare os.Stat of the log path and of the failure marker without an IsRegular() check, reporting Healthy=true with a fabricated LastWrite for a directory-shaped, never-written log. Independently re-measured: fixed, wired, and held by two mutation-proven regression tests."
  gaps_remaining:
    - "Truth 3 / SUB-03 remains FAILED, on a different filesystem state than the one the previous pass named: an unwritable (not unreadable) garden directory."
  regressions:
    - "None functional. go build, go vet, go test -count=1 ./... and go test -race on the five touched packages are all clean; the seam refactor changed no production behaviour (syncFile defaults to (*os.File).Sync)."
    - "Documentation drift only: 01-03-PLAN.md must_haves still assert the pre-seam literal `if err := f.Sync(); err != nil` and the key-link pattern `f\\.Sync\\(\\)`, both of which no longer match the source. See Warnings."
gaps:
  - truth: "A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing \"nothing has run since 2026-09-01\" from \"writes have been failing since 2026-09-01\", without reading the code"
    status: failed
    reason: "The previously-reported defect (CR-01, log path occupied by a directory) is genuinely fixed and mutation-proven. A distinct instance of the same must-not survives, in a filesystem state Success Criterion 1 names by name: a read-only garden directory. events.HealthOf establishes a failing streak solely from $HUGEL_HOME/events.failing-since, and markFailing() creates that marker inside the very directory that is unwritable, so the marker creation fails silently (internal/events/events.go:187-191, error deliberately discarded). HealthOf then sees a readable directory, a readable regular events.jsonl, and no marker, and computes Healthy=true (events.go:404). Independently reproduced end-to-end twice against this tree, through the real built binary: (a) garden with a real event written 2026-08-20, then chmod 0555 on the directory and 0444 on the log -- appends now fail with permission denied -- and `hugel yield --health` prints `event log:  healthy` / `last write: 2026-08-20 10:00 (20d ago)`, with JSON `\"healthy\": true, \"reachable\": true`; (b) fresh read-only garden, in-process Emit returns `open event log: ...: permission denied` while HealthOf returns {Healthy:true Reachable:true LastWrite:<nil> FailingSince:<nil>} and the CLI prints `event log:  healthy` / `last write: never -- nothing has been recorded yet`. Case (a) is SC-3's stated distinction answered backwards: \"nothing has run since 2026-08-20\" reported when the truth is \"writes have been failing since 2026-08-20\". The phase's own artifacts already forbid this: 01-04-PLAN.md:326-329 instructs that reading two absences as \"healthy, nothing has run\" \"would be the exact wrong answer to the question SUB-03 asks\", and 01-RESEARCH.md:475-484 prescribes that this degradation must produce \"'no answer available' rather than a wrong answer\". The delivered code applies that rule only on the read axis (`!st.IsDir()` on $HUGEL_HOME, events.go:363) and not on the write axis. By the same mechanism the full-disk case named in SC-1 is expected to behave identically -- markFailing's O_CREATE|O_EXCL needs a new inode -- though that one is inferred, not observed (ENOSPC is not portably reproducible on macOS)."
    artifacts:
      - path: "internal/events/events.go"
        issue: "HealthOf (events.go:345-406) never establishes that the garden is writable. Its only failure signal is the events.failing-since marker, which markFailing (events.go:179-192) cannot create when the garden directory itself is unwritable -- the one state where writes are certainly failing. Healthy therefore computes to true from two absences at events.go:404."
      - path: "internal/cli/yield.go"
        issue: "showHealth (yield.go:345-376) renders that answer verbatim as `event log:  healthy`; the link is correct, the data it surfaces is not."
      - path: "internal/events/events_test.go"
        issue: "No test constructs an unwritable-but-readable garden and calls HealthOf. TestHealthOfRefusesToGuessWhenTheGardenCannotBeRead covers the unreadable garden; the two new IsRegular tests cover non-regular paths; the writable-directory axis is untested, which is why the gap survived the code review, the previous verification, and 19/19 UAT."
    missing:
      - "A writability check in HealthOf before it reports Healthy: probe the garden directory for write access (e.g. golang.org/x/sys/unix.Access(home, unix.W_OK), or an O_CREATE|O_EXCL temp-file probe that is removed immediately) and demote to Reachable=false / an explicit unknown rather than Healthy=true when the garden cannot be written. Emit must keep writing nothing extra; the probe belongs to the health reader."
      - "A regression test constructing a readable-but-unwritable garden (os.Chmod(home, 0o555), skipped or asserted-non-root per 01-RESEARCH.md:513-532 since root bypasses the mode bits) that asserts HealthOf reports not-healthy both before any Emit and after a real Emit failure -- the two states reproduced above."
      - "Either close the gap or record the limitation where a gardener will read it: 01-RESEARCH.md required the degraded case be documented in the plan's success criteria, and neither README.md's `hugel yield --health` line (README.md:34) nor showHealth's output says the answer is only trustworthy while the garden is writable."
deferred: []
---

# Phase 1: Trustworthy Event Writes Verification Report

**Phase Goal:** A gardener can trust the event log — silence means nothing ran, not that writes have been failing unseen
**Verified:** 2026-09-09T20:52:00Z
**Status:** gaps_found
**Re-verification:** Yes — after the uncommitted gap-closure work described in the previous report's Remediation section

## Method

The tree was measured, not read. Every claim below rests on a command run against the working
tree in this session: `go build`, `go vet`, `go test -count=1 ./...`, `go test -race` on the five
touched packages, three named-test runs, three mutation tests (guard/flush stripped, test observed
to fail, source restored and SHA-checked back to `ac21671820ec0e70f43d648985309517fcce946c`), and
seven end-to-end runs of a freshly built `cmd/hugel` binary against purpose-built gardens.
The previous report's Remediation section and 01-UAT.md's 19/19 were read as claims and then
re-tested from scratch; both survived re-testing on the point they addressed.

## Goal Achievement

### Observable Truths

Truths are the four ROADMAP.md Success Criteria for this phase (the roadmap contract). The five
plans' `must_haves.truths` were checked against them and add detail without reducing scope.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | With the event file made unwritable (read-only directory, bad `HUGEL_HOME`, full disk), the gardener sees the failure reported by the command that emitted the event, instead of the command reporting success | ✓ VERIFIED | `Emit` (events.go:224-269) returns a wrapped error on all four write-path failures. All **10 of 10** production call sites check it — `grep -c 'events\.Emit('` excluding tests = 10, `grep -c 'if err := events\.Emit('` = 10, zero unchecked (`internal/gate/run.go` ×6 at 32/48/109/140/197/235, `internal/tender/start.go` ×3 at 117/128/343, `internal/cli/dispatch.go` ×1 at 174). Every site prints `hugel: event %q not recorded: %v` to stderr and returns nothing new. Behaviourally confirmed by two named tests run this session: `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` (PASS) and `TestATenderStopsEvenWhenTheLogCannotBeWritten` (PASS). Independently reproduced on the read-only-directory variant: `Emit` on a `0555` garden returns `open event log: ...: permission denied` rather than nil. The full-disk variant is covered by the unwritable-path proxy plus an unexecuted Linux human-check (01-02-PLAN). |
| 2 | An event written by a command that has returned survives a crash — the tail of the log is durable, not sitting in a buffer | ✓ VERIFIED | Materially stronger than at the previous pass. `Emit` calls `syncFile(f)` as a checked, non-deferred statement before its success return (events.go:264-267); `syncFile` is `(*os.File).Sync` (events.go:56), so production behaviour is unchanged by the seam. **Mutation-proven this session:** deleting the flush block leaves `TestEmitFlushesBeforeItReturns` failing (`flushes during one Emit = 0, want 1`) and `TestAFailedFlushIsReportedAndMarksTheGarden` failing (`Emit = nil error, want one`) — and, exactly as the new test comment claims, leaves `TestEmitIsDurableBeforeItReturns` **passing**, which is the honest reason the seam was added. The sync-failure branch (error wrapped as `sync event log`, garden marked failing) is now covered where it previously was not. Ceiling stated correctly and not overclaimed: the doc comment (events.go:216-222) scopes the guarantee to `fsync(2)` and names the macOS `F_FULLFSYNC` gap (golang/go#26650); 01-03-PLAN records this as a deliberate human-checked limit and UAT test 2 confirms the user explored and declined `F_FULLFSYNC`. |
| 3 | A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing "nothing has run since 2026-09-01" from "writes have been failing since 2026-09-01", without reading the code | ✗ FAILED | **The previously-reported defect is closed.** `HealthOf` now guards both stats with `st.Mode().IsRegular()` (events.go:373, 397) and demotes to `Reachable: false`. Verified end-to-end, not from the summary: with `$HUGEL_HOME/events.jsonl` created as a directory, the built binary prints `event log:  unknown -- garden could not be read at ...` and JSON `"healthy": false, "reachable": false` — where the previous pass observed `healthy` / `a few seconds ago`. Both new regression tests were mutation-proven load-bearing (neutering each guard reproduces the previous report's exact symptom). **A different instance of the same must-not survives.** In a read-only garden directory — a state Success Criterion 1 names explicitly — writes fail *and* the failure marker cannot be created, so health reports `healthy` off two absences. Reproduced twice through the real binary; see Behavioral Spot-Checks rows 8–9 and Gaps Summary. Three of the four states the design targets are answered correctly (rows 4–7); the fourth is answered backwards. |
| 4 | A failing event write is visible but never destroys the work it was instrumenting — gate and tender still complete, and the gardener is told | ✓ VERIFIED | Read all 10 sites this session. Every one uses `if err := events.Emit(...); err != nil { fmt.Fprintf(os.Stderr, ...) }`; no site returns, wraps or propagates the event error, and each surrounding function's return value is computed from bindings made before the `Emit` statement (`gate/run.go:118` `step(StageTest, err == nil, ...)` sits outside the `if` and reads the outer `err`; `dispatch.go:183` calls `beads.HandBack` unconditionally after the failed emit). The three `outcomeOf(err == nil)` / `err.Error()` sites resolve to the outer `err` by Go's rule that a short-var-decl's identifier enters scope at the *end* of the declaration — confirmed by reading, and by the two named tests passing. Behaviourally confirmed: `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` and `TestATenderStopsEvenWhenTheLogCannotBeWritten` both assert the real outcome is unchanged and that the lost event is named on stderr. |

**Score:** 3/4 truths verified (0 present, behavior-unverified)

### Deferred Items

None. `roadmap.analyze` was checked for all six phases: phases 2–6 cover wide events, relations,
the stored graph, the resident garden and the work loop. None of them names event-log health,
`HealthOf`, garden writability, or `hugel yield --health`. The gap below is not scheduled anywhere
later and is therefore a real gap, not a deferral.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/events/events.go` (`Emit`) | returns `error`; checked flush before the success return | ✓ VERIFIED | events.go:224-269; flush mutation-proven load-bearing |
| `internal/events/events.go` (`markFailing`/`clearFailing`) | sticky first-failure marker, cleared on the next success | ✓ VERIFIED | Plan's own acceptance greps re-run and all pass: `markFailing()` code-only count 5 (want 5), `clearFailing()` 2 (want 2), `os.O_CREATE\|os.O_EXCL\|os.O_WRONLY` 1 (want 1), `events.failing-since` 1 (want 1). `TestTheFailureMarkKeepsTheFirstFailuresTime`, `TestAFailedWriteMarksTheGardenAsFailing` PASS. **Limitation:** the marker lives in the garden it is trying to report on — see the gap. |
| `internal/events/events.go` (`HealthOf`) | a health reader that never reports a state it cannot establish | ⚠️ PARTIAL — present, wired, correct on 3 of 4 states | Non-regular-path fabrication fixed and mutation-proven. Unwritable-garden false positive remains. |
| `internal/gate/run.go` | all 6 `events.Emit` sites checked | ✓ VERIFIED | 6/6 checked, control flow untouched |
| `internal/tender/start.go` | 3 `events.Emit` sites checked | ✓ VERIFIED | Lines 117, 128, 343 |
| `internal/cli/dispatch.go` (`handBack`) | checks and reports before telling bd | ✓ VERIFIED | Line 174; `beads.HandBack` called unconditionally at 183 |
| `internal/cli/yield.go` (`showHealth`) | renders `events.HealthOf`, plain or JSON | ✓ VERIFIED and WIRED | All four documented renderings exercised end-to-end (spot-checks 4–7); inherits `HealthOf`'s remaining false positive |
| `internal/complete/spec.go` | `--health` completion entry | ✓ VERIFIED | `Name: "health"` at line 85; `spec_test.go` drift test passes |
| `internal/events/events_test.go` | regression tests for the closed defect | ✓ VERIFIED | Both new tests present and independently mutation-proven load-bearing |
| `README.md` | events/health contract documented | ⚠️ INCOMPLETE | `hugel yield --health` documented at README.md:34, but the unwritable-garden limitation 01-RESEARCH.md required to be on the record is documented nowhere a gardener reads |

`gsd_run query verify.artifacts` was run against all five plans: 18/19 pass. The single failure is
`internal/events/events.go` missing `if err := f.Sync(); err != nil` in 01-03-PLAN — stale plan
frontmatter, not a code gap. See Warnings.

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `internal/gate/run.go` | `internal/events/events.go` | checked `Emit`, stderr report, control flow unchanged | ✓ WIRED | Tool-verified; behaviourally confirmed by named test |
| `internal/tender/start.go` | `internal/events/events.go` | same pattern, 3 sites | ✓ WIRED | Tool-verified; behaviourally confirmed by named test |
| `internal/cli/dispatch.go` | `internal/beads/beads.go` | `beads.HandBack` still called after a failed event write | ✓ WIRED | Tool-verified; read-confirmed at dispatch.go:183. No dedicated test — a documented plan decision (`bd` is shelled), accepted at UAT test 1 |
| `internal/cli/yield.go` | `internal/events/events.go` | `showHealth` calls `events.HealthOf` | ✓ WIRED | Tool-verified; the link works, the data it carries can be wrong (Truth 3) |
| `internal/cli/yield.go` | `internal/complete/spec.go` | `--health` in the completion table | ✓ WIRED | Tool-verified both directions |
| `internal/events/events.go` | filesystem | write then a checked flush before `Emit` returns nil | ✓ WIRED (tool reports NOT_WIRED) | Tool looks for `f\.Sync\(\)`; the source now reads `syncFile(f)` with `syncFile = (*os.File).Sync`. Verified by reading **and** by mutation. Stale pattern in 01-03-PLAN, not a broken link |
| `internal/events/events.go (Emit)` | `$HUGEL_HOME/events.failing-since` | `markFailing` on every write-path failure, `clearFailing` on success | ✓ WIRED (tool cannot parse) | Tool rejects the parenthetical in `from:`. Verified by hand: 4 call sites inside the locked section plus 1 definition, 1 `clearFailing` on the success path |
| `internal/events/events.go (HealthOf)` | marker and log paths | `os.Stat` on both plus a reachability check | ⚠️ WIRED BUT INSUFFICIENT (tool cannot parse) | Both stats present and now guarded. The reachability check covers readability only; nothing establishes writability, which is the gap |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `internal/cli/yield.go` (`showHealth`) | `h` | `events.HealthOf()` — live `os.Stat` of `$HUGEL_HOME`, the log and the marker | Yes — confirmed live: creating/backdating real files changed the rendered output every time (spot-checks 4–9) | ✓ FLOWING |
| `internal/events/events.go` (`HealthOf`) | `h.LastWrite` | `os.Stat(events.jsonl).ModTime()`, now gated on `IsRegular()` | Yes; moved from `never` to the real timestamp after a real `Emit` (spot-check 10) | ✓ FLOWING |
| `internal/events/events.go` (`HealthOf`) | `h.FailingSince` | `os.Stat(events.failing-since).ModTime()`, now gated on `IsRegular()` | Yes when the garden is writable; **structurally unobtainable when it is not** — the marker cannot be created in the directory whose unwritability it would report | ⚠️ STATIC in the unwritable case (a `nil` that always reads as "no failures") |
| `internal/events/events.go` (`HealthOf`) | `h.Healthy` | `h.Reachable && h.FailingSince == nil` (events.go:404) | Derived from the above; inherits the hole | ⚠️ HOLLOW in the unwritable case |

### Behavioral Spot-Checks

All run this session against the working tree. Rows 4–10 use a binary built from the tree
(`go build -o .../hugel ./cmd/hugel`) against purpose-built gardens; no real garden was touched.

| # | Behavior | Command | Result | Status |
|---|----------|---------|--------|--------|
| 1 | Build | `go build ./...` | exit 0 | ✓ PASS |
| 2 | Static check | `go vet ./...` | exit 0, no findings | ✓ PASS |
| 3 | Full suite, uncached, plus `-race` on the 5 touched packages | `go test -count=1 ./...` then `go test -race -count=1 ./internal/{events,cli,gate,tender,complete}` | all packages `ok`, no races | ✓ PASS (run once) |
| 4 | Health, fresh garden | `HUGEL_HOME=<empty> hugel yield --health` | `event log:  healthy` / `last write: never -- nothing has been recorded yet` | ✓ PASS |
| 5 | Health, written garden | log backdated to 2026-09-01 | `event log:  healthy` / `last write: 2026-09-01 10:00 (8d ago)` | ✓ PASS |
| 6 | Health, failing streak — SC-3's exact distinction | marker backdated 2026-09-01, log 2026-08-20 | `event log:  FAILING since 2026-09-01 10:00 (8d)` / `last write: 2026-08-20 10:00 (20d ago)`; JSON `healthy:false` with both dates | ✓ PASS |
| 7 | Health, unreadable garden | `HUGEL_HOME` is a regular file | `unknown -- garden could not be read at ...` on both lines, exit 0 | ✓ PASS |
| 8 | **Previous report's repro** — `events.jsonl` as a directory, never written | `mkdir -p $HUGEL_HOME/events.jsonl; hugel yield --health` | `event log:  unknown -- garden could not be read at ...`; JSON `healthy:false, reachable:false`. Previously `healthy` / `a few seconds ago` | ✓ PASS — defect closed |
| 9 | **New repro** — previously-healthy garden goes read-only | log written 2026-08-20, then `chmod 0555` dir + `0444` log; append confirmed to fail with `permission denied`; then `hugel yield --health` | `event log:  healthy` / `last write: 2026-08-20 10:00 (20d ago)`; JSON `"healthy": true, "reachable": true` | ✗ FAIL — writes failing 20 days, reported healthy |
| 9b | **New repro** — fresh read-only garden, in-process | `os.Chmod(home, 0o555)`, then `Emit` then `HealthOf` | `Emit err = open event log: ...: permission denied`; `Health = {Healthy:true Reachable:true LastWrite:<nil> FailingSince:<nil>}`; CLI prints `healthy` / `never -- nothing has been recorded yet` | ✗ FAIL — same class, asymmetry between an honest `Emit` and a dishonest `HealthOf` |
| 10 | `--health` last-write moves after a real emit (01-05 human-check, synthetic half) | `HealthOf` before/after a real `Emit` on a temp garden, then the CLI | `never` → `2026-09-09 15:46 (0s ago)`; log line present on disk | ✓ PASS (synthetic garden; the real-garden half stays a human item) |
| M1 | Mutation: neuter the log-path `IsRegular` guard | `go test -run TestHealthOfWillNotCallADirectoryAWrittenLog` | FAIL with `Healthy = true`, `Reachable = true`, `LastWrite = <dir mtime>` — the previous report's exact symptom | ✓ Guard is load-bearing |
| M2 | Mutation: neuter the marker `IsRegular` guard | `go test -run TestHealthOfWillNotTakeAFailureDateFromSomethingItDidNotWrite` | FAIL with `FailingSince = <dir mtime>` | ✓ Guard is load-bearing |
| M3 | Mutation: delete `Emit`'s flush | three named tests | `TestEmitFlushesBeforeItReturns` FAIL, `TestAFailedFlushIsReportedAndMarksTheGarden` FAIL, `TestEmitIsDurableBeforeItReturns` **PASS** | ✓ Seam is load-bearing, and the stated reason for adding it is true |

Source was restored after every mutation and re-hashed to `ac21671820ec0e70f43d648985309517fcce946c`;
all ad hoc probe files were deleted. `git status` on `internal/events/` shows only the two files
that were already modified before this verification began.

### Probe Execution

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| — | `find scripts -path '*/tests/probe-*.sh'` | no matches; no probe paths declared in any PLAN or SUMMARY | SKIPPED (no probes in this project) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SUB-01 | 01-01, 01-02, 01-03 | `events.Record` (`Emit`) returns an error instead of swallowing it | ✓ SATISFIED | `Emit` returns `error`; 10/10 production sites check it; `TestEmitReturnsAnErrorWhenTheLogCannotBeWritten` PASS; end-to-end gate and tender tests PASS |
| SUB-02 | 01-03 | Event writes are flushed durably | ✓ SATISFIED | Checked flush before the success return, now held by two mutation-proven tests rather than a grep; the sync-failure branch is covered; `Timer.Done` returns `Emit`'s error (events.go:293-311); durability claim scoped honestly to `fsync(2)` |
| SUB-03 | 01-04, 01-05 | A gardener can distinguish "nothing has run since \<date\>" from "writes have been failing since \<date\>" without reading the code | ✗ BLOCKED | The surface exists, is wired, discoverable and correct in three of four states — but reports the opposite of the truth in an unwritable garden, which is the requirement's own failure mode. See the gap |

No orphaned requirements: REQUIREMENTS.md maps exactly SUB-01/02/03 to Phase 1 (lines 78–80, 112),
and all three appear across the five plans' `requirements:` frontmatter (01-01 `[SUB-01]`,
01-02 `[SUB-01]`, 01-03 `[SUB-01, SUB-02]`, 01-04 `[SUB-03]`, 01-05 `[SUB-03]`). No plan claims a
requirement outside that set.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/events/events.go` | 179-192, 345-406 | `HealthOf` establishes readability but never writability; its only failure signal is a marker that cannot be written in the state it is meant to report | 🛑 Blocker | Causes Truth 3 / SUB-03 to fail. Reproduced end-to-end twice |
| `.planning/.../01-03-PLAN.md` | `must_haves.artifacts`, `must_haves.key_links` | Asserts `if err := f.Sync(); err != nil` and pattern `f\.Sync\(\)`; the source now reads `syncFile(f)` after the seam refactor. `verify.artifacts` and `verify.key-links` both fail on it | ⚠️ Warning | Traceability drift only — the underlying truth is now held *more* strongly than the literal it asserts. The plan contract should be updated or overridden so the drift is recorded rather than silently carried |
| `.planning/.../01-03-SUMMARY.md` | 116, 127, 144, 150, 202 | Prose still describes the call as `f.Sync()`; line 202's verification note (`one call (f.Sync())`) is now false | ⚠️ Warning | The session that made the fix updated the machine-readable `coverage.ref` block but left the human-readable prose stale. Cosmetic, but it is the kind of drift that makes a later reader trust the wrong artifact |
| `internal/gate/run.go` 109, 197; `internal/tender/start.go` 117 | — | `outcomeOf(err == nil)` / `err.Error()` read the outer `err` from inside the initializer of the `if err := events.Emit(...)` that shadows it (WR-01) | ⚠️ Warning — acceptable to leave open | Independently re-confirmed correct today by reading and by the two passing named tests. Tracked as bead `hugel4-k65` (OPEN, P3) with an accurate description, the complete site list, the reasoning, and a one-line-per-site fix. **Judged acceptable for this phase:** no present misbehaviour, no user-visible effect, explicitly carried past UAT with the risk written down rather than forgotten. It should not be carried past Phase 2, which widens exactly these event payloads and so raises the cost of a silent inversion |
| `README.md` | 34 | `hugel yield --health` is documented; the condition under which its answer is untrustworthy is not | ⚠️ Warning | 01-RESEARCH.md:475-484 required this limitation to be on the record. Folded into the gap's `missing` list |
| `.planning/.../01-05-SUMMARY.md` | coverage `kind` | Three entries changed `command` → `other` | ℹ️ Info | Cosmetic schema conformance; no behavioural claim, ref or status changed. All three refs re-checked and still accurate |

No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` markers in any of the 13 files this phase
modified (scanned individually).

**On the edited coverage ref in 01-03-SUMMARY.md** — scrutinised as instructed. The previous ref
was `grep -c 'if err := f.Sync(); err != nil' ... (must be 1)`, which the seam refactor would have
broken. It was replaced by two named tests. This is legitimate upkeep and a strict strengthening,
not papering over: the grep asserted a spelling, the tests assert the behaviour, and **both
replacement tests were independently mutation-proven load-bearing in this session** (M3). The
sibling ref `grep -c 'sync event log: %w' ... (must be 1)` was left in place and still returns 1.
The one thing the edit missed was propagating the same change into 01-03-PLAN's `must_haves` and
into its own prose — recorded above as Warnings.

### Human Verification Required

Recorded but not blocking; the gap above takes precedence and these should be re-offered at the
end of gap closure.

1. **Dogfood `--health` against the real garden.** Run `hugel yield --health` with no `HUGEL_HOME`
   override, then run a real gate or `hugel tender`, then run it again. Expected: the last-write
   time moves. *Why human:* no test may go near the gardener's own garden. The mechanism is proven
   on a synthetic garden (spot-check 10) and 01-05-SUMMARY records a partial read-only dogfood;
   the write half is unexecuted.
2. **Full-disk case on Linux.** Point `HUGEL_HOME` at a small full tmpfs, run `hugel gate` against
   a finished tender, and confirm the gate still reaches its verdict while the `not recorded`
   notices name the lost events. *Why human:* ENOSPC is not portably reproducible on macOS.
   *Note:* by the mechanism in the gap, this run is also expected to expose the health false
   positive — `markFailing`'s `O_CREATE|O_EXCL` needs a new inode and will fail under ENOSPC — so
   it is worth running `hugel yield --health` immediately afterwards and recording what it says.

### Gaps Summary

**The gap the previous pass named is genuinely closed.** CR-01 and IN-01 are fixed at
`events.go:373` and `events.go:397`, the two regression tests exist, and — measured rather than
taken on trust — neutering either guard reproduces the previous report's exact symptom, while the
end-to-end repro that once printed `healthy` / `a few seconds ago` now prints
`unknown -- garden could not be read`. SUB-02 also came out of this round stronger than it went in:
a durability claim previously held by a grep is now held by two tests that fail when the flush is
deleted. The remediation was real work, honestly described, and it survived independent re-testing.

**It did not close the class.** The must-not that CR-01 violated is stated twice in this phase's
own artifacts — 01-04-PLAN.md:326-329 ("reading those two absences as 'healthy, nothing has run'
would be the exact wrong answer to the question SUB-03 asks") and 01-RESEARCH.md:475-484 (this
degradation must yield "'no answer available' rather than a wrong answer"). The fix applies that
rule on the *read* axis only. On the *write* axis it is still violated, in a filesystem state
Success Criterion 1 names by name.

The mechanism is structural rather than incidental. `HealthOf` has exactly one way to learn that
writes are failing: the `events.failing-since` marker. `markFailing` creates that marker *inside
the garden directory* and discards its own error by design (events.go:187-191, with a doc comment
that names the chicken-and-egg honestly). So in the one state where writes are certainly failing
because the directory is unwritable, the marker cannot exist — and `HealthOf` reads its absence as
the absence of failures. `Healthy` computes to `true` at events.go:404 from two absences, which is
the sentence 01-04-PLAN wrote down as the thing not to do.

What the gardener sees, reproduced through the real binary on a garden that was healthy until the
directory went read-only:

```
event log:  healthy
last write: 2026-08-20 10:00 (20d ago)
```

Writes had been failing for those twenty days. That is not a near-miss on Success Criterion 3 —
it is Success Criterion 3's own worked example ("nothing has run since \<date\>" vs "writes have
been failing since \<date\>") answered with the wrong one of the two. And it is the phase goal,
inverted, on the single surface the phase built to prevent it: silence is being reported as
"nothing ran" while writes have been failing unseen.

Two things keep this from being catastrophic, and they are why the phase is close rather than
far. First, Success Criterion 1's channel still works: the gate or tender that could not write
does tell the gardener on stderr, at the moment it happens — verified. 01-RESEARCH.md:475-484 was
right that this is the load-bearing channel. Second, three of the four states `HealthOf` targets
are answered correctly, including the failing-streak case that is the common one whenever the
garden itself remains writable. The retrospective surface is wrong only when the failure is
directory-wide.

But "the retrospective surface is wrong when the failure is directory-wide" is not a footnote for
a phase whose entire purpose is that the retrospective surface can be trusted. The research
anticipated this and set the bar at *no answer* rather than a wrong answer; the code clears that
bar for unreadable gardens and not for unwritable ones. The fix is small and symmetric with what
is already there: probe the garden for write access in `HealthOf` and demote to unknown, exactly
as the `!st.IsDir()` and `!IsRegular()` demotions above it already do.

**If this is judged out of scope for Phase 1** — a defensible reading, given that SC-1's stderr
channel does fire and that no reviewer, verifier or UAT session caught it — then it should be an
explicit, recorded decision rather than a silent pass, and the limitation belongs in README.md
next to the `--health` line so a gardener reads it before trusting the answer. To accept it, add
to this file's frontmatter:

```yaml
overrides:
  - must_have: "A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing \"nothing has run since 2026-09-01\" from \"writes have been failing since 2026-09-01\", without reading the code"
    reason: "The unwritable-garden false positive is accepted as a known limitation for this milestone: SC-1's stderr channel still reports the failure at the moment it happens, and the retrospective surface is correct whenever the garden directory remains writable. Documented in README.md and filed as <bead>."
    accepted_by: "Charles Harris"
    accepted_at: "<ISO timestamp>"
```

---

_Verified: 2026-09-09T20:52:00Z_
_Verifier: Claude (gsd-verifier)_
