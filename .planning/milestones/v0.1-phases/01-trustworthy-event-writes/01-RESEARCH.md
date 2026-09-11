# Phase 1: Trustworthy Event Writes - Research

**Researched:** 2026-09-08
**Domain:** Go stdlib file I/O durability + error-propagation refactor of an existing internal package (`internal/events`)
**Confidence:** HIGH

## Summary

This phase touches one existing package (`internal/events`, 243 lines) and three call-site
files (`internal/tender/start.go`, `internal/gate/run.go`, `internal/cli/dispatch.go`).
There is no new external dependency, no new framework, and no library to select — this is a
targeted refactor of a function whose current contract (`Emit` returns nothing and swallows
every error) is exactly the defect SUB-01 through SUB-03 name. The requirement text says
`events.Record`; the actual function is `events.Emit` (`internal/events/events.go:148`) plus
a `Timer`/`Start`/`Done` pair that wraps it. `Start`/`Timer` are declared and tested but **never
called from production code** — grep confirms zero non-test call sites — so the real blast
radius is the 10 `Emit` calls across 3 files, not the wrapper type.

The package's own doc comment (lines 141-147) argues at length for the current swallow-errors
design ("an instrument that can break the thing it measures is worse than no instrument") and
one test (`TestEmitCannotFailTheCaller`) asserts exactly that contract. SUB-01 supersedes that
design decision; it does not invalidate the rationale behind it — criterion 4 restates the same
worry in the new contract's terms ("never destroys the work it was instrumenting"). The fix is
therefore not "let the error propagate" but "let the error be *seen* without letting it
*propagate into a failure of the surrounding work*." Every call site must change from
`events.Emit(...)` (return value discarded) to checking the returned error and reporting it
without failing the tender/gate/handback it instruments.

`internal/draws` (`internal/draws/draws.go`) already returns `error` from `Append` and every
caller already checks it (confirmed by reading `draws.go` and `draws_test.go`) — but `draws.Append`
does **not** call `f.Sync()` either. It is not a template for durability, only for the
error-return shape. SUB-02's fsync requirement has no existing sibling in this codebase to copy;
it must be added fresh, guided by Go stdlib semantics researched below.

**Primary recommendation:** Change `Emit`'s signature to `func Emit(e Event) error`, add
`f.Sync()` before `f.Close()` inside it, update all 10 call sites to check-and-report (stderr
note, never a failed return), and add a small first-failure marker file
(`$HUGEL_HOME/events.failing-since`, following the exact idiom already used by
`internal/cli/compost.go:compostMark`) that a new `hugel yield --health` view reads alongside
`events.jsonl`'s own mtime to answer SUB-03.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SUB-01 | `events.Record` returns an error instead of swallowing it, so a caller can tell a written event from a dropped one | "Codebase Orientation" enumerates all 10 real call sites (the function is actually named `Emit`) with exact file:line and the disposition each needs; "Research: SUB-01 blast radius" and "Disposition at each call site" sections give the concrete signature change and the report-and-continue mechanism |
| SUB-02 | Event writes are flushed durably, so a crash does not silently lose the tail of the log | "Research: SUB-02 durability in Go" gives the exact `f.Sync()` pattern, platform nuances (Darwin F_FULLFSYNC gap, directory-entry sync edge case), and a cost argument for why no perf gate is needed at this project's write volume |
| SUB-03 | A gardener can distinguish "nothing has run since \<date\>" from "writes have been failing since \<date\>" without reading the code | "Research: SUB-03 health surface" designs the two-fact model (last-write mtime + sticky first-failure marker file, following the existing `compostMark` idiom), names the CLI surface (`hugel yield --health`), and explicitly addresses the "log that can't write can't record its own failure" edge case |
</phase_requirements>

## Architectural Responsibility Map

This is a single-binary Go CLI, not a multi-tier web app — the standard Browser/SSR/API/CDN/DB
tiers do not apply. Adapted to this project's actual layers (per
`.planning/codebase/ARCHITECTURE.md`): **CLI** (`internal/cli/*`, thin handlers), **Domain
package** (`internal/events`, owns the write mechanism), **Filesystem** (`$HUGEL_HOME/*.jsonl`,
no database, no server).

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Durable append + fsync | Domain package (`internal/events`) | Filesystem | The write mechanism belongs beside the file convention it protects — the CLI and callers should not know about O_APPEND/Sync at all |
| Error surfaced without failing instrumented work | Call sites (`internal/tender`, `internal/gate`, `internal/cli/dispatch.go`) | — | Only the caller knows whether its own work can tolerate a warning-and-continue; `events` package cannot decide this for tender/gate |
| Health question ("healthy? last written when?") | CLI (`internal/cli/yield.go`, new `--health` view) | Domain package (`internal/events`, new `Health()` function) | Follows the existing `yield` reporting family pattern exactly (`--soil`, `--changes`, `--spikes` are all boolean-flag views in the same file) |
| First-failure marker persistence | Domain package (`internal/events`) | — | Co-located with `Emit` since it is `Emit`'s own failure that creates the marker; a caller-level marker would need duplicating at every call site |

## Package Legitimacy Audit

**Not applicable.** This phase introduces no new external dependency. `os.File.Sync`,
`os.OpenFile`, `os.Chtimes`, `os.Remove` are all Go standard library (`os` package), already
imported by `internal/events/events.go` and `internal/cli/compost.go`. No `go.mod` change is
required.

## Codebase Orientation (required reading before planning)

### `events.Record` / `events.Emit` — current signature and behavior

`internal/events/events.go:148-174`:

```go
// Emit records an event, and cannot fail the caller.
//
// It returns nothing on purpose. Every emitter is inside work that matters more
// than its own instrumentation — a tender mid-run, a gate about to merge — and
// an error return is an invitation to propagate it. An instrument that can
// break the thing it measures is worse than no instrument, so a log that cannot
// be written loses the event and nothing else.
func Emit(e Event) {
	if e.Time.IsZero() {
		e.Time = now()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	p, err := Path()
	if err != nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	// One write per event: an append of a single line is what keeps concurrent
	// emitters from interleaving halves of each other's events.
	_, _ = f.Write(append(b, '\n'))
}
```
`[VERIFIED: internal/events/events.go:141-174]` — read this session, quoted verbatim above.

Four bare `return`s on error (marshal failure, path-resolution failure, mkdir failure, open
failure) and one discarded write result (`_, _ = f.Write(...)`) — five distinct silent-failure
points, matching `.planning/codebase/CONCERNS.md`'s "Instrumentation error swallowing" entry
(`internal/events/events.go:173` cited there for the discarded `Write`).
`[VERIFIED: .planning/codebase/CONCERNS.md:7-11]`

The file is opened and closed **per event**, not held open across the process — every `Emit`
call does its own `OpenFile`→`Write`→`Close` inside the `mu.Lock()`/`defer mu.Unlock()` block.
`[VERIFIED: internal/events/events.go:161-174]`. The mutex serializes concurrent *goroutines
within one process* (relevant because `hugel dispatch` runs tenders concurrently — see
`internal/cli/dispatch.go`); it does **not** serialize concurrent *processes* (e.g. a `hugel
gate` process and a `hugel tender` process racing). `O_APPEND` gives per-write atomicity at the
OS level for POSIX-compliant filesystems as long as a single `Write` call's byte count does not
exceed the platform's atomic-write limit — existing behavior, not something SUB-02 needs to
change, but worth flagging as a pre-existing limitation this phase does not fix.

There is no `Sync()` call anywhere in the file — confirmed by reading the full 243-line file;
`grep -n "Sync" internal/events/events.go` returns nothing.

### `Timer` / `Start` / `Done` — declared, tested, never called in production

`internal/events/events.go:177-209` defines `Start(name string, e Event) *Timer` and
`(*Timer).Done(outcome string, fields F)`, both of which route through `Emit`. `Done` currently
returns nothing.

```bash
$ grep -rn "events\.Start(" --include="*.go" . | grep -v _test.go
# (no output)
```
`[VERIFIED: grep run this session against the full repo]` — zero production call sites. Only
`internal/events/events_test.go` (`TestTimerMeasuresAndMerges`, `TestNilTimerIsHarmless`) uses
`Start`/`Timer`. **Planning implication:** the plan does not need to design a call-site
disposition for `Timer.Done`'s error — there are no callers yet. It should still get the same
signature treatment (`Done(outcome string, fields F) error`, or a documented decision to leave
it swallowing since nothing depends on it) so the type stays consistent with `Emit`, but this is
a much smaller decision than the 10 real call sites below.

### Every `Emit` call site — enumerated

Ten non-test call sites, across three files, all found via:

```bash
grep -rn "events\.Emit" --include="*.go" internal cmd | grep -v _test.go
```

| # | File:Line | Event name | Context | What happens today if this line's write fails |
|---|-----------|------------|---------|------------------------------------------------|
| 1 | `internal/tender/start.go:117` | `tender.start` (outcome `failed`) | Inside the `if err := tmux(args...); err != nil` branch — tmux launch already failed, function already returning that error | Nothing observably changes: `err` (the tmux error) is what gets returned regardless |
| 2 | `internal/tender/start.go:126` | `tender.start` (outcome `ok`) | Success path, immediately before `return t, nil` | Tender start "succeeds" with no observable trace that the event recording it failed |
| 3 | `internal/tender/start.go:339` | `tender.stop` | Inside `Stop(t Tender, removeWorktree bool) error`, before the tmux `kill-session` call | Stop proceeds to kill the session and clean up regardless |
| 4 | `internal/gate/run.go:32` (via `step` closure, called ~7 times through `Run`) | `gate.stage` | Called at every gate stage transition (kind, result, test, review, merge, retest, push, close) | Gate proceeds to the next stage regardless |
| 5 | `internal/gate/run.go:46` (via `finishAs` closure) | `gate.run` | Called at every gate exit (refused, stopped, finished) | The `Report` is still returned to the CLI caller regardless |
| 6 | `internal/gate/run.go:105` | `gate.test` (`on: branch`) | After running tests on the tender's own branch, before `step(StageTest, ...)` | Gate testing/stepping continues regardless |
| 7 | `internal/gate/run.go:134` | `gate.review` | After the review agent returns a verdict | Gate review flow continues regardless |
| 8 | `internal/gate/run.go:189` | `gate.test` (`on: merged`) | After running tests on the merged tree | Gate continues to landing regardless |
| 9 | `internal/gate/run.go:225` | `gate.land` | Immediately after `step(StagePush, true, ...)`, before `beads.Close(...)` | Landing (already pushed) proceeds to closing the bead regardless |
| 10 | `internal/cli/dispatch.go:174` (inside `handBack`) | `tender.handback` | Before calling `beads.HandBack(t.Repo, t.Bead, note)` | `HandBack` (the bd write) still runs regardless |

`[VERIFIED: internal/tender/start.go:90-343, internal/gate/run.go:1-243, internal/cli/dispatch.go:150-189]`
— all read this session; line numbers and event names quoted verbatim from source.

**Blast radius verdict:** small. Three files, ten call sites, all following the identical shape
`events.Emit(events.Event{...})` with the return value currently discarded. None of them are
inside a hot loop; `gate.stage` is the only one called more than twice per command invocation
(once per gate stage, ≤8 times). Every call site is already inside a function that returns
`error` (`Run`, `Stop`, `handBack`'s caller) or is the terminal statement before a `return`, so
adding `if err := events.Emit(...); err != nil { ... }` is a mechanical, low-risk edit at each
site — the risk is entirely in choosing the right *disposition* (below), not in the mechanics of
threading the error through Go's type system.

### Disposition at each call site (SUB-01 + criterion 4)

Criterion 4 is explicit: *"A failing event write is visible but never destroys the work it was
instrumenting — gate and tender still complete, and the gardener is told."* This rules out
`if err := events.Emit(...); err != nil { return err }` at any of the 10 sites — that would make
an unwritable event log turn a successful gate/tender/handback into a hard failure, which is
strictly worse than today's silent swallow.

The mechanism this phase should use, consistent with the project's existing conventions
(`.planning/codebase/CONVENTIONS.md`: "No Structured Logging... Production code uses formatted
print statements", "Use `fmt.Fprintf(os.Stderr, ...)` for error messages"): **report-and-continue
via a stderr note, at the point of failure**, plus (for SUB-03) a persisted first-failure marker
written from inside `events.Emit` itself so retrospective health-checking works even for a
gardener who was not watching the terminal when the failure happened.

```go
// at every one of the 10 call sites, replace:
events.Emit(events.Event{...})
// with:
if err := events.Emit(events.Event{...}); err != nil {
    fmt.Fprintf(os.Stderr, "hugel: event %q not recorded: %v\n", "tender.start", err)
}
// ...then fall through to whatever the function already does next.
```

This satisfies criterion 1 ("the gardener sees the failure reported by the command that emitted
the event") because the stderr note is synchronous, in-process, and does not depend on any
further disk write succeeding — it is the one signal that works even under a fully-unwritable
`$HUGEL_HOME` (see the SUB-03 section below for why the persisted marker alone is not sufficient
for that case). It satisfies criterion 4 because none of the 10 sites change their return value
or control flow — they only add an `if` block that prints and falls through.

**`gate/run.go`'s two closures (`step`, `finishAs`) need the disposition once, not seven times.**
Since every `gate.stage` and `gate.run` emission goes through `step`/`finishAs`, put the
check-and-report inside those two closures rather than duplicating it at each of the ~7 call
sites that invoke `step(...)`. This is the one place in the blast radius where fixing the closure
fixes multiple call sites at once — flag this explicitly for the planner so tasks are not
duplicated across all `step()` call sites individually.

### How the event file path is derived

`internal/config/config.go:Home()` (lines 30-39):
```go
func Home() (string, error) {
	if h := os.Getenv("HUGEL_HOME"); h != "" {
		return Sandbox(h), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return Sandbox(filepath.Join(home, ".hugel")), nil
}
```
`[VERIFIED: internal/config/config.go:30-39]`

`internal/events/events.go:Path()` (lines 131-137) joins `Home()` with `"events.jsonl"`.
`Sandbox()` (`internal/config/sandbox.go:32-39`) panics — **only while `testing.Testing()` is
true** — if the resolved home directory is not inside a temp directory, to stop a test from ever
writing the gardener's real garden. In production (`testing.Testing() == false`) `Sandbox` is a
no-op passthrough. This is the mechanism `TestEmittingWithoutATemporaryGardenFailsTheTest`
exercises (a *test-safety* panic, unrelated to production durability — do not conflate the two
when designing SUB-01's disposition).
`[VERIFIED: internal/config/sandbox.go:1-90]`

### CLI command surface (for SUB-03)

No cobra, no third-party CLI framework. `internal/cli/cli.go:Run` is a flat `switch args[0]`
dispatcher (`[VERIFIED: internal/cli/cli.go:37-80]`) calling `runYield`, `runGate`, `runTender`,
etc., each defined in its own `internal/cli/<command>.go` file using stdlib `flag.FlagSet`. The
`yield` command already implements exactly the "several report views behind boolean flags"
pattern SUB-03 needs:

```go
// internal/cli/yield.go:37-49 (flags), :88-96 (dispatch to view)
soilRep  = fs.Bool("soil", false, "report draws from the pile rather than spend")
changes  = fs.Bool("changes", false, "report what each landed bead cost")
spikes   = fs.Bool("spikes", false, "report what each spike put in the pile and what came of it")
asJSON   = fs.Bool("json", false, "emit JSON")
...
if *soilRep {  return showSoil(all_, f, *asJSON) }
if *changes {  return showChanges(all_, f, *asJSON, *limit) }
if *spikes {   return showSpikes(f, *asJSON) }
```
`[VERIFIED: internal/cli/yield.go:37-96]`

`internal/cli/yield.go` already imports `internal/events` and already calls `events.Load()`
(`internal/cli/yield.go:481`, inside `gatherAttempts`) to build its cost reports.

**Recommendation:** add `--health` as a fifth boolean view on the existing `hugel yield` command
(`showHealth(asJSON bool)` following the exact shape of `showSoil`/`showChanges`/`showSpikes`),
rather than inventing a new top-level command. This is the idiomatic fit: it reuses the file
that already imports `events`, already has the `--json` convention, and keeps the flat "one verb
per top-level command, views behind flags" surface the CLI already has. A new top-level `hugel
health` command would also work but breaks the existing "reporting commands live under `yield`"
grouping without a compensating benefit.

### Test conventions (from `.planning/codebase/TESTING.md` and the actual `_test.go` files)

- Runner: `go test ./...` (`Makefile:test`), no external assertion library, table-driven tests
  preferred.
- Every test that touches the garden sets `t.Setenv("HUGEL_HOME", t.TempDir())` — confirmed in
  `internal/events/events_test.go`, `internal/draws/draws_test.go`,
  `internal/config/config_test.go`.
- `Sandbox()` (`internal/config/sandbox.go`) panics if a test resolves the garden home outside a
  temp directory, as a defense-in-depth backstop for the `t.Setenv` convention.
- The unwritable-directory case is **already tested** in this exact package:
  `TestEmitCannotFailTheCaller` (`internal/events/events_test.go:140-155`) creates a *file* at
  the path `HUGEL_HOME` would resolve to (so `os.MkdirAll(filepath.Dir(p), ...)` fails because a
  path component is not a directory), then calls `Emit` in a goroutine and asserts no panic:
  ```go
  func TestEmitCannotFailTheCaller(t *testing.T) {
      garden := filepath.Join(t.TempDir(), "unwritable")
      if err := os.WriteFile(garden, []byte("i am a file, not a directory"), 0o644); err != nil {
          t.Fatal(err)
      }
      t.Setenv("HUGEL_HOME", garden)
      done := make(chan bool, 1)
      go func() {
          defer func() { done <- recover() == nil }()
          Emit(Event{Name: "into the void", Bead: "x-1"})
      }()
      if !<-done {
          t.Fatal("Emit panicked when the log could not be written")
      }
  }
  ```
  `[VERIFIED: internal/events/events_test.go:140-155]` — this test **must be rewritten**, not
  deleted, once `Emit` returns `error`: the new version should assert `Emit` returns a non-nil,
  descriptive error under this exact "HUGEL_HOME points at a file" condition, in addition to
  still not panicking.
- Reader-side tests in other packages (`internal/gate/gate_test.go:310,367`,
  `internal/tender/tender_test.go:466`, `internal/survival/survival_test.go:68-74`,
  `internal/yield/changes_test.go:151-195`) call `events.Load()` or construct `[]events.Event`
  literals directly — none of them call `Emit` with its return value discarded in a way that
  would break when the signature changes to return `error`, since Go allows (but the linter/vet
  will flag) an unused single return value only via explicit `_ =` or ignoring in a statement
  context. **Planning implication:** these four test files should not need edits for the
  signature change itself, only `events_test.go` and the three production files in the blast
  radius table above.

## Research: SUB-01 blast radius

Covered fully above (Codebase Orientation section). Summary: `Emit(e Event) error`, 10 call
sites in 3 files, disposition is stderr-note-and-continue, with the two `gate/run.go` closures
(`step`, `finishAs`) absorbing 7 of the 10 sites into 2 edits.

## Research: SUB-02 durability in Go

**Concrete pattern:**

```go
f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
if err != nil {
    return fmt.Errorf("open event log: %w", err)
}
defer f.Close()
if _, err := f.Write(append(b, '\n')); err != nil {
    return fmt.Errorf("write event: %w", err)
}
if err := f.Sync(); err != nil {
    return fmt.Errorf("sync event log: %w", err)
}
return nil
```

This is `os.OpenFile` + `O_APPEND` (already used) + `Write` + `f.Sync()` before `Close()` — the
standard Go durable-append pattern. Two platform nuances the plan should account for:

1. **`Sync()` must be called before `Close()`, and its error must not be swallowed.** A common
   mistake is calling `Sync()` in a `defer` alongside `Close()` and discarding the error, or
   relying on `Close()` alone — on Linux, `Close()` does not guarantee the data reached stable
   storage; only an explicit successful `Sync()` (fsync) does.
   `[CITED: Go community discussion cross-referencing Linux fsync semantics — https://groups.google.com/g/Golang-Nuts/c/tHtIVXmYhVk]`
   — MEDIUM confidence (community source, not `golang.org` official docs, but consistent with
   documented POSIX `fsync(2)` semantics).
2. **On macOS, `os.File.Sync()` calls `fsync(2)`, which does *not* flush the drive's write
   cache** — Darwin requires `fcntl(fd, F_FULLFSYNC)` for a true durability guarantee, and this
   is a long-standing open issue against the Go toolchain itself, not something this project's
   code can silently work around without an OS-specific build (`golang.org/x/sys/unix` +
   build-tagged file).
   `[CITED: github.com/golang/go issue #26650 — "os: File.Sync() for Darwin should use the
   F_FULLFSYNC fcntl"]` — this is a primary source (the Go project's own issue tracker), MEDIUM
   confidence since it documents a known gap rather than a fix that has shipped as of this
   research (verify current `go1.26` behavior if this matters at execution time — the phase's
   dev machine is macOS 15.3.2 per this session's `sw_vers` output, so the gap is directly
   relevant here). **Recommendation for the plan:** call `f.Sync()` and treat its error as
   authoritative (SUB-02 only requires that a crash after a *returned* success does not lose the
   tail — `f.Sync()` returning `nil` is the strongest cross-platform signal Go's stdlib exposes
   without adding a build-tagged `F_FULLFSYNC` path; do not gold-plate this phase with
   platform-specific fcntl calls unless a later phase's requirements demand it).
3. **Directory-entry sync (new-file case).** On Linux, `fsync()` on a file guarantees the file's
   *data* is durable but not that the *directory entry* making the file discoverable survives a
   crash — that requires a separate `fsync` on the parent directory's file descriptor, and matters
   only the moment a file is newly created (`O_CREATE` actually creates it) rather than on every
   append to an already-existing file.
   `[CITED: Linux fsync(2) durability discussion, cross-referenced against multiple sources including the WAL-log article surfaced in this session's search — general POSIX behavior, not specific to this project]`
   — MEDIUM confidence, general knowledge. **Given this project's write volume (~5 events/day,
   `.planning/PROJECT.md` "Out of Scope" table) and that `events.jsonl` is created once ever and
   appended to for the file's entire life**, the risk window (a crash in the exact instant
   between file creation and the first successful data fsync, before the directory entry itself
   is durable) is vanishingly narrow. Recommend noting this as a documented, accepted gap rather
   than adding directory-fsync code — it would meaningfully complicate `Emit` (need a directory
   file handle, need to detect "this call created the file" vs. "file already existed") for a
   failure mode this project's own scale argument (Out of Scope table, "~5 events/day against
   tooling for six-figure writes/sec") already argues against over-engineering for.

**Cost at this project's write volume:** an `fsync` call on typical local SSD storage (the target
platform — no network filesystem is in scope per STACK.md) costs low single-digit milliseconds.
At ~5 events/day this is immeasurably below any threshold that would matter to a gardener running
interactive CLI commands. **No perf gate is needed for this phase's fsync addition** —
flag this in the plan's success criteria as "no explicit performance test needed, cost is
negligible by two orders of magnitude" rather than asking the planner to invent a benchmark.

**File stays open per-event, not per-process** (see Codebase Orientation above) — this phase
should not change that architecture. There is no long-lived `*os.File` handle held across the
process lifetime today, and introducing one would be a larger, riskier change (lifecycle
management, handling `HUGEL_HOME` changing mid-process in tests, etc.) than SUB-02 requires.
Keep the existing per-call open/write/sync/close shape; only add the `Sync()` call and the error
return.

## Research: SUB-03 health surface

**What "healthy" must distinguish:** "nothing has run since `<date>`" (no failures — the log's
last write is simply old because no work has happened) from "writes have been failing since
`<date>`" (work has been happening, but the log stopped capturing it). These require two
independent facts:

1. **Last-written timestamp.** No new state needed — `os.Stat($HUGEL_HOME/events.jsonl).ModTime()`
   already answers this, since every successful `Emit` appends via `O_APPEND`, which updates the
   file's mtime. This is read-only; no change to `Emit` is required to support it.

2. **First-failure timestamp (the "since when has this been failing" fact).** This state does
   **not exist anywhere today** and must be created by this phase. `internal/cli/compost.go`
   already establishes the exact idiom to follow — a zero-byte marker file whose *mtime* is the
   payload, using `os.Chtimes`:
   ```go
   // internal/cli/compost.go:256-265 (existing pattern to mirror)
   func compostMark() (string, error) {
       home, err := config.Home()
       if err != nil { return "", err }
       return filepath.Join(home, "composted"), nil
   }
   func markComposted() error {
       p, err := compostMark()
       if err != nil { return err }
       if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { return err }
       f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644)
       if err != nil { return err }
       defer f.Close()
       now := time.Now()
       return os.Chtimes(p, now, now)
   }
   ```
   `[VERIFIED: internal/cli/compost.go:256-282]`

   For events, the marker must be **sticky on first failure, cleared on next success** (not
   re-touched on every failure, or "since" would always read "just now"):
   ```go
   func failMarkPath() (string, error) { /* Home() + "events.failing-since" */ }

   // inside Emit's error paths, on first failure only:
   func markFailing() {
       p, err := failMarkPath()
       if err != nil { return }
       // O_EXCL: a mark that already exists means we are already in a failing
       // streak: leave its mtime (the *first* failure's time) untouched.
       f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
       if err != nil { return } // already marked, or genuinely can't write here either
       f.Close()
   }
   // on the next successful Emit:
   func clearFailing() {
       p, err := failMarkPath()
       if err != nil { return }
       _ = os.Remove(p) // ignore not-exist; a clean streak has nothing to clear
   }
   ```
   Note `os.OpenFile(..., os.O_CREATE|os.O_EXCL, ...)` (`cli/dispatch.go:202` already uses this
   exact exclusive-create idiom for a different purpose — `[VERIFIED: internal/cli/dispatch.go:198-202]`
   — confirming `O_EXCL` is an idiom already present in this codebase, not a new pattern).

3. **The chicken-and-egg case the phase's own research question names explicitly: "an event log
   that cannot be written cannot record its own failure."** If `$HUGEL_HOME` itself is fully
   inaccessible (bad `HUGEL_HOME`, permission-denied directory, or a disk truly at 100% free
   space including inode/block slack), the `events.failing-since` marker write will fail for the
   *same reason* the events write failed — there is no third location to fall back to that isn't
   subject to the identical failure. **This is why the stderr note (criterion 1's mechanism) is
   load-bearing and the marker file is not sufficient on its own:** the stderr note is emitted
   synchronously in the same process, at the moment of failure, and requires no further disk
   write to succeed — it reaches the gardener's terminal even when every persistence layer is
   down. The marker file is a best-effort *retrospective* signal for the common partial-failure
   case (e.g. `events.jsonl` itself made read-only while `$HUGEL_HOME` remains writable, or the
   whole `$HUGEL_HOME` was writable when the failure started but a later `hugel yield --health`
   check is asked about it). **Plan this as two independent lines of defense, not one
   mechanism standing in for the other**, and document in the plan's success criteria that a
   fully-inaccessible `$HUGEL_HOME` degrades SUB-03's retrospective health check to "no answer
   available" rather than a wrong answer — the stderr note at the time of the actual command is
   the only channel guaranteed to work in that case.

**Recommended `hugel yield --health` output shape** (plain-text, following the existing
`showSoil`/`showChanges` rendering style in `internal/cli/yield.go`):
```
event log:  healthy
last write: 2026-09-08 14:32 (3h ago)
```
or, in the failing case:
```
event log:  FAILING since 2026-09-01 09:14 (7d ago)
last write: 2026-08-25 11:02 (14d ago)
```
`--json` variant returns a struct with `Healthy bool`, `LastWrite time.Time`, `FailingSince
*time.Time` (nil when healthy) — mirroring the existing `enc := json.NewEncoder(os.Stdout)`
pattern already used by every other `yield` view.

## Testing the unwritable case

The codebase **already has a working, portable technique**, proven in
`TestEmitCannotFailTheCaller` (quoted above): make `HUGEL_HOME` point at a path where a
*required intermediate directory component is actually a regular file*, so `os.MkdirAll`
fails with `ENOTDIR`. This works identically on macOS and Linux and, critically, **works when
tests run as root**, because it is not a permission-bit check — it fails because a directory
operation is attempted against a non-directory path component, which the OS refuses regardless
of the calling user's privilege level.

**Why `os.Chmod(dir, 0o444)` (read-only directory) is the wrong primary technique for this
repo's test suite:** on Unix systems, the root user bypasses discretionary file-permission
checks entirely — a process running as root can still write into a directory chmod'd `0o444` (or
write to a file chmod'd `0o444`) because permission bits are a *discretionary* access-control
mechanism the kernel does not enforce against `CAP_DAC_OVERRIDE` (which root has by default).
`[CITED: general POSIX/Linux behavior, cross-referenced against Baeldung "How Do File Permissions
Work for the Root User" — https://www.baeldung.com/linux/root-user-file-permissions]` — MEDIUM
confidence (well-established OS behavior, secondary source, not project-specific). Many CI
containers (including common GitHub Actions Docker-based runners) execute as `root` by default,
so a test relying purely on `chmod 0o444` would silently pass (fail to reproduce the unwritable
condition) in exactly the environment most likely to run it unattended.

**Recommended test techniques, ranked:**

| Technique | Portable macOS+Linux | Works as root | Notes |
|-----------|----------------------|----------------|-------|
| `HUGEL_HOME` resolves through a regular file (existing `TestEmitCannotFailTheCaller` pattern) | Yes | Yes | The proven, existing technique — reuse directly for the new "returns error" assertion |
| Bad/garbage `HUGEL_HOME` pointing somewhere `os.MkdirAll` cannot create (e.g. under a nonexistent device path) | Partially | Depends | Less portable than the file-as-directory trick; not recommended as primary |
| `os.Chmod(dir, 0o444)` read-only directory | Yes | **No — silently succeeds as root, defeating the test** | Only use as a *secondary* test, explicitly skipped or asserted-non-root when `os.Geteuid() == 0` |
| `/dev/full` (writes fail with `ENOSPC`) | Linux only — **does not exist on macOS** | Yes (device-level failure, not permission-based) | Cannot be the primary/only technique since this project's dev and research environment is macOS (confirmed via this session's `sw_vers`/`go version` output: macOS 15.3.2, darwin/arm64) |

**Recommendation:** the plan's test for "unwritable event file" should extend the existing
`TestEmitCannotFailTheCaller` pattern (file-as-directory-component) as the primary, always-run
case; it already covers macOS+Linux+root uniformly. If the plan wants to additionally cover "the
directory exists and is writable but the *file itself* is not" (e.g. someone `chmod 444
events.jsonl` directly), guard that sub-test with `if os.Geteuid() == 0 { t.Skip(...) }` so CI
running as root does not get a false pass. Do not attempt `/dev/full` given the project's
macOS-first development environment — it is Linux-only and would need a build-tag-gated test to
avoid failing to compile/run on macOS.

## Architecture Patterns

### System Flow: an event write, before and after this phase

```
BEFORE (current):
  caller (tender/gate/dispatch)
     │
     ▼
  events.Emit(Event{...})  ── returns nothing ──▶  caller proceeds unconditionally
     │
     ├─ marshal fails ──────────────▶ silently dropped
     ├─ Home()/Path() fails ────────▶ silently dropped
     ├─ MkdirAll fails ─────────────▶ silently dropped
     ├─ OpenFile fails ─────────────▶ silently dropped
     └─ Write fails ────────────────▶ silently dropped (no Sync at all)


AFTER (this phase):
  caller (tender/gate/dispatch)
     │
     ▼
  err := events.Emit(Event{...})
     │
     ├─ any internal failure ─▶ Emit returns descriptive error
     │                             │
     │                             ├─▶ markFailing() (first-failure marker, sticky)
     │                             ▼
     │                    caller: fmt.Fprintf(stderr, "event %q not recorded: %v", ...)
     │                             │
     │                             ▼
     │                    caller proceeds with its own work UNCHANGED (criterion 4)
     │
     └─ success ──▶ Write, then f.Sync() (durable before Emit returns nil)
                        │
                        ▼
                  clearFailing() (remove marker if one existed)


READ SIDE (new, SUB-03):
  hugel yield --health
     │
     ├─▶ os.Stat(events.jsonl).ModTime()        → "last write: ..."
     └─▶ os.Stat(events.failing-since), if exists → "FAILING since: ..."
```

### Recommended code shape (illustrative, not a full diff)

```go
// internal/events/events.go — Emit's new shape
func Emit(e Event) error {
	if e.Time.IsZero() {
		e.Time = now()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	p, err := Path()
	if err != nil {
		return fmt.Errorf("resolve event log path: %w", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		markFailing()
		return fmt.Errorf("create garden dir: %w", err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		markFailing()
		return fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		markFailing()
		return fmt.Errorf("write event: %w", err)
	}
	if err := f.Sync(); err != nil {
		markFailing()
		return fmt.Errorf("sync event log: %w", err)
	}
	clearFailing()
	return nil
}
```

```go
// internal/tender/start.go:126 — disposition at a call site
if err := events.Emit(events.Event{
	Name: "tender.start", Bead: t.Bead, Bed: t.Bed, Outcome: "ok",
	Fields: events.F{ /* ... unchanged ... */ },
}); err != nil {
	fmt.Fprintf(os.Stderr, "hugel: event %q not recorded: %v\n", "tender.start", err)
}
return t, nil // unchanged: SUB-01/criterion 4 — the tender still starts
```

### Anti-Patterns to Avoid

- **Propagating the event-write error as the function's own error.** Turns an observability gap
  into a work-blocking failure; directly violates criterion 4.
- **Re-touching the failure marker's mtime on every subsequent failure.** Destroys the "since
  when" fact SUB-03 needs — the marker must be created with `O_EXCL` (fails harmlessly if it
  already exists) precisely so its mtime stays pinned to the *first* failure in a streak.
- **Relying on `os.Chmod` alone to simulate "unwritable" in tests.** Silently passes when the
  test binary runs as root (common in CI containers) — the file-as-directory-component technique
  already proven in this package does not have that blind spot.
- **Holding a long-lived `*os.File` handle across the process to "avoid open/close overhead."**
  Not needed at this write volume (~5/day), and would require solving cache-invalidation for
  `HUGEL_HOME` changing mid-test — a much larger change than SUB-02 asks for.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Durable single-writer append | A custom write-ahead-log or buffered-writer-with-flush abstraction | `os.OpenFile(O_APPEND\|O_CREATE\|O_WRONLY)` + `Write` + `f.Sync()`, exactly as already sketched above | At ~5 events/day this project has no throughput problem to solve; a WAL/buffering layer adds real complexity (background flush goroutine, shutdown-drain logic) for zero measurable benefit and is explicitly the kind of over-engineering `.planning/PROJECT.md`'s Out of Scope table (Cassandra/Kafka rejection) already argues against |
| "Since when has X been failing" tracking | A structured state machine / status enum persisted in JSON | A single mtime-bearing marker file, following `compostMark`'s exact idiom | The codebase already has this exact pattern solving an isomorphic problem (`internal/cli/compost.go`); duplicating it as a new bespoke mechanism would be inconsistent with `.planning/codebase/CONVENTIONS.md`'s stated preference for small, direct solutions over abstraction |

**Key insight:** this phase has no "don't hand-roll, use a library" case in the usual sense —
there is no external durability/logging library this project should adopt (and PROJECT.md's Out
of Scope table explicitly rules out server-backed logging stacks). The relevant discipline here
is *don't hand-roll a bigger abstraction than the actual requirement*, not "use an existing
library instead of hand-rolling."

## Common Pitfalls

### Pitfall 1: Treating `Emit`'s error return as something callers must fail on

**What goes wrong:** A well-intentioned but incorrect read of SUB-01 ("errors should propagate")
leads to `if err := events.Emit(...); err != nil { return err }` at one or more of the 10 call
sites, which then fails a `gate.Run` or `tender.Start` because the *observability* write failed,
not the *actual work*.
**Why it happens:** SUB-01's phrasing ("returns an error instead of swallowing it") reads, out of
context, like ordinary Go error handling — but criterion 4 is the disambiguating constraint and
must be read together with SUB-01, not treated as a separate, lower-priority requirement.
**How to avoid:** Every call site's disposition should be reviewed against criterion 4 explicitly
during planning; the plan's verification step should include a test that forces an event-write
failure mid-`gate.Run` (or mid-tender-start) and asserts the gate/tender *still completes
successfully* despite it.
**Warning signs:** Any new `return err` immediately following an `events.Emit` call in the diff.

### Pitfall 2: Losing the "since" fact by re-marking on every failure

**What goes wrong:** Using `os.OpenFile(..., os.O_CREATE|os.O_WRONLY, ...)` (without `O_EXCL`)
or `os.Chtimes` unconditionally for the failure marker resets its mtime on every failed `Emit`,
so `hugel yield --health` always reports "failing since just now" no matter how long the streak
has actually run.
**Why it happens:** `O_CREATE|os.O_WRONLY` is the pattern used everywhere else in this codebase
for *successful*-write markers (`compostMark`, `config.Save`) where "most recent write wins" is
exactly the desired semantic — it is easy to copy that idiom without noticing the semantic is
inverted for a *first*-failure marker.
**How to avoid:** Use `O_CREATE|O_EXCL` for the failure marker specifically (already an idiom
present elsewhere in this codebase — `internal/cli/dispatch.go:202` — so this is not introducing
an unfamiliar pattern).
**Warning signs:** A test that emits two failures in a row and asserts the "since" timestamp
changed between them (should be a failing test if this pitfall is present).

### Pitfall 3: `f.Sync()` error swallowed by a bare `defer f.Close()`

**What goes wrong:** If `Sync()`'s error is discarded (e.g. by only calling `Close()` and
ignoring its own error, or calling `Sync()` inside a `defer` where the error is dropped), the
function can return `nil` (success) even though the data never reached durable storage.
**Why it happens:** `defer f.Close()` is already idiomatic in this codebase for cleanup; it is
easy to reach for `defer` again for `Sync()` without noticing `Sync()` must be checked
*synchronously*, before the success path returns, not deferred.
**How to avoid:** Call `f.Sync()` as a normal (non-deferred) statement, check its error, and only
return `nil` after it succeeds. Keep `defer f.Close()` for cleanup only — `Close()`'s own error
is secondary once `Sync()` has already succeeded (the data is durable; a subsequent close failure
does not un-durable it).
**Warning signs:** Any `_ = f.Sync()` or `defer f.Sync()` in the diff.

## Code Examples

See the "Recommended code shape" block under Architecture Patterns above — the concrete `Emit`
rewrite and one representative call-site disposition, both grounded in code read this session.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|---------------|--------|
| `Emit(e Event)` — no return, all errors silently dropped, no fsync | `Emit(e Event) error` — every internal failure surfaced, `f.Sync()` before success | This phase (SUB-01/SUB-02) | The package's own doc comment (lines 141-147) and one test (`TestEmitCannotFailTheCaller`) both currently assert the *opposite* contract and must be rewritten as part of this phase, not merely extended |

**Deprecated/outdated:** The package doc comment's rationale for swallowing errors
(`internal/events/events.go:141-147`) is the exact position this phase reverses. The plan should
include updating that comment (not just the code) so the doc and behavior stay in sync — a stale
doc comment arguing for behavior the code no longer has is worse than no comment, per this
project's own stated conventions around comments explaining "why."

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Go's `os.File.Close()` does not itself guarantee durability on Linux; an explicit `Sync()` is required | SUB-02 durability | If wrong (i.e. if `Close()` already fsyncs), the extra `Sync()` call is merely redundant/harmless, not incorrect — low risk either way |
| A2 | On macOS, `os.File.Sync()` maps to `fsync(2)` rather than `fcntl(F_FULLFSYNC)`, so it does not fully flush the drive cache | SUB-02 durability | If the Go toolchain has since closed golang/go#26650, this caveat is stale and the plan can drop the "don't gold-plate" recommendation; either way it does not change the code this phase should write, only how strong a durability claim can be made in the phase's success-criteria language |
| A3 | Root bypasses `chmod`-based permission restrictions on Unix, making read-only-directory tests unreliable under root-run CI | Testing the unwritable case | If a specific CI environment enforces permissions even for root (e.g. via a security module), the `os.Chmod` sub-test would work there too — but the file-as-directory-component primary technique is unaffected either way, so this only affects whether the *secondary* test needs a root-skip guard |
| A4 | `/dev/full` is Linux-only and unavailable on macOS | Testing the unwritable case | Low risk — this is a well-known, stable Unix convention difference; if wrong, the plan simply has one more portable option available, not a broken one |

**If this table is empty:** N/A — see entries above. All four assumptions are corroborating,
non-authoritative web sources layered on top of directly-verified codebase facts; none of them
gate a design decision that cannot be revisited cheaply if wrong (see "Risk if Wrong" column).

## Open Questions

1. **Does `Timer.Done` need an error return too, given it has zero production callers?**
   - What we know: `Start`/`Timer` exist, are tested, and are unused in production code today.
   - What's unclear: Whether a future phase (Phase 2, which widens event emission to more
     subsystems) will start using `Timer` for its "measure from start to Done" convenience, in
     which case leaving `Done` swallowing errors while `Emit` does not would be an inconsistency
     baked in now that Phase 2 inherits.
   - Recommendation: Give `Done` the same `error`-returning treatment as `Emit` (it already just
     forwards to `Emit` internally) for consistency, even though no call site today exercises it.
     Low cost, avoids a foreseeable inconsistency.

2. **Should the `--health` view live under `hugel yield` or become its own top-level command?**
   - What we know: `hugel yield` already has four boolean-flag views and already imports/reads
     `events`; the CLI's existing "verb + flag-selected views" convention fits a fifth view
     cleanly.
   - What's unclear: Whether a future phase (the roadmap's Phase 5, "The Resident Garden") wants
     event-log health surfaced prominently in the TUI rather than as a CLI report line, which
     might argue for a more standalone, easily-embeddable function regardless of which CLI
     command exposes it.
   - Recommendation: Implement the underlying logic as an exported function in `internal/events`
     (e.g. `events.Health() (Health, error)`) so it is trivially reusable from a future TUI
     surface, and expose it via `hugel yield --health` for this phase's CLI surface — the
     function/command split keeps both options open without extra cost now.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Building/testing this phase's changes | ✓ | go1.26.0 darwin/arm64 (confirmed this session) | — |
| `go test ./...` | Verification | ✓ | Standard library testing, no external framework | — |
| No new external Go modules | N/A | N/A | This phase adds zero new `go.mod` entries | — |

No CI workflow file exists in this repository (`.github/workflows` absent, confirmed this
session) — verification for this phase is developer-run `make test` / `go test ./...`, matching
`.planning/codebase/TESTING.md`. If a root-run CI is added later, the "Testing the unwritable
case" guidance above (avoid relying solely on `chmod`) remains the relevant guard.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard library `testing` package, Go 1.26 |
| Config file | none — `go test ./...` via `Makefile:test` |
| Quick run command | `go test ./internal/events/... ./internal/tender/... ./internal/gate/... ./internal/cli/...` |
| Full suite command | `go test ./...` (also `make test`) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SUB-01 | `Emit` returns a non-nil error when the write fails, and does not panic | unit | `go test -run TestEmit ./internal/events/...` | ✅ existing test to rewrite (`TestEmitCannotFailTheCaller`) |
| SUB-01 | Every one of the 10 call sites reports (stderr) but does not fail its enclosing operation when `Emit` errors | integration | `go test -run TestGate ./internal/gate/...` / `go test -run TestTender ./internal/tender/...` (new sub-tests forcing a write failure mid-run) | ❌ Wave 0 — new sub-tests needed in `gate_test.go` and `tender_test.go` |
| SUB-02 | An event written before a simulated crash is present after reopening the log (durability) | unit | `go test -run TestEmitIsDurable ./internal/events/...` | ❌ Wave 0 |
| SUB-02 | `f.Sync()` is called and its error surfaces through `Emit`'s return | unit | Covered by the same durability test plus a forced-sync-failure case if feasible on the target OS | ❌ Wave 0 |
| SUB-03 | `hugel yield --health` distinguishes "nothing has run" from "writes have been failing", using the last-write mtime and the failure marker | integration | `go test -run TestHealth ./internal/cli/...` (and/or `./internal/events/...` for the underlying `Health()` function) | ❌ Wave 0 |
| SUB-03 | The failure marker's mtime stays pinned to the *first* failure across repeated failures, and clears on the next success | unit | `go test -run TestFailureMarker ./internal/events/...` | ❌ Wave 0 |
| Criterion 1 | Unwritable event file (file-as-directory-component technique) → gardener sees the failure reported by the emitting command | integration | Extends `TestEmitCannotFailTheCaller`; add a call-site-level test asserting stderr output | ❌ Wave 0 (extends existing) |
| Criterion 4 | Gate/tender still complete when the event write fails | integration | New sub-test in `gate_test.go`/`tender_test.go` injecting a write failure (e.g. via a temporarily-bad `HUGEL_HOME` mid-test, or a test seam) and asserting `Run`/`Start` still returns success | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/events/... ./internal/tender/... ./internal/gate/... ./internal/cli/...`
- **Per wave merge:** `go test ./...` (full suite; this repo has no separate integration-test tag)
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/events/events_test.go` — rewrite `TestEmitCannotFailTheCaller` to assert a
      returned error (not just "no panic"); add durability test (write, don't crash, but assert
      `Sync` was reached — e.g. via a fake/interceptable file if the plan wants to unit-test
      `Sync` failure specifically, otherwise an integration-level "reopen and Load()" check is
      sufficient given `f.Sync()` itself is stdlib and does not need re-proving)
  — covers SUB-01, SUB-02
- [ ] `internal/events/events_test.go` — new tests for the failure-marker create/pin/clear cycle
      — covers SUB-03
- [ ] `internal/gate/gate_test.go`, `internal/tender/tender_test.go` — new sub-tests forcing an
      event-write failure mid-run and asserting the gate/tender still completes — covers
      criterion 4 (this is the most important gap: it is the only test that directly proves the
      "never destroys the work" guarantee end-to-end rather than at the `events` package's own
      boundary)
- [ ] `internal/cli/yield_test.go` (may not exist yet — check before planning; not confirmed in
      this research pass) — new test(s) for `--health` view, both plain-text and `--json` output,
      and both the "healthy" and "failing since" cases — covers SUB-03
- [ ] Framework install: none — `testing` is stdlib, already present

## Security Domain

`security_enforcement: true`, `security_asvs_level: 1` (per `.planning/config.json`).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | No | Single-user local CLI tool; no auth surface exists or is touched by this phase |
| V3 Session Management | No | No session concept in this phase's scope |
| V4 Access Control | No | No multi-user access boundary; `.planning/PROJECT.md` explicitly rules multi-gardener/shared garden out of scope |
| V5 Input Validation | Marginal | `HUGEL_HOME` is read from an environment variable and used directly in filesystem path construction (`filepath.Join`); this is pre-existing behavior this phase does not change. No new user-controlled input is introduced by SUB-01/02/03 |
| V6 Cryptography | No | No cryptographic operation is introduced or touched |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Event log leaking sensitive data if the file or a backup of it is exposed | Information Disclosure | Already flagged in `.planning/codebase/CONCERNS.md` ("Event log contains sensitive data") — pre-existing, out of this phase's scope (this phase changes *how reliably* events are written, not *what* they contain); do not expand `Event.Fields` content as part of this phase |
| A first-failure marker file or health-check output revealing that `$HUGEL_HOME` is misconfigured/unwritable | Information Disclosure (low severity) | The marker file lives in the same private, user-owned directory as the rest of the garden (`~/.hugel/`, not world-readable by default via standard `0o644`/`0o755` modes already used throughout this codebase); no new exposure surface beyond what `events.jsonl` itself already has |

`[VERIFIED: .planning/codebase/CONCERNS.md:53-57]` for the pre-existing event-log-sensitivity
finding, quoted/summarized above.

## Sources

### Primary (HIGH confidence)
- `internal/events/events.go` (full file, 243 lines) — read this session
- `internal/events/convention.go` — read this session
- `internal/events/events_test.go` (full file) — read this session
- `internal/tender/start.go` (lines 1-350) — read this session
- `internal/gate/run.go` (full file, 377 lines) — read this session
- `internal/cli/dispatch.go` (lines 150-205) — read this session
- `internal/cli/cli.go` (full file) — read this session
- `internal/cli/yield.go` (relevant sections) — read this session
- `internal/config/config.go`, `internal/config/sandbox.go` — read this session
- `internal/draws/draws.go`, `internal/draws/draws_test.go` — read this session
- `internal/cli/compost.go` (marker-file idiom, lines 230-282) — read this session
- `.planning/codebase/ARCHITECTURE.md`, `CONCERNS.md`, `CONVENTIONS.md`, `STACK.md`,
  `STRUCTURE.md`, `TESTING.md`, `INTEGRATIONS.md` — read this session
- `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md`, `.planning/PROJECT.md`,
  `.planning/STATE.md`, `.planning/config.json` — read this session
- `grep -rn "events\.Emit\|events\.Start"` run against the full repo this session — confirms
  call-site enumeration and confirms zero production `Timer`/`Start` callers

### Secondary (MEDIUM confidence)
- Go community discussion on `os.File.Sync()`/`Close()` durability semantics —
  https://groups.google.com/g/Golang-Nuts/c/tHtIVXmYhVk
- `golang/go` issue #26650, "os: File.Sync() for Darwin should use the F_FULLFSYNC fcntl" —
  https://github.com/golang/go/issues/26650 (primary source for the Go project itself, but
  documents an open/unresolved gap rather than settled current behavior — verify against the
  installed `go1.26.0` toolchain if the plan needs a stronger guarantee)
- Baeldung, "How Do File Permissions Work for the Root User" —
  https://www.baeldung.com/linux/root-user-file-permissions

### Tertiary (LOW confidence)
- None used without cross-checking — every claim above that came from WebSearch was corroborated
  against either a second source or this session's own `sw_vers`/`go version` environment probe.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new stack decision to make; pure Go stdlib, confirmed by reading the
  actual files
- Architecture: HIGH — call-site enumeration and disposition design grounded entirely in code
  read this session, not inferred
- Pitfalls: HIGH for the codebase-specific pitfalls (grounded in the actual `Emit`
  implementation and existing tests); MEDIUM for the cross-platform fsync nuances (grounded in
  cited external sources, not independently reproduced against a real crash in this session)

**Research date:** 2026-09-08
**Valid until:** No expiry driver — this is a stdlib-only, no-external-dependency phase; the
research does not go stale on a library-release cadence. Re-verify only if `go.mod`'s Go version
changes materially, or if the `events` package's shape changes before planning begins.
