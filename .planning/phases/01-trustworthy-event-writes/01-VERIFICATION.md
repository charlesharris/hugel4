---
phase: 01-trustworthy-event-writes
verified: 2026-09-09T21:20:00Z
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
  - "go.mod"
  - "internal/cli/dispatch.go"
  - "internal/cli/main_test.go"
  - "internal/cli/yield.go"
  - "internal/cli/yield_test.go"
  - "internal/complete/spec.go"
  - "internal/config/sandbox.go"
  - "internal/events/events.go"
  - "internal/events/events_test.go"
  - "internal/events/writable_other.go"
  - "internal/events/writable_unix.go"
  - "internal/gate/gate_test.go"
  - "internal/gate/run.go"
  - "internal/tender/start.go"
  - "internal/tender/tender_test.go"
covered_digest: "v1:sha256:1ab24fa74465ffa49734f19b7433a0811fe72f981235191d83a5ce656eb44875"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 3/4
  gaps_closed:
    - "The two permission-denied instances the previous pass named are closed and mutation-proven: an unwritable garden directory with no log, and an unwritable directory holding an unwritable log, both now render `unknown` instead of `healthy`. Independently re-measured through a binary built from this tree."
  gaps_remaining:
    - "Truth 3 / SUB-03 remains FAILED. The fix closes the permission-denied instances of the class and leaves the ENOSPC instance open. `unix.Access(W_OK)` tests permission bits, not free space, so on a full disk the probe returns true and health reports `healthy` while every write is failing. Measured directly this pass on a purpose-built full filesystem — no longer inferred."
    - "01-03-PLAN.md `must_haves.key_links.pattern` is still `f\\.Sync\\(\\)`. The session updated the `artifacts.contains` assertion and the interface table but not the key-link pattern; `verify.key-links` still fails on 01-03."
  regressions:
    - "NEW, measured against a binary built from `main`: a garden directory that does not exist yet now renders `unknown -- garden at <path> could not be read or written`, where `main` correctly rendered `healthy` / `never -- nothing has been recorded yet`. Emit into that same path succeeds (it MkdirAll's), so the sentence the gardener is shown is false. This is the default state of a fresh install (`~/.hugel` before first run) and it removes SC-3's `nothing has run` reading from the exact state where it is the truth."
    - "NEW, measured against `main`: a garden whose log is unwritable but whose directory is writable — where markFailing CAN and DOES leave its marker — now renders `unknown` instead of `FAILING since 2026-09-01 (8d) / last write: 2026-08-20 (20d ago)`. The probe runs before the marker stat, so a correct, dated answer that the pre-fix code delivered is discarded. That answer is SC-3's own worked example."
    - "None in build or test: go build, go vet, GOOS=windows/linux/js cross-builds, go test -count=1 ./... and go test -race on the five touched packages are all clean; go mod tidy is a no-op."
gaps:
  - truth: "A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing \"nothing has run since 2026-09-01\" from \"writes have been failing since 2026-09-01\", without reading the code"
    status: failed
    reason: "The writability probe added this round closes the permission-denied instances of the defect class and leaves the ENOSPC instance wide open, while breaking two states that answered correctly before. (1) FULL DISK — measured, not inferred. On a purpose-built 2 MB HFS+ volume filled to 100%, with an existing log: `Emit` returns `write event: ...: no space left on device`; `markFailing` cannot create its marker (confirmed absent); `writable(home)` and `writable(log)` BOTH return true, because `unix.Access(W_OK)` tests permission bits and knows nothing about free space; `HealthOf` returns `{Healthy:true Reachable:true}` and the built binary prints `event log:  healthy` / `last write: 2026-08-20 10:00 (20d ago)`. That is the original defect, unchanged, in a filesystem state Success Criterion 1 names by name. (2) FRESH GARDEN REGRESSION — a garden directory that does not yet exist now prints `unknown -- garden at <path> could not be read or written`, a false sentence: `Emit` into that same path returns nil and creates the garden. `main` printed `healthy` / `never -- nothing has been recorded yet`. This is a fresh install's first run of the phase's flagship command, and it contradicts HealthOf's own `os.IsNotExist -> Reachable = true` branch forty lines above it (events.go:359-360). (3) FAILING-STREAK REGRESSION — with the log unwritable and the directory writable, the marker exists and dates the streak, but the probe returns before the marker stat, so `FAILING since 2026-09-01 (8d) / last write: 2026-08-20 (20d ago)` on `main` becomes `unknown` on HEAD. SC-3's second reading is thrown away with the data in hand."
    artifacts:
      - path: "internal/events/events.go"
        issue: "The probe at events.go:409-415 answers the wrong question. `unix.Access(W_OK)` establishes permission, not writability: a full filesystem, a quota-exhausted user, an inode-exhausted filesystem and an immutable-flagged file all pass it while every write fails. Measured on a full filesystem: probe true, Emit ENOSPC, marker absent, Healthy true."
      - path: "internal/events/events.go"
        issue: "The probe runs at events.go:409 before the failure-marker stat at events.go:417, so a demotion to unknown pre-empts a correct, dated FAILING answer that the marker already holds. Reordering the probe after the marker read would preserve it."
      - path: "internal/events/events.go"
        issue: "When the garden directory does not exist, `writable(home)` gets ENOENT and returns false, demoting a garden that Emit would create and write. This silently overrides the deliberate `case os.IsNotExist(err): h.Reachable = true` at events.go:359 and its comment (`a garden that has not been created yet -- fine, and still reachable`)."
      - path: "internal/events/events_test.go"
        issue: "No test covers a garden directory that does not exist (`TestHealthOfSaysNothingHasRunInAFreshGarden` uses `t.TempDir()`, which exists), and no test covers a writable-but-full filesystem. `TestHealthOfWillNotCallAnUnwritableLogHealthy` asserts only `!Healthy`, so it passes whether the answer is the informative `FAILING since` or the lossy `unknown` — it cannot see the regression."
      - path: "README.md"
        issue: "README.md:40-47 now states `--health reports unknown for any garden it could not read *or* write. A healthy answer therefore means writes were landing, not merely that no failure was found.` Measured false on a full disk (healthy, writes failing) and false by construction on any non-unix platform, where `writable()` returns true unconditionally (internal/events/writable_other.go:10)."
      - path: "internal/events/writable_other.go"
        issue: "The `!unix` build returns true unconditionally, keeping the pre-fix false-healthy behaviour on Windows and plan9. Honestly explained in the file's doc comment, but recorded nowhere a gardener reads, while the README states the guarantee unconditionally."
    missing:
      - "A writability signal that survives ENOSPC. `unix.Access` cannot provide it. Options: a real O_CREATE|O_EXCL probe file written and immediately removed (which the current comment rejects because a read-only report would write to the garden — a defensible objection worth re-deciding now that the cheap probe is proven insufficient); or, better, stop trying to answer the write axis from the read path at all and give Emit a durable place to record its own failure that does not depend on the garden it is reporting on."
      - "Restore `healthy / never -- nothing has been recorded yet` for a garden directory that does not exist. The probe must not fire on ENOENT — that is the state HealthOf already decided is reachable, and Emit creates it."
      - "Move the writability probe after the failure-marker read, so a garden that has a dated marker reports `FAILING since <date>` rather than discarding it for `unknown`."
      - "A regression test for each of the three states above: garden dir absent (expect healthy/never), log unwritable with a writable dir and a marker present (expect FailingSince set), and a full filesystem (expect not-healthy — provokable with a small ram disk mounted under /tmp so config.Sandbox accepts it, as this verification did)."
      - "Correct README.md:40-47 so it does not claim more than the probe delivers, and record the non-unix limitation where a gardener will read it."
      - "Fix the stale `pattern: \"f\\\\.Sync\\\\(\\\\)\"` in 01-03-PLAN.md `must_haves.key_links` — the sibling `artifacts.contains` was updated to `syncFile(f)` but this was not, and `verify.key-links` still fails 0/1 on that plan."
deferred: []
---

# Phase 1: Trustworthy Event Writes Verification Report

**Phase Goal:** A gardener can trust the event log — silence means nothing ran, not that writes have been failing unseen
**Verified:** 2026-09-09T21:20:00Z
**Status:** gaps_found
**Re-verification:** Yes — third pass, after the four commits on `fix/phase-01-event-write-trust`

## Method

Nothing below rests on a commit message, a SUMMARY bullet or the previous report. Every claim was
re-measured against the tree in this session: `go build`, `go vet`, `GOOS=windows/linux/js`
cross-builds, `go mod tidy`, `go test -count=1 ./...`, `go test -race` on the five touched
packages, six named-test runs, two mutation tests, an in-process probe matrix over six filesystem
states, a side-by-side comparison against a binary built from `main` in a throwaway worktree, and
a purpose-built full filesystem (a 2 MB HFS+ ram disk mounted under `/private/tmp` so
`config.Sandbox` would accept it). Source was restored and re-hashed to
`0835167d66df66536b8687a9dde9183029f806c2` after every mutation; the worktree, ram disk and all
probe files were removed. `git status` at the end shows only the two entries that were already
dirty when this pass began.

## The correction to the previous pass — confirmed

**The correction is right, and I was wrong.** Measured directly: `chmod 500` on a directory does
**not** prevent an append to a file already inside it. A shell append into a `dr-x------`
directory's existing `events.jsonl` succeeded; in-process, `Emit` into that state returns `nil`
and `HealthOf` returns `Healthy: true`, which is the correct answer. The directory's permission
bits govern creating and removing entries, not writing through to an existing file.

One clarification on what the previous pass actually did. Its row 9 repro chmod'd **both** the
directory (`0555`) **and** the log (`0444`), so the write failure it observed was real — but it
was caused by the log's mode, not the directory's, and the prose that summarised it as "a
read-only garden directory" mis-attributed the cause. The measurement was of a genuinely failing
state; the label on it was wrong, and the wrong label is what made the recommended fix aim at the
directory. The two failing states named in the correction are exactly the two I reproduced again
this pass (rows C and D below), and both are now fixed.

## Goal Achievement

### Observable Truths

Truths are the four ROADMAP.md Success Criteria (the roadmap contract). The five plans'
`must_haves.truths` were checked against them and add detail without reducing scope.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | With the event file made unwritable (read-only directory, bad `HUGEL_HOME`, full disk), the gardener sees the failure reported by the command that emitted the event, instead of the command reporting success | ✓ VERIFIED | `Emit` returns a wrapped error on all four write-path failures (events.go:243-268). 10 of 10 production call sites check it (`grep -c 'events\.Emit('` excluding tests = 10; `grep -c 'if err := events\.Emit('` = 10; gate ×6, tender ×3, dispatch ×1) and each prints `hugel: event %q not recorded: %v` to stderr. Named tests re-run and PASS: `TestEmitReturnsAnErrorWhenTheLogCannotBeWritten`, `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten`, `TestATenderStopsEvenWhenTheLogCannotBeWritten`. **The full-disk half is now measured rather than inferred:** on a filled filesystem `Emit` returns `write event: ...: no space left on device`. Emit is honest in every state tested, including the one where health is not. |
| 2 | An event written by a command that has returned survives a crash — the tail of the log is durable, not sitting in a buffer | ✓ VERIFIED | `Emit` calls `syncFile(f)` as a checked, non-deferred statement before its success return (events.go:264); `syncFile = (*os.File).Sync` (events.go:56), so production behaviour is unchanged by the seam. Re-mutation-proven this pass: deleting the flush block leaves `TestEmitFlushesBeforeItReturns` and `TestAFailedFlushIsReportedAndMarksTheGarden` red. The ceiling is stated honestly in the doc comment (`fsync(2)`, macOS `F_FULLFSYNC` gap, golang/go#26650), which satisfies 01-03's prohibition. |
| 3 | A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing "nothing has run since 2026-09-01" from "writes have been failing since 2026-09-01", without reading the code | ✗ FAILED | **Two instances closed, one open, two states newly broken.** Permission-denied gardens (dir unwritable with and without a log) now correctly report `unknown`, and the guard is mutation-proven. But `unix.Access(W_OK)` answers "may I write here", not "will a write succeed": on a **full disk** — a state SC-1 names by name — the probe returns true, the marker cannot be created, and the binary prints `event log:  healthy` / `last write: 2026-08-20 10:00 (20d ago)` while every write fails. Measured, not inferred. Separately, the probe now demotes two states that `main` answered correctly: a garden directory that does not exist (false `unknown`, where `Emit` would succeed) and a log-unwritable/dir-writable garden with a live marker (`unknown` in place of `FAILING since <date>`). See Behavioral Spot-Checks and Gaps Summary. |
| 4 | A failing event write is visible but never destroys the work it was instrumenting — gate and tender still complete, and the gardener is told | ✓ VERIFIED | Re-read all 10 sites this pass. Every one is `if err := events.Emit(...); err != nil { fmt.Fprintf(os.Stderr, ...) }`; no site returns, wraps or propagates the event error, and no surrounding return value is computed from it. Behaviourally confirmed: `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` and `TestATenderStopsEvenWhenTheLogCannotBeWritten` both PASS, asserting the real outcome is unchanged and the lost event is named on stderr. The three `outcomeOf(err == nil)` / `err.Error()` sites (WR-01) resolve to the outer `err` — re-confirmed by reading and by those two tests. |

**Score:** 3/4 truths verified (0 present, behavior-unverified)

### Deferred Items

None. `roadmap.analyze` was re-run for all six phases. Phases 2–6 cover wide events, relations,
the stored graph, the resident garden and the work loop; none names event-log health, `HealthOf`,
garden writability or `hugel yield --health`. The gap is scheduled nowhere later.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/events/events.go` (`Emit`) | returns `error`; checked flush before the success return | ✓ VERIFIED | events.go:243-268; flush re-mutation-proven load-bearing |
| `internal/events/events.go` (`markFailing`/`clearFailing`) | sticky first-failure marker, cleared on the next success | ✓ VERIFIED | `TestTheFailureMarkKeepsTheFirstFailuresTime`, `TestAFailedWriteMarksTheGardenAsFailing` PASS. Structural limitation unchanged: the marker lives in the garden it reports on, and cannot be created under ENOSPC (measured absent) |
| `internal/events/events.go` (`HealthOf`) | a health reader that never reports a state it cannot establish | ⚠️ PARTIAL — correct on 4 of 7 states measured | Non-regular-path and permission-denied instances fixed. Full-disk instance open; two previously-correct states newly demoted |
| `internal/events/writable_unix.go` | a non-writing writability probe | ⚠️ PRESENT, INSUFFICIENT | 18 lines, wired, single caller at events.go:413. `unix.Access(W_OK)` cannot see ENOSPC, quota or an immutable flag — measured returning true on a full filesystem while `Emit` failed |
| `internal/events/writable_other.go` | non-unix fallback | ⚠️ PRESENT, UNRECORDED LIMITATION | Returns `true` unconditionally, preserving the pre-fix false-healthy on Windows/plan9. `go list -f '{{.GoFiles}}'` confirms correct build-tag selection on both `GOOS=linux` and `GOOS=windows`; both cross-build and cross-vet clean. The limitation is explained in the file, but the README states the guarantee unconditionally |
| `internal/gate/run.go` | all 6 `events.Emit` sites checked | ✓ VERIFIED | 6/6 checked, control flow untouched |
| `internal/tender/start.go` | 3 `events.Emit` sites checked | ✓ VERIFIED | Lines 117, 128, 343 |
| `internal/cli/dispatch.go` (`handBack`) | checks and reports before telling bd | ✓ VERIFIED | Line 174; `beads.HandBack` called unconditionally after |
| `internal/cli/yield.go` (`showHealth`) | renders `events.HealthOf`, plain or JSON | ✓ VERIFIED and WIRED | Renders faithfully; inherits `HealthOf`'s remaining defect. The reworded unreachable line (`could not be read or written`) is now shown in a state where the garden *can* be written — see gap |
| `internal/complete/spec.go` | `--health` completion entry | ✓ VERIFIED | Drift test passes |
| `internal/events/events_test.go` | regression tests for the closed defects | ✓ VERIFIED for what they cover | Three new permission tests, all independently mutation-proven load-bearing (M1, M2). No coverage of ENOSPC, of an absent garden dir, or of the FailingSince-preservation the probe now breaks |
| `go.mod` | `golang.org/x/sys` promoted to direct | ✓ VERIFIED, LEGITIMATE | The only module change; `go.sum` untouched (v0.36.0 was already in the graph, so nothing new was downloaded). `go mod tidy` produces a zero diff. Promotion is required — `writable_unix.go` imports it directly |
| `README.md` | events/health contract documented | ✗ NOW INACCURATE | The paragraph added this round (README.md:40-47) claims more than the code delivers on a full disk and on non-unix |

`verify.artifacts` across all five plans: **19/19 pass** (was 18/19 — the `syncFile(f)` spelling was
corrected).

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `internal/gate/run.go` | `internal/events/events.go` | checked `Emit`, stderr report, control flow unchanged | ✓ WIRED | Tool-verified 1/1; behaviourally confirmed |
| `internal/tender/start.go` | `internal/events/events.go` | same pattern, 3 sites | ✓ WIRED | Tool-verified 2/2 |
| `internal/cli/dispatch.go` | `internal/beads/beads.go` | `beads.HandBack` still called after a failed event write | ✓ WIRED | Tool-verified; read-confirmed |
| `internal/cli/yield.go` | `internal/events/events.go` | `showHealth` calls `events.HealthOf` | ✓ WIRED | Tool-verified 2/2; the link works, the data it carries can be wrong |
| `internal/cli/yield.go` | `internal/complete/spec.go` | `--health` in the completion table | ✓ WIRED | Tool-verified both directions |
| `internal/events/events.go` | filesystem | write then a checked flush before `Emit` returns nil | ⚠️ WIRED, TOOL FAILS 0/1 | Source reads `syncFile(f)`; the plan's `key_links.pattern` is still `f\.Sync\(\)`. The session fixed the sibling `artifacts.contains` and missed this one. Link verified by reading and by mutation |
| `internal/events/events.go (HealthOf)` | `writable()` | non-writing writability probe before the health verdict | ⚠️ WIRED BUT WRONG QUESTION | Single caller at events.go:413, correct build-tag dispatch. The link is real; the predicate is insufficient |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `internal/cli/yield.go` (`showHealth`) | `h` | `events.HealthOf()` — live stats of `$HUGEL_HOME`, the log and the marker | Yes — output changed with every state built | ✓ FLOWING |
| `internal/events/events.go` (`HealthOf`) | `h.LastWrite` | `os.Stat(events.jsonl).ModTime()`, gated on `IsRegular()` | Yes | ✓ FLOWING |
| `internal/events/events.go` (`HealthOf`) | `h.FailingSince` | `os.Stat(events.failing-since).ModTime()`, gated on `IsRegular()` | Only when the garden is writable AND the probe lets execution reach the stat. Under ENOSPC the marker cannot exist (measured absent); in the log-unwritable/dir-writable state the marker exists but the probe returns first | ⚠️ STATIC under ENOSPC, unreachable in one state where it holds real data |
| `internal/events/events.go` (`HealthOf`) | `h.Healthy` | `h.Reachable && h.FailingSince == nil` (events.go:432) | Inherits both holes | ⚠️ HOLLOW under ENOSPC |
| `internal/events/writable_unix.go` | probe result | `unix.Access(path, W_OK)` | Real syscall, wrong predicate — measured `true` on a filesystem that refused every write | ⚠️ FLOWING BUT MISLEADING |

### Behavioral Spot-Checks

All run this session. Health rows use a binary built from this tree; the `main` column uses a
binary built from `main` in a throwaway worktree, for regression comparison.

| # | Behavior | Command | Result | Status |
|---|----------|---------|--------|--------|
| 1 | Build | `go build ./...` | exit 0 | ✓ PASS |
| 2 | Static check | `go vet ./...` | exit 0 | ✓ PASS |
| 3 | Cross-build | `GOOS=windows/linux/js go build`, `GOOS=windows/linux go vet` | all exit 0; `go list -f '{{.GoFiles}}'` selects `writable_unix.go` on linux, `writable_other.go` on windows | ✓ PASS |
| 4 | Module hygiene | `go mod tidy` then diff | zero diff; `go.sum` unchanged by the branch | ✓ PASS |
| 5 | Full suite + `-race` on 5 packages | `go test -count=1 ./...`, `go test -race ...` | all `ok`, no races | ✓ PASS (run once) |
| 6 | **Correction check** — dir `0500`, existing log, log writable | shell append + in-process `Emit` | append **succeeds**; `Emit` = nil; health `healthy` | ✓ PASS — the correction is right; this is not a failing state |
| 7 | Fresh garden, dir exists and is empty | `HUGEL_HOME=<empty dir> hugel yield --health` | `healthy` / `never -- nothing has been recorded yet` | ✓ PASS |
| 8 | Garden dir **does not exist** | `HUGEL_HOME=<nonexistent> hugel yield --health` | HEAD: `unknown -- garden at ... could not be read or written`. `main`: `healthy` / `never`. In-process: `Emit` into the same path returns **nil** and creates the garden | ✗ FAIL — REGRESSION, and a false sentence |
| 9 | Dir `0500`, no log | in-process `Emit` + `HealthOf`, then CLI | `Emit` = `permission denied`; `{Healthy:false Reachable:false}`; CLI `unknown` | ✓ PASS — previously-open gap closed |
| 10 | Dir `0500` + log `0400` | in-process `Emit` + `HealthOf` | `Emit` = `permission denied`; `{Healthy:false Reachable:false}` | ✓ PASS — previously-open gap closed |
| 11 | Log `0400`, dir writable, marker present | CLI, HEAD vs `main` | `main`: `FAILING since 2026-09-01 10:00 (8d)` / `last write: 2026-08-20 10:00 (20d ago)`. HEAD: `unknown`. Marker confirmed created (`stat` err nil) | ✗ FAIL — REGRESSION, a correct dated answer discarded |
| 12 | **Full disk (ENOSPC)** — 2 MB HFS+ ram disk at 100%, log present, appends confirmed failing | in-process `Emit` + `HealthOf`, then CLI | `Emit` = `write event: ...: no space left on device`; marker **absent**; `writable(home)=true writable(log)=true`; `Health = {Healthy:true Reachable:true}`; CLI prints `event log:  healthy` / `last write: 2026-08-20 10:00 (20d ago)` | ✗ FAIL — the original defect, unclosed, in a state SC-1 names by name |
| 13 | Named tests | gate, tender, events (5 tests) | all PASS | ✓ PASS |
| M1 | Mutation: force the probe to always use `home` | `go test -run TestHealthOfStillTrustsAWritableLogInAReadOnlyGarden` | FAIL: `Healthy = false, want true` | ✓ The over-correction guard test **is** load-bearing |
| M2 | Mutation: delete the whole writable probe | 3 named tests | `TestHealthOfWillNotCallAnUnwritableGardenQuiet` FAIL, `TestHealthOfWillNotCallAnUnwritableLogHealthy` FAIL, `TestHealthOfStillTrustsAWritableLogInAReadOnlyGarden` PASS | ✓ Probe is load-bearing and its guard test is correctly scoped |
| M3 | Mutation: delete `Emit`'s flush | 2 named tests | both FAIL | ✓ Seam still load-bearing |

### Probe Execution

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| — | `find scripts -path '*/tests/probe-*.sh'` | no matches; no probe paths declared in any PLAN or SUMMARY | SKIPPED (no probes in this project) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SUB-01 | 01-01, 01-02, 01-03 | `events.Record` (`Emit`) returns an error instead of swallowing it | ✓ SATISFIED | 10/10 sites check; error returned on permission-denied, is-a-directory, ENOSPC and a forced sync failure — all four observed this pass |
| SUB-02 | 01-03 | Event writes are flushed durably | ✓ SATISFIED | Checked flush before the success return, held by two mutation-proven tests; failure branch covered; claim scoped to `fsync(2)` |
| SUB-03 | 01-04, 01-05 | A gardener can distinguish "nothing has run since \<date\>" from "writes have been failing since \<date\>" without reading the code | ✗ BLOCKED | On a full disk the gardener is told `healthy / last write 20d ago` when writes have been failing for 20 days — the wrong one of SC-3's two readings. On a fresh install they are told `unknown` when the truth is `nothing has run`. In one state that used to name the failure date, they now get `unknown` |

No orphaned requirements. REQUIREMENTS.md maps exactly SUB-01/02/03 to Phase 1 (lines 78–80, 112),
and all three appear across the plans' `requirements:` frontmatter (01-01 `[SUB-01]`, 01-02
`[SUB-01]`, 01-03 `[SUB-01, SUB-02]`, 01-04 `[SUB-03]`, 01-05 `[SUB-03]`). No plan claims a
requirement outside that set.

### Prohibitions

| Plan | Statement | Tier | Disposition |
|------|-----------|------|-------------|
| 01-01, 01-02 | A failed event write must never change the outcome of the work it instruments | test | ✓ HELD — two named tests re-run and passing, all 10 sites re-read |
| 01-03 | Durability must never be claimed beyond what the platform provides | judgment | ✓ HELD — doc comment names the `fsync(2)` ceiling and the macOS `F_FULLFSYNC` gap |
| 01-04, 01-05 | The health surface must never report a state it cannot establish — when the garden cannot be read, it must say so rather than defaulting to healthy | test | ✗ VIOLATED, both ways. Under ENOSPC it defaults to healthy (measured). On an absent garden directory it asserts `could not be read or written` about a garden that can be written — also a state it has not established |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/events/writable_unix.go` | 16-17 | `unix.Access(W_OK)` used as a proxy for "a write will succeed"; it tests permission bits only | 🛑 Blocker | Measured true on a full filesystem while `Emit` returned ENOSPC and the marker could not be created — health reports `healthy`. This is Truth 3 / SUB-03's failure, in a state SC-1 names |
| `internal/events/events.go` | 409-415 | The probe fires on ENOENT, overriding the deliberate `os.IsNotExist -> Reachable = true` branch 50 lines above | 🛑 Blocker | A fresh install's first `hugel yield --health` prints a false sentence and loses SC-3's `nothing has run` reading |
| `internal/events/events.go` | 409 vs 417 | The probe returns before the failure-marker stat | ⚠️ Warning | Discards a correct, dated `FAILING since` answer in the one permission state where the marker survives — SC-3's own worked example |
| `README.md` | 40-47 | States the health guarantee unconditionally (`a healthy answer therefore means writes were landing`) | ⚠️ Warning | Measured false on a full disk and false by construction on non-unix. Documentation now overclaims where it previously underclaimed |
| `internal/events/writable_other.go` | 10 | `!unix` fallback returns `true` unconditionally | ℹ️ Info | Defensible (no CI, no Windows target, honest in-file comment), but combined with the README's unconditional claim it is an unrecorded limitation. Both `GOOS=windows` build and vet are clean, including tests — though `TestHealthOfWillNotCallAnUnwritableGardenQuiet` would fail if ever *run* there |
| `.planning/.../01-03-PLAN.md` | `must_haves.key_links.pattern` | Still `f\.Sync\(\)` after the seam refactor; `verify.key-links` fails 0/1 | ⚠️ Warning | Half-finished upkeep — the sibling `artifacts.contains` was updated in the same commit |
| `.planning/.../01-03-PLAN.md` | acceptance criteria | `test "$(grep -c 'defer f.Close()' internal/events/events.go)" = "1"` — actual count is **2** | ℹ️ Info — pre-existing, NOT this session's doing | **Independently confirmed**: `git show main:internal/events/events.go \| grep -c 'defer f.Close()'` also returns 2. The second is `Load`'s own close (events.go:453), unrelated to `Emit`. A bad assertion, not a code defect, and it was already failing before this branch |
| `internal/gate/run.go` 109, 197; `internal/tender/start.go` 117 | — | `outcomeOf(err == nil)` / `err.Error()` read the outer `err` from inside a shadowing `if err := events.Emit(...)` (WR-01) | ⚠️ Warning — **still acceptable to leave open** | Re-confirmed correct today by reading and by two passing tests. Tracked as `hugel4-k65` (OPEN, P3) with the complete site list and a one-line-per-site fix. Judged acceptable for this phase: no present misbehaviour, no user-visible effect, risk written down rather than forgotten. The judgement should not extend past Phase 2, which widens exactly these payloads |
| beads | — | No bead was filed for the writability fix (`hugel4-0kf` and `hugel4-q21` cover the other two, both closed) | ℹ️ Info | The work is committed and described; only the tracking record is missing |

No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` markers in any file this phase touched.

**On the 01-03-PLAN source-assertion edits — scrutinised as instructed.** This is legitimate,
disclosed upkeep, not history laundering. Three things make it so: the plan body carries an
explicit dated amendment (`NOTE (amended after phase 01 UAT): ... only its spelling moved. See
bead hugel4-0kf`), the bead exists with an accurate description, and the property asserted is
unchanged — one checked, non-deferred, non-discarded flush before the success return. It is also
a strict strengthening: a grep over a spelling was replaced by two tests that I re-proved fail
when the flush is deleted (M3). The one thing it left undone is the sibling `key_links.pattern`,
recorded above as a Warning.

### Human Verification Required

Not blocking; the gap takes precedence. Re-offer these at the end of gap closure.

1. **Dogfood `--health` against the real garden.** Run `hugel yield --health` with no `HUGEL_HOME`
   override, run a real gate or `hugel tender`, run it again. Expected: the last-write time moves.
   *Why human:* no test may go near the gardener's own garden. The mechanism is proven on synthetic
   gardens.

*Retired this pass:* the previous report's "full-disk case on Linux" human item is no longer a
human item — the state was built and measured directly on macOS (spot-check 12), and it produced
a failure rather than a confirmation.

### Gaps Summary

**What the round genuinely fixed.** The two permission-denied states the correction named are
closed. A garden directory that cannot be written, with or without a log, now prints
`unknown -- garden at <path> could not be read or written` where it printed `healthy`. Both new
tests are load-bearing under mutation, and — importantly — so is the over-correction guard:
`TestHealthOfStillTrustsAWritableLogInAReadOnlyGarden` genuinely fails when the probe is forced to
ask about the directory instead of the log, so the log-versus-directory distinction is pinned, not
decorative. The go.mod change is the only module change, `go.sum` is untouched, `go mod tidy` is a
no-op, and both `GOOS=windows` and `GOOS=linux` build and vet cleanly. The plan-assertion edits are
disclosed and beaded. That is real work, and the tree is in better shape than it was.

**Why the phase still cannot pass.** The fix treats a symptom of the class rather than the class.
`unix.Access(W_OK)` answers *may I write here*, and the question `HealthOf` needs answered is
*will a write succeed*. Those diverge precisely where SC-1 said they would. Built a 2 MB HFS+
volume, filled it to 100%, confirmed appends fail:

```
Emit err        = write event: /.../events.jsonl: no space left on device
marker present? = false
writable(home)  = true      writable(log) = true
Health          = {Healthy:true Reachable:true}
```

and through the binary:

```
event log:  healthy
last write: 2026-08-20 10:00 (20d ago)
```

That is the previous report's headline symptom, verbatim, on a garden whose every write is being
refused — and it is no longer an inference. "Full disk" is one of the three states Success
Criterion 1 lists by name, and it is the one a gardener is most likely to actually meet, since it
arrives without anyone changing a permission bit.

**And the fix took two correct answers away.** Measured against a binary built from `main`:

- A garden directory that does not exist now reads `unknown -- garden at ~/.hugel could not be
  read or written`. `main` read `healthy` / `never -- nothing has been recorded yet`. The new
  sentence is false — `Emit` into that path returns nil and creates the garden — and this is a
  fresh install's first run of the very command the phase exists to deliver. It also silently
  overrides `HealthOf`'s own `os.IsNotExist -> Reachable = true` branch, whose comment says a
  garden that has not been created yet is "fine, and still reachable". The probe and that branch
  now disagree, and the probe wins.
- A garden with an unwritable log but a writable directory — where `markFailing` *can* leave its
  marker, and does — now reads `unknown`. `main` read `FAILING since 2026-09-01 10:00 (8d)` /
  `last write: 2026-08-20 10:00 (20d ago)`. The probe runs before the marker stat, so a correct,
  dated answer already sitting on disk is thrown away. That answer is literally SC-3's worked
  example.

Neither regression is a lie in the dangerous direction, and neither is caught by the suite:
`TestHealthOfSaysNothingHasRunInAFreshGarden` uses `t.TempDir()`, which exists, and
`TestHealthOfWillNotCallAnUnwritableLogHealthy` asserts only `!Healthy`, which `unknown` satisfies.

**The shape of a real fix.** The structural problem is unchanged from the previous pass and was
not addressed: `HealthOf`'s only evidence of failure is a marker that `markFailing` writes *into
the garden it is reporting on*. Every attempt to patch that from the read side runs into the same
wall — a probe cheap enough not to write cannot see ENOSPC, and a probe that can see ENOSPC has to
write. The honest options are (a) accept a real O_CREATE|O_EXCL probe file, written and removed
immediately, and revisit the "a read-only report must not write" objection now that the cheap
probe is proven insufficient; or (b) give `Emit` somewhere to record its own failure that does not
depend on the garden — the option that actually closes the class. Either way the two regressions
above should be reverted first, since both come from the probe firing where it should not.

**On accepting instead of fixing.** If the milestone judges this out of scope, it must be an
explicit recorded decision, not a silent pass — and the README paragraph added this round has to
change either way, because it now claims a guarantee that a full disk and every non-unix platform
falsify. To accept, add to this file's frontmatter:

```yaml
overrides:
  - must_have: "A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing \"nothing has run since 2026-09-01\" from \"writes have been failing since 2026-09-01\", without reading the code"
    reason: "The ENOSPC false-healthy is accepted as a known limitation for this milestone: SC-1's stderr channel still reports the failure at the moment it happens, and the retrospective surface is correct for permission failures. README.md corrected to scope the claim; filed as <bead>."
    accepted_by: "Charles Harris"
    accepted_at: "<ISO timestamp>"
```

---

_Verified: 2026-09-09T21:20:00Z_
_Verifier: Claude (gsd-verifier)_
