---
status: complete
phase: 01-trustworthy-event-writes
source: 01-01-SUMMARY.md, 01-02-SUMMARY.md, 01-03-SUMMARY.md, 01-04-SUMMARY.md, 01-05-SUMMARY.md
started: 2026-09-09T20:16:49Z
updated: 2026-09-09T20:32:39Z
---

## Current Test

[testing complete]

## Tests

### 1. handback event site held without a dedicated test
expected: internal/cli/dispatch.go's handBack event site is covered only by a source assertion plus the phase-wide grep, not a dedicated test — a documented plan decision, not an oversight. Confirm the gap is acceptable and that plan 05's harness closes it.
result: pass
coverage_id: 01-02/D4

### 2. Durability claim stated at, not above, the fsync ceiling
expected: The Emit doc comment (internal/events/events.go:209-214) claims only what fsync(2) actually provides and names the macOS F_FULLFSYNC gap (golang/go#26650). TestEmitIsDurableBeforeItReturns proves the narrower testable claim — nothing of ours buffers the write. Read the caveat and confirm the stated guarantee is no stronger than that ceiling.
result: pass
note: doc comment confirmed accurate. Test gap found and fixed during UAT — syncFile seam + TestEmitFlushesBeforeItReturns + TestAFailedFlushIsReportedAndMarksTheGarden (bead hugel4-0kf). F_FULLFSYNC explored and declined: SUB-02 promises crash-durability, which write(2) already survives.
coverage_id: 01-03/D2

### 3. hugel yield --health reports log state and last write in one command
expected: `hugel yield --health` (and `--health --json`) reports whether the event log is healthy and when it was last written, in one command, without a transcript directory. KNOWN GAP — 01-VERIFICATION.md marks this FAILED: when $HUGEL_HOME/events.jsonl exists as a directory (never written, unwritable), events.HealthOf trusts the bare os.Stat at internal/events/events.go:363 and reports healthy with a fabricated last-write time. Still unfixed on this tree.
result: pass
note: CR-01/IN-01 fixed during UAT (bead hugel4-q21) — IsRegular guards on both stats in events.HealthOf, plus TestHealthOfWillNotCallADirectoryAWrittenLog and TestHealthOfWillNotTakeAFailureDateFromSomethingItDidNotWrite. Both verified load-bearing: stripping the guards fails them with the verifier\'s exact symptom. End-to-end repro now prints "unknown -- garden could not be read" instead of "healthy". 01-VERIFICATION.md still records the original FAILED verdict — re-verification is a separate pass, not self-certified here.
coverage_id: 01-05/D1

### 4. --health is discoverable from a real shell session
expected: A gardener finds the flag without reading the source — shell completion offers --health (internal/complete/spec.go:85) and the README documents it (README.md:34). Both are asserted by grep/test; confirm it is actually discoverable in practice.
result: pass
coverage_id: 01-05/D4

### 5. events.Emit returns a non-nil, descriptive error on every failure path instead of swallowing it
expected: events.Emit returns a non-nil, descriptive error on every failure path instead of swallowing it
result: pass
source: automated
coverage_id: 01-01/D1

### 6. A gate whose event writes all fail still reaches its verdict and returns it unchanged
expected: A gate whose event writes all fail still reaches its verdict and returns it to its caller unchanged (nil error, correct Report)
result: pass
source: automated
coverage_id: 01-01/D2

### 7. Gardener sees a named not-recorded line on stderr, without leaking Event.Fields
expected: The gardener sees a `hugel: event %q not recorded: %v` line on stderr for each event the gate could not write, without leaking Event.Fields
result: pass
source: automated
coverage_id: 01-01/D3

### 8. No events-package test still asserts a dropped event is silent
expected: No test in the events package still asserts that a dropped event is silent
result: pass
source: automated
coverage_id: 01-01/D4

### 9. Four remaining gate/run.go Emit sites check and report without altering control flow
expected: The four remaining direct events.Emit call sites in internal/gate/run.go (gate.test on branch, gate.review, gate.test on merged, gate.land) check the error and print a named not-recorded notice without altering gate control flow
result: pass
source: automated
coverage_id: 01-02/D1

### 10. Tender and handback Emit sites check and report without changing return values
expected: All three tender/start.go Emit sites and dispatch.go's handBack site check and report without changing return values; a tender still stops and a handback still reaches bd
result: pass
source: automated
coverage_id: 01-02/D2

### 11. Ten of ten production emitters check the error phase-wide
expected: Ten of ten production emitters check the error phase-wide (zero unchecked events.Emit call sites left in internal/, excluding tests)
result: pass
source: automated
coverage_id: 01-02/D3

### 12. Emit calls a checked f.Sync() before returning nil
expected: Emit calls a checked f.Sync() before it returns nil; a sync failure returns a wrapped "sync event log" error instead of a silent success
result: pass
source: automated
coverage_id: 01-03/D1

### 13. Timer.Done returns nil on a nil timer, otherwise whatever Emit returned
expected: Timer.Done returns error: nil on a nil timer, otherwise whatever Emit returned
result: pass
source: automated
coverage_id: 01-03/D3

### 14. No comment or doc still describes event emission as discarding its errors
expected: No comment in the events package, the config package, or the README still describes hugel's event emission as something that discards its errors
result: pass
source: automated
coverage_id: 01-03/D4

### 15. A failed write leaves a first-failure marker; a success clears it
expected: A failed event write leaves a marker (events.failing-since) dated to the first failure of the streak; later failures in the same streak do not move that date; the next successful write clears it
result: pass
source: automated
coverage_id: 01-04/D1

### 16. HealthOf reports last-write and failure-streak start as two independent nilable facts
expected: events.HealthOf reports the log's last-write time and the open failure streak's start as two independent, nilable facts
result: pass
source: automated
coverage_id: 01-04/D2

### 17. An unreadable garden is reported as unreachable, not healthy
expected: When the garden itself cannot be read, HealthOf says so (Reachable/Healthy both false, Home named, nil error) rather than reporting healthy from two absences
result: pass
source: automated
coverage_id: 01-04/D3

### 18. The report distinguishes "nothing has run" from "writes have been failing"
expected: The report distinguishes a garden where nothing has run from one where writes have been failing, and names the date of each
result: pass
source: automated
coverage_id: 01-05/D2

### 19. When the garden cannot be read, the report says so instead of reporting healthy
expected: When the garden cannot be read, the report says so instead of reporting healthy
result: pass
source: automated
coverage_id: 01-05/D3

## Summary

total: 19
passed: 19
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none yet]
