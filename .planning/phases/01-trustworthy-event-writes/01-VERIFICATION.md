---
phase: 01-trustworthy-event-writes
verified: 2026-09-09T22:55:00Z
status: human_needed
score: 4/4 must-haves verified
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
covered_digest: "v1:sha256:2df09e9f8eb608bea1fd31fccf00ea73b164635800c24d2ac6dbd3803172257c"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: gaps_found
  previous_score: 3/4
  gaps_closed:
    - "ENOSPC false-healthy (the pass-3 headline gap) is closed. Re-measured on a purpose-built 4 MB HFS+ volume filled to 100%: unix.Statfs reports Bavail=0, writable() returns false, HealthOf returns {Healthy:false Reachable:false}, and the binary prints `unknown -- cannot confirm a write to <path> would land`. The pass-3 output `event log: healthy` / `last write: 20d ago` on a filesystem refusing writes could not be reproduced on this tree in any state built."
    - "REGRESSION CLOSED: a garden directory that does not exist now prints `healthy` / `never -- nothing has been recorded yet` again. writeTarget() walks to the nearest existing ancestor, which is what MkdirAll actually needs. Mutation-proven (M2)."
    - "REGRESSION CLOSED: a garden with an unwritable log, a writable directory and a dated marker now prints `FAILING since 2026-09-01 10:00 (8d)` / `last write: 2026-08-20 10:00 (20d ago)` again. The probe is now gated on `h.FailingSince == nil`, so a dated answer already in hand is never replaced by unknown. Mutation-proven (M1)."
    - "01-03-PLAN.md `must_haves.key_links.pattern` is now `syncFile\\(f\\)`. `verify.key-links` reports 1/1 verified for 01-03."
    - "Neither fix re-opened the states the probe exists to catch: unwritable dir with no log, unwritable log with no marker, and a non-regular log all still render `unknown`, and the over-correction guard (read-only dir holding a writable log) still renders `healthy`. All four re-measured through a binary built from this tree."
  gaps_remaining: []
  regressions:
    - "None found. go build, go vet, GOOS=linux/windows cross-build and cross-vet, and `go test -count=1 ./...` (19 packages) are all clean. Build-tag selection re-confirmed: writable_unix.go on linux, writable_other.go on windows."
gaps: []
deferred: []
advisory:
  - finding: "The plain-text renderer prints `last write: unknown` in states where Health.LastWrite is populated and --json prints it. Measured on the full filesystem: JSON emitted `\"last_write\": \"2026-09-09T17:40:06-05:00\"` while the text output said unknown on both lines."
    category: other
    reason: "New scope; not a carried-forward gap and not a lie in the dangerous direction (it withholds rather than fabricates). But SC-3 asks for `when it was last written`, and in the ENOSPC window the plain output is the only surface most gardeners will read. Resolution: print the known last-write date beside the unknown verdict, or say `last write: <date> (unconfirmed)`."
    evidence_status: "measured this pass (full-disk CLI run, plain vs --json)"
  - finding: "`unknown -- cannot confirm a write to <path> would land` is inaccurate in one state that reaches it: a non-regular file standing at $HUGEL_HOME/events.failing-since while the garden is fully writable. Measured: health says the write cannot be confirmed; a shell append to the log succeeds."
    category: other
    reason: "New scope, contrived state, safe direction (never false-healthy). The old wording (`could not be read or written`) was accurate here and the new wording is not, so the rewording traded one obscure inaccuracy for accuracy in the common states. Resolution: either widen the sentence for the read-side demotions or keep two sentences."
    evidence_status: "measured this pass (marker-is-a-directory state, CLI + shell append)"
  - finding: "README.md claims `A dated failure always wins... it never answers unknown over an answer it already has`. Falsified in one state: a dated marker on disk plus a non-regular log renders `unknown`, because the log stat demotes and returns before the marker is read."
    category: other
    reason: "New scope. The claim is absolute and a counter-example exists; a README that overstates is the defect class this phase exists to remove. Nine of the paragraph's ten claims verified true, including the free-space early-warning window and the read-only guarantee. Resolution: scope the sentence to a readable garden."
    evidence_status: "measured this pass (marker present + log is a directory)"
  - finding: "The unix.Statfs free-space check — the code that closes this phase's headline gap — is held by no test. Deleting the whole Statfs block and returning true leaves `go test ./internal/events` green."
    category: other
    reason: "New scope. The behaviour is verified by direct measurement this pass, but nothing stops a future refactor from removing it silently, which is exactly the argument that produced the syncFile seam in 01-03. Resolution: a test that fakes the statfs call through a package var (the syncFile pattern), since a real full filesystem cannot be built inside go test."
    evidence_status: "mutation M3 run this pass"
behavior_unverified_items: []
coincidental_reliance_items: []
human_verification:
  - test: "Run `hugel yield --health` with no HUGEL_HOME override, against the real ~/.hugel garden. Run a real gate or `hugel tender`. Run it again."
    expected: "First run reports healthy with a last-write time; after the real command, the last-write time has moved forward."
    why_human: "config.Sandbox panics if any test resolves the garden to a non-temporary directory, so no test may ever go near the gardener's own garden. Every measurement in this report was made on synthetic gardens and a purpose-built filesystem."
  - test: "Read the Emit doc comment at internal/events/events.go:208-223 and confirm the durability claim is no stronger than what the platform gives."
    expected: "The comment claims only that the data left the process and the OS accepted it, names fsync(2) rather than F_FULLFSYNC, and cites golang/go#26650."
    why_human: "judgment-tier prohibition (01-03). The verdict recorded below is a non-authoritative LLM judgement — unverified-prohibition, human review recommended."
---

# Phase 1: Trustworthy Event Writes Verification Report

**Phase Goal:** A gardener can trust the event log — silence means nothing ran, not that writes have been failing unseen
**Verified:** 2026-09-09T22:55:00Z
**Status:** human_needed
**Re-verification:** Yes — fourth pass, against `main` after the five fix commits

## Method

Nothing below rests on a commit message, a SUMMARY bullet, or the previous report — including
the previous report's own `gaps_remaining`, which describes a tree that no longer exists.
Re-measured this session: `go build`, `go vet`, `GOOS=linux`/`GOOS=windows` cross-build and
cross-vet, `go list -f '{{.GoFiles}}'` on both, `go test -count=1 ./...` once over 19 packages,
seven named-test runs, four mutation tests, thirteen garden states exercised through a binary
built from this tree, an in-process `Emit`/`HealthOf`/`unix.Access`/`unix.Statfs` probe, and a
purpose-built 4 MB HFS+ ram disk filled to 100% under `/private/tmp`. Every mutated file was
restored and re-hashed (`events.go` `2bffa333`, `writable_unix.go` `63dfc750`); the ram disk was
unmounted and detached, the probe program deleted, and every scratch garden removed. `git status`
at the end shows only the three entries that were already dirty when the pass began.

## The correction offered for refutation — CONFIRMED

The correction is right. On a 100%-full HFS+ volume (`df` capacity 100%, `unix.Statfs` reporting
`Bavail=0 Bfree=0 Bsize=4096`), measured in this order on the same volume:

```
72-byte append to the existing log   SUCCESS
65536-byte append to the same log    FAILED  [Errno 28] No space left on device
O_CREAT|O_EXCL on a new file         FAILED  [Errno 28] No space left on device
```

So `Bavail == 0` does not imply the next append fails. A small append lands or does not depending
on slack in the log's last allocated block, and the free-space check therefore reports `unknown` a
write or two early. Demonstrated end to end on the same volume: with a freshly truncated log,
`hugel yield --health` printed `unknown -- cannot confirm a write to <path> would land` while a
real `events.Emit` immediately afterwards **returned nil and appended the event to the log**.

Pass 3 is not refuted on its own facts — I also reproduced `Emit` returning
`write event: ...: no space left on device` at `Bavail=0`, once the last block had been consumed.
Both outcomes occur on the same filesystem minutes apart. What pass 3 got wrong was the
generalisation, not the measurement: it read one ENOSPC as proof that `Bavail == 0` means writes
fail, and it does not.

## Judgement on the early-warning window — acceptable, and not inconsistent

Stated plainly, as asked: **reporting `unknown` while a write can still land is the right call
here, and it is not the same thing as the over-demotion this session rejected for the read-only
directory.** Three differences carry the judgement:

1. **Permanence.** The read-only-directory demotion was permanent: that garden writes successfully
   forever and would have read `unknown` forever. So would a fresh install's uncreated garden. The
   `Bavail == 0` demotion lasts exactly as long as the disk is full, and clears the moment a block
   frees. It is a warning about a condition, not a standing misdescription of a healthy garden.
2. **Population.** A read-only garden directory holding an appendable log is an ordinary
   configuration, and an uncreated `~/.hugel` is *every* new gardener's first run. A filesystem
   sitting at exactly zero available blocks is not a steady state anyone lives in.
3. **Truth value of the sentence actually printed.** `cannot confirm a write to X would land` is
   literally true at `Bavail == 0` — my own measurement shows the next append is decided by slack
   in one block, which is precisely something hugel cannot confirm. The same sentence about a
   read-only directory holding a writable log would have been *false*: hugel can confirm that one,
   and the append lands every time.

The rejected over-correction was a false statement about a permanently healthy garden. This is a
true statement about a garden a couple of events from permanent failure, in a window where the
failures that follow cannot be marked at all because a marker needs an inode. Different call, same
principle. I would have made it the same way.

The one thing I would change is not the threshold but the rendering: in that window health still
holds a real last-write date and prints `unknown` about it (see Advisory 1).

## Goal Achievement

Truths are the four ROADMAP.md Success Criteria. The five plans' `must_haves.truths` were checked
against them and add detail without reducing scope.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | With the event file made unwritable (read-only directory, bad `HUGEL_HOME`, full disk), the gardener sees the failure reported by the command that emitted the event, instead of the command reporting success | ✓ VERIFIED | `Emit` returns a wrapped error on all four write-path failures (events.go:243-268). 10 of 10 production sites check it and print `hugel: event %q not recorded: %v` to stderr (`grep -c 'events\.Emit('` non-test = 10; `grep -c 'if err := events\.Emit('` = 10; gate ×6, tender ×3, dispatch ×1). Measured in process this pass: permission-denied → error, log-is-a-directory → error, **full disk → `write event: ...: no space left on device`**. Named tests re-run and PASS: `TestEmitReturnsAnErrorWhenTheLogCannotBeWritten`, `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten`, `TestATenderStopsEvenWhenTheLogCannotBeWritten`. |
| 2 | An event written by a command that has returned survives a crash — the tail of the log is durable, not sitting in a buffer | ✓ VERIFIED | `syncFile(f)` is a checked, non-deferred statement before the success return (events.go:264); `syncFile = (*os.File).Sync` (events.go:56), so production behaviour is unchanged by the seam. Re-mutation-proven this pass (M4): deleting the flush block leaves `TestEmitFlushesBeforeItReturns` and `TestAFailedFlushIsReportedAndMarksTheGarden` red. The ceiling is stated honestly in the doc comment (fsync(2), macOS F_FULLFSYNC gap, golang/go#26650). |
| 3 | A gardener can ask one question and learn whether the log is healthy and when it was last written, distinguishing "nothing has run since 2026-09-01" from "writes have been failing since 2026-09-01", without reading the code | ✓ VERIFIED | **Both pass-3 regressions closed, the ENOSPC gap closed, and no state was re-opened.** Measured through a binary built from this tree across 13 garden states (table below). The two SC-3 readings are both produced verbatim: `healthy` / `never -- nothing has been recorded yet` for a garden that has not been created, and `FAILING since 2026-09-01 10:00 (8d)` / `last write: 2026-08-20 10:00 (20d ago)` for a garden with a dated marker. **No unix state I could construct reports `healthy` while writes fail** — including a real full filesystem, which is the state that defeated the previous three passes. Both fixes are mutation-proven load-bearing (M1, M2). Three renderer/README precision issues recorded as Advisory; none of them produces a false `healthy`, and none prevents the two readings. |
| 4 | A failing event write is visible but never destroys the work it was instrumenting — gate and tender still complete, and the gardener is told | ✓ VERIFIED | All 10 sites re-read this pass. Every one is `if err := events.Emit(...); err != nil { fmt.Fprintf(os.Stderr, ...) }`; no site returns, wraps or propagates the event error, and no surrounding return value is computed from it. `TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten` and `TestATenderStopsEvenWhenTheLogCannotBeWritten` both PASS. WR-01's three sites re-traced against Go's scoping rule (a ShortVarDecl's identifier enters scope at the *end* of the decl, so `outcomeOf(err == nil)` and `err.Error()` inside the `Emit` argument list read the outer `err`). |

**Score:** 4/4 truths verified (0 present, behavior-unverified)

### Deferred Items

None. Phases 2–6 cover wide events, relations, the stored graph, the resident garden and the work
loop; `grep -i 'health|writab|ENOSPC|full disk'` over ROADMAP.md matches only inside Phase 1. Nothing
here is scheduled later.

### Advisory (New Scope, Unevidenced)

All four items below carry deterministic evidence measured this pass, and none is a Blocker — they
are recorded as Advisory because they are new scope, they never produce a false `healthy`, and none
of them blocks the phase goal.

| # | Finding | Category | Why Advisory |
|---|---------|----------|--------------|
| 1 | Plain text prints `last write: unknown` where `--json` prints a real `last_write` | other | new scope; withholds rather than fabricates; SC-3's two readings still available |
| 2 | `cannot confirm a write ... would land` is false in the marker-is-a-non-regular-file state (a write does land) | other | new scope; contrived state; errs toward unknown |
| 3 | README's "a dated failure always wins ... never answers unknown over an answer it already has" has a counter-example | other | new scope; overstates informativeness, not safety |
| 4 | The Statfs free-space check is held by no test (M3 deletes it, suite stays green) | other | new scope; behaviour verified by direct measurement, but unguarded against future refactors |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/events/events.go` (`Emit`) | returns `error`; checked flush before the success return | ✓ VERIFIED | events.go:243-268; flush re-mutation-proven (M4) |
| `internal/events/events.go` (`markFailing`/`clearFailing`) | sticky first-failure marker, cleared on the next success | ✓ VERIFIED | `TestTheFailureMarkKeepsTheFirstFailuresTime`, `TestAFailedWriteMarksTheGardenAsFailing` PASS. Structural limitation unchanged and now documented: the marker lives in the garden it reports on and cannot be created under ENOSPC (re-measured absent) |
| `internal/events/events.go` (`HealthOf`) | a health reader that never reports a state it cannot establish | ✓ VERIFIED — correct on 13 of 13 states measured | Probe now runs last, gated on `FailingSince == nil`, against `writeTarget()`. No state produces a false `healthy` |
| `internal/events/events.go` (`writeTarget`) | the path Emit's next write really needs permission on | ✓ VERIFIED and WIRED | events.go:437-452. Log if it exists, else the nearest existing ancestor. Single caller at events.go:418; mutation-proven (M2) |
| `internal/events/writable_unix.go` | a non-writing writability probe that survives ENOSPC | ✓ VERIFIED | 37 lines, wired, single caller. `unix.Access(W_OK)` then `unix.Statfs().Bavail > 0`. Measured returning **false** on the full filesystem that defeated pass 3. Statfs error deliberately falls through to `true` rather than manufacturing a failure — correct choice, and untested (Advisory 4) |
| `internal/events/writable_other.go` | non-unix fallback | ⚠️ PRESENT, LIMITATION DOCUMENTED | Returns `true` unconditionally. `go list -f '{{.GoFiles}}'` confirms tag selection (`writable_unix.go` on linux, `writable_other.go` on windows); both cross-build and cross-vet clean. Limitation now stated in the file **and** in README.md where a gardener reads. Judged sufficient — see below |
| `internal/gate/run.go` | all 6 `events.Emit` sites checked | ✓ VERIFIED | 6/6 checked, control flow untouched |
| `internal/tender/start.go` | 3 `events.Emit` sites checked | ✓ VERIFIED | Lines 117, 128, 343 |
| `internal/cli/dispatch.go` (`handBack`) | checks and reports before telling bd | ✓ VERIFIED | Line 174; `beads.HandBack` called unconditionally after |
| `internal/cli/yield.go` (`showHealth`) | renders `events.HealthOf`, plain or JSON | ✓ VERIFIED and WIRED | Renders faithfully in every state built. Two rendering imprecisions recorded as Advisory 1 and 2 |
| `internal/complete/spec.go` | `--health` completion entry | ✓ VERIFIED | Drift test passes |
| `internal/events/events_test.go` | regression tests for the closed defects | ✓ VERIFIED | Two new regression guards, both mutation-proven: `TestHealthOfCallsAnUncreatedGardenFreshRatherThanUnknown` (M2) and `TestHealthOfKeepsADatedFailureRatherThanAnsweringUnknown` (M1). No ENOSPC test — Advisory 4 |
| `internal/cli/yield_test.go` | render tests for four health states | ✓ VERIFIED | Four tests, and they assert behaviour (`not healthy`, path named) rather than the exact sentence, so the rewording did not require editing them |
| `go.mod` | `golang.org/x/sys` promoted to direct | ✓ VERIFIED, LEGITIMATE | Only module change across the phase; `go.sum` untouched (v0.36.0 was already in the graph). Required by `writable_unix.go`'s direct import |
| `README.md` | events/health contract documented | ✓ VERIFIED with one over-strong sentence | Nine of ten claims independently checked true, including the free-space early-warning window, the writes-nothing guarantee and the non-unix gap. One counter-example — Advisory 3 |

`verify.artifacts` across all five plans: **19/19 pass.**

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `internal/gate/run.go` | `internal/events/events.go` | checked `Emit`, stderr report, control flow unchanged | ✓ WIRED | Tool-verified 1/1 |
| `internal/tender/start.go` | `internal/events/events.go` | same pattern, 3 sites | ✓ WIRED | Tool-verified 2/2 |
| `internal/cli/dispatch.go` | `internal/beads/beads.go` | `beads.HandBack` still called after a failed event write | ✓ WIRED | Tool-verified |
| `internal/cli/yield.go` | `internal/events/events.go` | `showHealth` calls `events.HealthOf` | ✓ WIRED | Tool-verified 2/2; behaviourally confirmed across 13 states |
| `internal/cli/yield.go` | `internal/complete/spec.go` | `--health` in the completion table | ✓ WIRED | Tool-verified both directions |
| `internal/events/events.go` | filesystem | write then a checked flush before `Emit` returns nil | ✓ WIRED, TOOL 1/1 | **Previously 0/1.** `key_links.pattern` is now `syncFile\\(f\\)` and matches source; link also mutation-proven (M4) |
| `internal/events/events.go (HealthOf)` | `writable()` / `writeTarget()` | non-writing writability probe, last, and only when the garden has not already answered | ✓ WIRED and CORRECT | Single caller at events.go:418; `unix.Access` + `unix.Statfs`; both orderings mutation-proven (M1, M2) |
| `internal/events/events.go (Emit)` | `$HUGEL_HOME/events.failing-since` | `markFailing` on every write-path failure, `clearFailing` on success | ✓ WIRED, TOOL 0/2 (format) | `verify.key-links` fails 01-04's two links with `from: must be a relative file path` — the plan spells `from:` as `internal/events/events.go (Emit)`. A plan-authoring format issue, **pre-existing**: `git log` shows 01-04-PLAN.md untouched since `c9f5a27`. Links verified by reading and by `TestAFailedWriteMarksTheGardenAsFailing` / `TestASuccessfulWriteClearsTheFailureMark` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `internal/cli/yield.go` (`showHealth`) | `h` | `events.HealthOf()` — live stats of `$HUGEL_HOME`, the log and the marker | Yes — output changed with every one of 13 states built | ✓ FLOWING |
| `internal/events/events.go` (`HealthOf`) | `h.LastWrite` | `os.Stat(events.jsonl).ModTime()`, gated on `IsRegular()` | Yes | ✓ FLOWING (not always rendered — Advisory 1) |
| `internal/events/events.go` (`HealthOf`) | `h.FailingSince` | `os.Stat(events.failing-since).ModTime()`, gated on `IsRegular()` | Yes, and now reached before the probe can pre-empt it | ✓ FLOWING |
| `internal/events/events.go` (`HealthOf`) | `h.Healthy` | `h.Reachable && h.FailingSince == nil` (events.go:433) | Yes | ✓ FLOWING |
| `internal/events/writable_unix.go` | probe result | `unix.Access(path, W_OK)` then `unix.Statfs(path).Bavail > 0` | Yes — measured `false` on the full filesystem where the old probe returned `true` | ✓ FLOWING |
| `internal/events/events.go` | `writeTarget` | `os.Stat` walk to the nearest existing ancestor | Yes — an uncreated garden resolves to its writable parent | ✓ FLOWING |

### Behavioral Spot-Checks

Rows 1–5 are toolchain. Rows A–O were run against a binary built from this tree
(`go build -o <scratch>/hugel ./cmd/hugel`). Rows M1–M4 are mutations, each reverted and re-hashed.

| # | Behavior | Command | Result | Status |
|---|----------|---------|--------|--------|
| 1 | Build | `go build ./...` | exit 0 | ✓ PASS |
| 2 | Static check | `go vet ./...` | exit 0 | ✓ PASS |
| 3 | Cross-build | `GOOS=linux/windows go build`, `go vet`, `go list -f '{{.GoFiles}}'` | all exit 0; `writable_unix.go` on linux, `writable_other.go` on windows | ✓ PASS |
| 4 | Full suite | `go test -count=1 ./...` | 19 packages `ok`, run once | ✓ PASS |
| 5 | Named tests | gate ×1, tender ×1, events ×4 | all PASS | ✓ PASS |
| A | Garden dir **does not exist** (fresh install) | `HUGEL_HOME=<nonexistent> hugel yield --health` | `healthy` / `never -- nothing has been recorded yet` | ✓ PASS — pass-3 regression closed |
| B | Garden dir exists, empty | same | `healthy` / `never` | ✓ PASS |
| C | Garden with a written log | same | `healthy` / `last write: 2026-08-20 10:00 (20d ago)` | ✓ PASS |
| D | Dir `0500`, no log | same | `unknown -- cannot confirm a write to <path> would land` | ✓ PASS — probe still catches it |
| E | Dir `0500`, log present and writable | same | `healthy` / `last write: ...` | ✓ PASS — over-correction guard held |
| F | Dir writable, log `0400`, **marker present and dated** | same | `FAILING since 2026-09-01 10:00 (8d)` / `last write: 2026-08-20 10:00 (20d ago)` | ✓ PASS — pass-3 regression closed |
| G | Dir writable, log `0400`, no marker | same | `unknown` | ✓ PASS |
| H | Log is a directory | same | `unknown` | ✓ PASS |
| I | Marker is a directory, garden fully writable | same, then a shell append | `unknown`; the append **succeeds** | ⚠️ Advisory 2 — safe direction, sentence inaccurate |
| J | Uncreated garden under an **unwritable** existing ancestor | same | `unknown` | ✓ PASS — the ancestor walk did not over-relax |
| K | Uncreated deep garden under a writable ancestor | same | `healthy` / `never` | ✓ PASS |
| L | `--health --json` on the unknown path | same | `{"healthy":false,"reachable":false,"home":...}`, exit 0 | ✓ PASS |
| M | Dated marker present **and** log is a directory | same | `unknown` | ⚠️ Advisory 3 — README's "a dated failure always wins" counter-example |
| N | Garden dir `0600` (no traverse), marker present | same | `unknown` | ✓ PASS — write genuinely cannot land |
| O | Does `--health` write anything? | full `ls -laR` + mtime + size before/after, healthy path and unknown path | byte-identical both times | ✓ PASS — README's "writes nothing" claim holds |
| P | **Full disk, `Bavail=0`, last block consumed** | in-process `Emit`/`HealthOf` + CLI on a 4 MB HFS+ ram disk at 100% | `Access(home)=nil Access(log)=nil` (permission says yes), `Statfs Bavail=0`, `Emit` = `no space left on device`, marker **absent**, `Health={Healthy:false Reachable:false}`, CLI `unknown` | ✓ PASS — **the pass-3 headline gap is closed** |
| Q | **Full disk, `Bavail=0`, last block has slack** | same volume, log truncated first | CLI `unknown`; a real `Emit` immediately after returns **nil** and appends | ⚠️ The documented early-warning window, reproduced. Judged acceptable above |
| M1 | Mutation: drop the `FailingSince == nil` guard (probe runs over the marker) | `go test -run 'TestHealthOfKeeps...'` | FAIL: `Reachable = false, want true` | ✓ Marker precedence is load-bearing |
| M2 | Mutation: `writeTarget` always returns `home` | `go test -run TestHealthOf` | FAIL ×2: `TestHealthOfStillTrustsAWritableLogInAReadOnlyGarden`, `TestHealthOfCallsAnUncreatedGardenFreshRatherThanUnknown` | ✓ Both fixes load-bearing, and the guard against over-correcting still bites |
| M3 | Mutation: delete the whole `unix.Statfs` block | `go test ./internal/events` | **ok — suite stays green** | ✗ The ENOSPC fix is held by no test (Advisory 4) |
| M4 | Mutation: delete `Emit`'s checked flush | `go test -run 'TestEmitFlushes\|TestAFailedFlush'` | FAIL ×2 | ✓ Seam still load-bearing |

### Probe Execution

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| — | `find scripts -path '*/tests/probe-*.sh'` | no matches; no probe paths declared in any PLAN or SUMMARY | SKIPPED (no probes in this project) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| SUB-01 | 01-01, 01-02, 01-03 | `events.Record` (`Emit`) returns an error instead of swallowing it | ✓ SATISFIED | 10/10 sites check and report; error returned on permission-denied, is-a-directory, ENOSPC and a forced sync failure — all four observed this pass |
| SUB-02 | 01-03 | Event writes are flushed durably | ✓ SATISFIED | Checked, non-deferred flush before the success return, held by two mutation-proven tests (M4); claim scoped to `fsync(2)` in the doc comment |
| SUB-03 | 01-04, 01-05 | A gardener can distinguish "nothing has run since \<date\>" from "writes have been failing since \<date\>" without reading the code | ✓ SATISFIED | Both sentences produced verbatim by `hugel yield --health` (rows A/K and F). No state built on unix — including a real full filesystem — reports `healthy` while writes fail. Renderer precision issues recorded as Advisory, not as coverage gaps |

No orphaned requirements. REQUIREMENTS.md maps exactly SUB-01/02/03 to Phase 1, and all three appear
across the plans' `requirements:` frontmatter (01-01 `[SUB-01]`, 01-02 `[SUB-01]`, 01-03
`[SUB-01, SUB-02]`, 01-04 `[SUB-03]`, 01-05 `[SUB-03]`). No plan claims a requirement outside that
set. **REQUIREMENTS.md still records all three as `Gaps Found` in its traceability table** — that
line is now stale and should be advanced to Complete by whoever lands this verification.

### Prohibitions

| Plan | Statement | Tier | Disposition |
|------|-----------|------|-------------|
| 01-01, 01-02 | A failed event write must never change the outcome of the work it instruments | test | ✓ HELD — enforcement wired and passing (`TestAGateStillReachesItsVerdictWhenTheLogCannotBeWritten`, `TestATenderStopsEvenWhenTheLogCannotBeWritten`), all 10 sites re-read |
| 01-03 | Durability must never be claimed beyond what the running platform provides | judgment | ⚠️ FLAGGED — `unverified-prohibition, human review recommended`. Non-authoritative LLM verdict: **held**. The doc comment (events.go:208-223) claims only that the data left the process and the OS accepted it, names fsync(2) rather than F_FULLFSYNC, and cites golang/go#26650. Routed to human verification |
| 01-04, 01-05 | The health surface must never report a state it cannot establish — when the garden cannot be read, it must say so rather than defaulting to healthy | test | ✓ HELD — enforcement wired and mutation-proven (M1, M2, plus `TestHealthOfWillNotCallAnUnwritableGardenQuiet`, `TestHealthOfWillNotCallAnUnwritableLogHealthy`, `TestHealthOfRefusesToGuessWhenTheGardenCannotBeRead`). Caveat: the free-space half of the enforcement has no test (M3, Advisory 4) |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/cli/yield.go` | 364-365 | Prints `last write: unknown` while `Health.LastWrite` is populated and `--json` prints it | ⚠️ Warning (Advisory 1) | Measured on the full disk. Withholds a real date in exactly the state the round closed; the gardener must reach for `--json` to get SC-3's second fact |
| `internal/cli/yield.go` | 364-365 | `cannot confirm a write ... would land` is false when the *marker* is non-regular and the garden is writable | ⚠️ Warning (Advisory 2) | Measured (row I). Contrived state; never a false healthy. The pre-rewording sentence was accurate here |
| `README.md` | ~52-53 | "A dated failure always wins ... it never answers `unknown` over an answer it already has" — absolute claim with a counter-example | ⚠️ Warning (Advisory 3) | Measured (row M). Overstates informativeness, not safety |
| `internal/events/writable_unix.go` | 25-37 | The Statfs free-space check has no test | ⚠️ Warning (Advisory 4) | M3 deletes it and the suite stays green. This is the code that closes the phase's headline gap; the syncFile-seam argument from 01-03 applies to it verbatim |
| `internal/events/writable_other.go` | 18 | `!unix` fallback returns `true` unconditionally | ℹ️ Info | **Judged not to block — see below.** Now documented in the file *and* in README.md where a gardener reads. Both `GOOS=windows` build and vet clean |
| `.planning/.../01-04-PLAN.md` | `must_haves.key_links.from` | `internal/events/events.go (Emit)` — parenthetical suffix breaks `verify.key-links` (0/2) | ℹ️ Info — pre-existing | `git log` confirms 01-04-PLAN.md untouched since the planning commit `c9f5a27`. Format issue, not a wiring issue; both links verified by reading and by test |
| `.planning/.../01-03-PLAN.md` | acceptance criteria | `test "$(grep -c 'defer f.Close()' internal/events/events.go)" = "1"` — actual count is **2** | ℹ️ Info — pre-existing, NOT this session's doing | Independently re-confirmed: `git show c9f5a27:internal/events/events.go \| grep -c 'defer f.Close()'` also returns 2, so the assertion was wrong the day it was written. The second is `Load`'s own close (events.go:466) |
| `internal/gate/run.go` 109, 197; `internal/tender/start.go` 117 | — | `outcomeOf(err == nil)` / `err.Error()` read the outer `err` from inside a shadowing `if err := events.Emit(...)` (WR-01) | ⚠️ Warning — **still acceptable to leave open** | Re-traced today against Go's ShortVarDecl scoping rule and re-confirmed correct; two passing tests agree. Tracked as `hugel4-k65` (OPEN, P3) with the complete site list and a one-line-per-site fix. Judged acceptable for this phase: no present misbehaviour, no user-visible effect, risk written down rather than forgotten. **The judgement should not extend past Phase 2**, which widens exactly these payloads and is where a refactor most plausibly inverts the field |
| beads | — | No bead tracks the ENOSPC early-warning window or the non-unix gap; both live only in code comments and the README | ℹ️ Info | Recommend filing one, so the two accepted limitations are auditable rather than only narrated |

No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` markers in any file this phase touched.

**On `writable_other.go` — judged sufficient, does not block.** Documenting is the right call here,
for reasons that are about this project rather than about non-Unix in general: hugel is a
single-gardener tool that targets macOS and Linux, there is no non-Unix CI, nothing on that platform
is exercised by a test, and the two alternatives are both worse — demoting every non-Unix garden to
`unknown` reintroduces the false-alarm failure this session just rejected twice, and a real Windows
implementation (`GetDiskFreeSpaceEx` plus an access check) is speculative work for no user. What
changed since pass 3 is the part that mattered: the limitation is now stated in README.md, where a
gardener reads, rather than only in a source comment. The residual is that it is documented but
untracked — hence the bead recommendation above.

**On the 01-03-PLAN edits — re-judged, and legitimate.** The set is now complete and internally
consistent: `artifacts.contains`, `key_links.pattern`, the interface-table row and all four source
acceptance criteria have moved to the `syncFile` spelling, and `verify.key-links` reports 1/1 where
it reported 0/1. Three things make this upkeep rather than laundering, and I checked each rather
than taking the previous pass's word: the plan body carries a dated, in-place amendment
(`NOTE (amended after phase 01 UAT): ... only its spelling moved. See bead hugel4-0kf`); the bead
exists and describes the change accurately; and `git diff c9f5a27 HEAD -- 01-03-PLAN.md` shows
*only* the spelling change plus that NOTE — no criterion was weakened, deleted, or made vacuous. It
is a strict strengthening: a grep over a spelling became two tests I re-proved go red when the flush
is deleted (M4). The one criterion left untouched is the `defer f.Close() = 1` count, which is
wrong and was wrong before this branch existed — leaving a failing assertion visible is the honest
outcome there.

### Human Verification Required

1. **Dogfood `--health` against the real garden.**
   **Test:** Run `hugel yield --health` with no `HUGEL_HOME` override. Run a real gate or
   `hugel tender`. Run it again.
   **Expected:** The last-write time moves forward.
   **Why human:** `config.Sandbox` panics if any test resolves the garden to a non-temporary
   directory, so no test may ever go near the gardener's own garden. Every measurement in this
   report was made on synthetic gardens and a purpose-built filesystem.

2. **Judgment-tier prohibition (01-03): the durability claim.**
   **Test:** Read the `Emit` doc comment at `internal/events/events.go:208-223`.
   **Expected:** It claims only that the data left the process and the OS accepted it, names
   `fsync(2)` rather than `F_FULLFSYNC`, and cites golang/go#26650.
   **Why human:** judgment-tier prohibition. The verdict recorded above is a non-authoritative LLM
   judgement — `unverified-prohibition, human review recommended`.

### Gaps Summary

**No gaps.** The phase goal is achieved.

The state that defeated three verification passes — `event log: healthy` printed over a filesystem
refusing every write — could not be reproduced on this tree. I rebuilt it from scratch (a 4 MB HFS+
volume filled to 100%, appends confirmed failing, marker confirmed absent) and the binary printed
`unknown`. `unix.Access` still says yes in that state, exactly as before; `unix.Statfs` is what
now says no. That is the class being answered rather than an instance being patched, and it is the
right shape of fix: the read path stopped trying to infer writability from permission bits alone,
without ever writing to the garden it reports on.

The two regressions the previous pass caught are both closed, and closing them did not re-open
anything. I checked that specifically rather than assuming it: an unwritable directory with no log
still says `unknown` (row D), an unwritable log with no marker still says `unknown` (row G), and a
read-only directory holding a writable log still — correctly — says `healthy` (row E). The two
fixes are structural rather than special-cased. `writeTarget()` asks about the path `Emit` would
actually need permission on, which is the log once it exists and the nearest existing ancestor
otherwise, because that is what `MkdirAll` needs; and the probe now runs last, gated on
`FailingSince == nil`, so a dated marker is never overwritten by a shrug. Both are mutation-proven
load-bearing, and both have a named regression test that goes red when the fix is removed.

**What I would still change, none of it blocking.** Four things, all recorded as Advisory:
the free-space check that closes this phase's headline gap is held by no test and can be deleted
silently (M3) — the same argument that produced the `syncFile` seam in 01-03 applies to it, and a
statfs seam would close it; the plain renderer prints `last write: unknown` about a date it holds
and `--json` prints; the reworded unreachable sentence is inaccurate in the one state where a
non-regular marker demotes a writable garden; and the README's "a dated failure always wins" has a
counter-example. None produces a false `healthy`, none prevents either of SC-3's two readings, and
none is worth another gap-closure round on its own — but the README one in particular is the defect
class this phase exists to remove, and should be tightened the next time that file is touched.

**On the early-warning window, once more, plainly.** It is acceptable and it is not inconsistent
with rejecting the read-only-directory over-demotion. The rejected demotion was a permanent false
sentence about a garden that writes successfully forever, including every fresh install; this is a
transient true sentence — hugel genuinely cannot confirm a write will land when `Bavail` is zero,
as my own measurement of a 72-byte append succeeding and a 64 KB append failing on the same volume
shows — about a garden that is a couple of events from permanent, unmarkable failure. Different
call, same principle.

---

_Verified: 2026-09-09T22:55:00Z_
_Verifier: Claude (gsd-verifier)_
