---
phase: 01-trustworthy-event-writes
reviewed: 2026-09-08T21:30:00Z
depth: standard
files_reviewed: 13
files_reviewed_list:
  - README.md
  - internal/cli/dispatch.go
  - internal/cli/main_test.go
  - internal/cli/yield.go
  - internal/cli/yield_test.go
  - internal/complete/spec.go
  - internal/config/sandbox.go
  - internal/events/events.go
  - internal/events/events_test.go
  - internal/gate/gate_test.go
  - internal/gate/run.go
  - internal/tender/start.go
  - internal/tender/tender_test.go
findings:
  critical: 1
  warning: 1
  info: 1
  total: 3
status: issues_found
---

# Phase 01: Code Review Report

**Reviewed:** 2026-09-08T21:30:00Z
**Depth:** standard
**Files Reviewed:** 13
**Status:** issues_found

## Summary

This phase changed `events.Emit` from a silent-failure function into one that
returns an error, swept all ten production call sites to check and report that
error on stderr, added a checked `f.Sync()`, added a sticky first-failure
marker (`events.failing-since`) plus `events.HealthOf`, and surfaced it all as
`hugel yield --health`.

The central invariant — a failed event write must never change the outcome of
the work it instruments — holds. I traced every one of the ten production
`events.Emit` call sites (`internal/gate/run.go` ×6, `internal/tender/start.go`
×3, `internal/cli/dispatch.go` ×1) and confirmed none of them let the event
write's error leak into the caller's own control flow; the `if err := ...;
err != nil` pattern is scoped correctly at each site, including two places
that rely on Go's short-variable-declaration scoping rules to read the
*pre-shadow* outer `err` inside the event literal itself before the inner
`err` comes into scope (`gate/run.go:110`, `gate/run.go:198`, `tender/start.go:119`).
Those all evaluate correctly today, verified by `go build`, `go vet`, and
`go test -race` across the affected packages, all of which pass.

However, the new `events.HealthOf` — the trust-surfacing mechanism this phase
exists to deliver — has a real, demonstrable correctness bug: it can report a
garden as `Healthy: true` with a fabricated, recent `LastWrite` timestamp even
though not a single event has ever been successfully recorded and the log is
currently unwritable. This directly contradicts the feature's stated purpose
(let a gardener trust what `hugel yield --health` says about the log) and is
detailed below as a Critical finding, with a reproduction.

## Critical Issues

### CR-01: `HealthOf` can report a broken, never-written garden as healthy

**File:** `internal/events/events.go:363-372`

**Issue:** `HealthOf` derives `LastWrite` from `os.Stat(p).ModTime()` without
checking that `p` (the event log path) is a regular file. If something other
than `Emit` occupies that path — most plausibly a directory, e.g. from a typo
in a setup script (`mkdir -p $HUGEL_HOME/events.jsonl`) or a botched
migration — `os.Stat` still succeeds, and its `ModTime()` (the directory's own
creation/modification time) is reported as `LastWrite`, even though no event
was ever appended there. Worse: until the *first* `Emit` call is actually
attempted against that broken path, `events.failing-since` has never been
created, so `FailingSince` is `nil` and `Healthy` computes to `true`.

The result: `hugel yield --health` on a garden that has never recorded a
single event and cannot currently record one prints `event log: healthy` and
`last write: <a few seconds ago>` — the exact scenario this feature exists to
catch, reported as the opposite of what is true. This also violates the type's
own documented contract: `LastWrite` is documented as "when the log was last
successfully appended to, nil when it has never been written" (events.go:322-324),
and `Health.Reachable`'s doc says "a log that cannot be written cannot record
its own failure, so health has to be able to say that it does not know"
(events.go:313-318) — but here it confidently says the opposite of "I don't know."

Reproduction (verified against this tree):
```go
home := t.TempDir()
t.Setenv("HUGEL_HOME", home)
p, _ := Path()
os.MkdirAll(p, 0o755) // the log path is a directory, nothing has ever written to it

h, _ := HealthOf()
// h == {Healthy:true Reachable:true LastWrite:<just now> FailingSince:<nil>}
```
No test in `events_test.go` or `yield_test.go` exercises `HealthOf` against a
directory-shaped (or otherwise non-regular) log path — the existing tests that
construct this exact filesystem state (`TestAFailedWriteMarksTheGardenAsFailing`,
`TestTheFailureMarkKeepsTheFirstFailuresTime`, and the gate/tender tests that
simulate an unwritable log) all drive the scenario through `Emit`, which
*does* correctly fail and mark it — but never call `HealthOf` before that
first `Emit` happens, which is exactly the window where the bug is invisible.

**Fix:** Only trust the stat as evidence of a real write when it names a
regular file; otherwise report unreachable/unknown rather than fabricating a
timestamp, mirroring the treatment already given to the permission-denied
case immediately below it:

```go
if st, err := os.Stat(p); err == nil {
    if !st.Mode().IsRegular() {
        // Something other than Emit occupies the log path -- there is no
        // write to report, and trusting its mtime would fabricate one.
        h.Reachable = false
        return h, nil
    }
    t := st.ModTime()
    h.LastWrite = &t
} else if !os.IsNotExist(err) {
    h.Reachable = false
    return h, nil
}
```
Add a regression test alongside the existing `TestHealthOf*` tests that
constructs the log path as a directory (as `TestAFailedWriteMarksTheGardenAsFailing`
already does) and asserts `HealthOf()` before any `Emit` call reports
`Healthy: false` / `LastWrite: nil`, not a fabricated recent write.

## Warnings

### WR-01: Event-outcome fields depend on fragile short-variable-declaration shadowing

**File:** `internal/gate/run.go:109-114`, `internal/gate/run.go:197-202`, `internal/tender/start.go:117-123`

**Issue:** Several sites compute a value that must reflect the *outer*
operation's error (not the event write's error) from inside the initializer of
the very `if err := events.Emit(...); err != nil` statement that shadows that
name, e.g.:

```go
out, err := runTests(t.Worktree, test)
if err := events.Emit(events.Event{
    Name: "gate.test", Bead: t.Bead, Bed: t.Bed, Outcome: outcomeOf(err == nil),
    ...
}); err != nil {
```

This is correct today only because Go's short-variable-declaration rule
evaluates the right-hand side (`events.Emit(...)`, including the
`outcomeOf(err == nil)` expression nested inside its `Event` literal) using
the *pre-existing* `err` — the new `err` introduced by `:=` is not in scope
until after the declaration completes. This is easy for a future editor to
break silently: splitting the `Event{}` construction out of the `if`
initializer, reordering the two statements, or renaming a variable during a
refactor would compile cleanly either way and could quietly start reporting
`outcomeOf` (or, in `tender/start.go:119`, the recorded `"error"` field) based
on the *event write's* failure instead of the operation actually being
measured — silently corrupting the exact event data this phase was written to
make trustworthy, with no compiler diagnostic to catch it.

**Fix:** Capture the value before it can be shadowed, so correctness does not
depend on evaluation-order trivia:

```go
out, err := runTests(t.Worktree, test)
testOK := err == nil
if err := events.Emit(events.Event{
    Name: "gate.test", Bead: t.Bead, Bed: t.Bed, Outcome: outcomeOf(testOK),
    ...
}); err != nil {
    fmt.Fprintf(os.Stderr, "hugel: event %q not recorded: %v\n", "gate.test", err)
}
```
and similarly bind `errMsg := err.Error()` before the nested `if err :=
events.Emit(...)` in `tender/start.go:117-123`.

## Info

### IN-01: `HealthOf`'s failure-marker stat has the same non-regular-file assumption

**File:** `internal/events/events.go:374-379`

**Issue:** The same pattern flagged in CR-01 — trusting `os.Stat(...).ModTime()`
without checking the file is regular — is repeated for the `events.failing-since`
marker. In normal operation this marker is only ever created by `markFailing`
via `O_CREATE|O_EXCL|O_WRONLY`, so it can only be a regular file unless
something external interferes with that exact path, making this a lower-risk
variant of CR-01 (there is no realistic code path in this package that would
create it as anything else). Worth fixing alongside CR-01 for consistency
since the fix is identical in shape, but not independently a blocker.

**Fix:** Apply the same `st.Mode().IsRegular()` guard used in the CR-01 fix
when stat'ing `mark`, or extract a small helper shared by both stats.

---

_Reviewed: 2026-09-08T21:30:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
