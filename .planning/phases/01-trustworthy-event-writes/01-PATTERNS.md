# Phase 1: Trustworthy Event Writes - Pattern Map

**Mapped:** 2026-09-08
**Files analyzed:** 8 (1 core rewrite, 3 call-site files, 4 test files) — no new files created; all edits to existing files
**Analogs found:** 6 / 6 pattern categories matched

## File Classification

| File to Modify | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/events/events.go` (`Emit` signature + durability + marker) | service (domain package, append-only log writer) | file-I/O | `internal/draws/draws.go:Append` (signature shape) + `internal/cli/compost.go:compostMark`/`markComposted` (marker-file idiom) | role-match (two analogs combined) |
| `internal/events/events.go` (new `Health()`) | service (read accessor) | request-response | `internal/cli/yield.go:showSoil` (reads a log + a fact, returns a struct) | role-match |
| `internal/tender/start.go` (3 `Emit` sites) | domain call site inside a function returning `error` | event-driven (fire-and-forget instrumentation call) | `internal/cli/soil.go:recordDraw` (calls `draws.Append`, reports-and-continues on error) | exact (report-and-continue idiom) |
| `internal/gate/run.go` (`step`, `finishAs` closures) | domain call site (closures wrapping ~7 emission points) | event-driven | `internal/cli/soil.go:recordDraw` (same report-and-continue idiom, applied once per closure instead of per call) | exact |
| `internal/cli/dispatch.go:handBack` (1 `Emit` site) | CLI handler / domain call site | event-driven | `internal/cli/soil.go:recordDraw` | exact |
| `internal/cli/yield.go` (new `--health` flag + `showHealth`) | CLI command / route (flag-selected view) | request-response | `internal/cli/yield.go:showSoil` + its `--soil` flag wiring (same file, sibling view) | exact — same file, same convention |
| `internal/events/events_test.go` (rewrite `TestEmitCannotFailTheCaller`, add durability/marker tests) | test | file-I/O / error-injection | `internal/events/events_test.go:TestEmitCannotFailTheCaller` (existing, to be rewritten in place) | exact |
| `internal/gate/gate_test.go`, `internal/tender/tender_test.go` (new sub-tests forcing write failure mid-run) | test | integration/event-driven | `internal/events/events_test.go:TestEmitCannotFailTheCaller` (HUGEL_HOME-through-a-file technique) | role-match (technique to be reused, not the same package) |

## Pattern Assignments

### `internal/events/events.go` — `Emit` signature, durability, failure marker

**Analog 1 (signature shape only — NOT durability):** `internal/draws/draws.go:47-66`

```go
// Append records a draw. Appending one line is atomic enough at this size, and
// the format is readable without a tool, which is the standard the pile is
// held to as well.
func Append(d Draw) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create garden dir: %w", err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal draw: %w", err)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open draw log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write draw log: %w", err)
	}
	return nil
}
```

**Explicit distinction (do not miss this):** `Append` has no `f.Sync()` call anywhere — copy the
`func X(...) error` signature shape, the `fmt.Errorf("<verb> <noun>: %w", err)` wrapping style, and
the open/write/return-on-error structure from this function, but SUB-02's `f.Sync()` requirement
has **no existing sibling to copy** — it must be added fresh as a new statement between `Write` and
`return nil`, checked synchronously (not deferred, not discarded).

**Analog 2 (marker-file idiom — sticky "first failure" marker):** `internal/cli/compost.go:230-282`

```go
// compostMark is the file whose modification time says when composting last
// ran. A timestamp in a file beats a timestamp in a config: it is one syscall
// to read, and touching it is the whole write.
func compostMark() (string, error) {
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "composted"), nil
}

func markComposted() error {
	p, err := compostMark()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	now := time.Now()
	return os.Chtimes(p, now, now)
}
```

**Semantic inversion warning:** `markComposted` uses `O_CREATE|O_WRONLY` (no `O_EXCL`) because
"most recent write wins" is exactly what a success marker wants. The new `events.failing-since`
marker needs the opposite semantic — first failure wins, mtime must stay pinned — so use
`O_CREATE|O_EXCL` instead (see Analog 3 below for the `O_EXCL` idiom already in this codebase),
and read the marker's mtime via `os.Stat(path).ModTime()` the same way `compostMark`'s caller
does (`internal/cli/compost.go:172-179`, not excerpted here — `if st, err := os.Stat(mark); err
== nil { since = st.ModTime()... }`).

**Analog 3 (O_EXCL idiom, already present in this codebase):** `internal/cli/dispatch.go:190-208`

```go
// lockGarden stops two dispatches racing for the same slot. An exclusive create
// is the whole mechanism: if the file is there, somebody else is dispatching.
func lockGarden(name string) (func(), error) {
	home, err := config.Home()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(home, name+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			b, _ := os.ReadFile(path)
			return nil, fmt.Errorf("another %s is running (%s); remove %s if it is not",
				name, strings.TrimSpace(string(b)), path)
		}
		return nil, err
	}
	...
```

Copy the `os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)` line verbatim for
`markFailing()`'s creation call — an `os.IsExist(err)` branch means "already marked, leave it
alone," which is exactly the sticky-marker semantic SUB-03 needs (RESEARCH.md's own sketch at
lines 450-457 already follows this shape; this is its concrete in-repo precedent).

**Full target shape** (already fully sketched and verified against this codebase in
`RESEARCH.md` "Recommended code shape" — reuse that block directly, it is not illustrative-only,
every line was checked against the conventions above during this mapping pass):

```go
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

**Pitfall to guard against (from RESEARCH.md, confirmed against Analog 1):** do not call
`f.Sync()` inside a `defer` or discard its error with `_ =` — it must be a normal, checked
statement before `return nil`, unlike `defer f.Close()` which stays deferred/best-effort.

---

### `internal/events/events.go` — new `Health()` function

**Analog:** `internal/cli/yield.go:showSoil` (`internal/cli/yield.go:271-284`) — the pattern of
"read a log-derived fact plus a filesystem fact, assemble into a small report struct, let the
caller decide plain-text vs JSON":

```go
func showSoil(sessions []*transcript.Session, f yield.Filter, asJSON bool) error {
	log, err := draws.Load()
	if err != nil {
		return err
	}
	dir, err := pile.DefaultRoot()
	if err != nil {
		return err
	}
	var entries []*pile.Entry
	if store, err := pile.Open(dir); err == nil {
		if entries, err = store.All(); err != nil {
			return err
		}
	}

	rep := yield.Soil(sessions, log, entries, f)
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	...
```

For `events.Health()`, the equivalent shape (per RESEARCH.md's SUB-03 section, function lives in
`internal/events` so it is reusable from a future TUI, per Open Question 2's recommendation):

```go
type Health struct {
	Healthy      bool       `json:"healthy"`
	LastWrite    time.Time  `json:"last_write"`
	FailingSince *time.Time `json:"failing_since,omitempty"`
}

func HealthOf() (Health, error) {
	p, err := Path()
	if err != nil {
		return Health{}, err
	}
	var h Health
	if st, err := os.Stat(p); err == nil {
		h.LastWrite = st.ModTime()
	}
	if fp, err := failMarkPath(); err == nil {
		if st, err := os.Stat(fp); err == nil {
			t := st.ModTime()
			h.FailingSince = &t
		}
	}
	h.Healthy = h.FailingSince == nil
	return h, nil
}
```

Note the `if st, err := os.Stat(p); err == nil { ... }` idiom (ignore-if-missing, do not fail the
whole call on a not-yet-existing log) is the same "missing is OK" convention documented in
`.planning/codebase/CONVENTIONS.md` Error Handling item 5.

---

### Call sites: `internal/tender/start.go`, `internal/gate/run.go`, `internal/cli/dispatch.go`

**Analog (exact — this is the report-and-continue idiom already used for an isomorphic
"don't-fail-the-caller" write in this codebase):** `internal/cli/soil.go:85-101`

```go
// A failed write must never cost the caller its soil: this runs inside live
// sessions, and an instrument that can break the thing it measures is worse
// than no instrument.
func recordDraw(s *soil.Soil, budget int) {
	ids := make([]string, 0, len(s.Items))
	for _, it := range s.Items {
		ids = append(ids, it.ID)
	}
	query, _ := redact.FromEnv().Redact(s.Query)
	err := draws.Append(draws.Draw{
		At: time.Now().UTC(), Bed: s.Bed, Query: query, Budget: budget,
		Tokens: s.Tokens, Considered: s.Considered, Entries: ids,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hugel: draw not recorded: %v\n", err)
	}
}
```

This is a direct, already-shipped precedent for the exact SUB-01/criterion-4 disposition: `Append`
(itself the `draws` package's error-returning analog to `Emit`) fails, the caller does not
propagate the error or change its own control flow, and reports via
`fmt.Fprintf(os.Stderr, "hugel: <noun> not recorded: %v\n", err)`. **Match this exact stderr
format for every `Emit` call site** — RESEARCH.md's own sketch (`"hugel: event %q not recorded:
%v\n"`) is consistent with this idiom; use the plain (non-`%q`) form to match `recordDraw`
verbatim: `fmt.Fprintf(os.Stderr, "hugel: event not recorded: %v\n", err)` — or keep the event
name if the plan wants more specificity, but the `hugel: <what> not recorded: %v` shape is the
one to preserve.

**Current call sites, to be edited (verified this session):**

`internal/tender/start.go:117-121` (inside the `tmux` failure branch):
```go
	if err := tmux(args...); err != nil {
		_ = git(o.Repo, "worktree", "remove", "--force", t.Worktree)
		events.Emit(events.Event{
			Name: "tender.start", Bead: t.Bead, Bed: t.Bed, Outcome: "failed",
			Fields: events.F{"error": err.Error()},
		})
		return nil, err
	}
```

`internal/tender/start.go:126-135` (success path, before `return t, nil`):
```go
	events.Emit(events.Event{
		Name: "tender.start", Bead: t.Bead, Bed: t.Bed, Outcome: "ok",
		Fields: events.F{
			"title": o.Bead.Title, "type": o.Bead.Type, "priority": o.Bead.Priority,
			"branch": t.Branch, "worktree": t.Worktree, "tmux": t.Session,
			"repo": o.Repo, "soil_tokens": tokensIn(o.Soil), "spike": o.Spike,
			"has_criteria": strings.TrimSpace(o.Bead.Accept) != "",
			"brief_bytes":  len(brief), "skip_permissions": o.SkipPermissions,
		},
	})
```

`internal/tender/start.go:339-343` (top of `Stop`):
```go
func Stop(t Tender, removeWorktree bool) error {
	events.Emit(events.Event{
		Name: "tender.stop", Bead: t.Bead, Bed: t.Bed, Outcome: t.State(),
		Duration: time.Since(t.Started),
		Fields:   events.F{"worktree_removed": removeWorktree, "branch": t.Branch},
	})
```

`internal/gate/run.go:1-58` (the `step` and `finishAs` closures — **fix once here, not at each of
the ~7 call sites that invoke `step(...)`**):
```go
	step := func(s Stage, ok bool, detail string, took time.Duration) {
		rep.Reached = s
		rep.Stages = append(rep.Stages, StageResultRecord{Stage: s, OK: ok, Detail: detail, Took: took})
		events.Emit(events.Event{
			Name: "gate.stage", Bead: t.Bead, Bed: t.Bed,
			Outcome: outcomeOf(ok), Duration: took,
			Fields: events.F{
				"stage": string(s), "detail": detail,
				"branch": t.Branch, "into": o.Into, "remote": o.Remote,
			},
		})
	}
	finishAs := func(r Report, outcome string) Report {
		events.Emit(events.Event{
			Name: "gate.run", Bead: t.Bead, Bed: t.Bed,
			Outcome: outcome, Duration: time.Since(began),
			Fields: events.F{
				"reached": string(r.Reached), "why": r.Why,
				"stages": len(r.Stages), "branch": t.Branch,
				"dry_run": o.DryRun, "into": o.Into, "remote": o.Remote,
				"tender_duration_ms": time.Since(t.Started).Milliseconds(),
				"spike":              t.Spike,
			},
		})
		return r
	}
```
Add the `if err := events.Emit(...); err != nil { fmt.Fprintf(os.Stderr, ...) }` check inside
these two closure bodies only; the file's existing imports already include `"fmt"`, `"os"`, and
`"github.com/charris/hugel/internal/events"` (`internal/gate/run.go:3-12`).

`internal/cli/dispatch.go:172-186` (`handBack`, already has a report-and-continue stderr
pattern immediately below the `Emit` call for a *different* failure — extend the same treatment to
the `Emit` line itself):
```go
func handBack(t tender.Tender, note, why string) {
	// How a tender's life ended, recorded where the rest of it is. Between this
	// and gate.run, every exit a tender has is on the event log.
	events.Emit(events.Event{
		Name: "tender.handback", Bead: t.Bead, Bed: t.Bed, Outcome: why,
		Duration: time.Since(t.Started),
		Fields: events.F{
			"reason": t.Reason(), "worktree": t.Worktree, "branch": t.Branch,
		},
	})
	if err := beads.HandBack(t.Repo, t.Bead, note); err != nil {
		fmt.Fprintf(os.Stderr, "hugel: %s needs attention but bd could not be told: %v\n", t.Bead, err)
		return
	}
	...
```
Note this function already demonstrates the "report on stderr, then fall through / return early
depending on severity" idiom for `beads.HandBack`'s own error — apply the identical shape (report,
fall through, do not `return`) to the new `events.Emit` check, since criterion 4 requires
`HandBack` to still run even if the event write failed.

---

### `internal/cli/yield.go` — new `--health` view

**Analog:** the existing `--soil` flag + `showSoil` dispatch, same file, `internal/cli/yield.go:37-49`
(flag declarations) and `:85-96` (dispatch):

```go
	var (
		since    = fs.String("since", "30d", "only sessions ending within this window (30d, 2w, 48h)")
		all      = fs.Bool("all", false, "no time limit")
		bed      = fs.String("bed", "", "restrict to one bed")
		sessions = fs.Bool("sessions", false, "list sessions instead of beds")
		session  = fs.String("session", "", "show one session in detail (id prefix is enough)")
		limit    = fs.Int("limit", 20, "rows to show in list views")
		soilRep  = fs.Bool("soil", false, "report draws from the pile rather than spend")
		changes  = fs.Bool("changes", false, "report what each landed bead cost")
		spikes   = fs.Bool("spikes", false, "report what each spike put in the pile and what came of it")
		asJSON   = fs.Bool("json", false, "emit JSON")
		root     = fs.String("root", "", "transcript root (default ~/.claude/projects)")
	)
	...
	if *soilRep {
		return showSoil(all_, f, *asJSON)
	}
	if *changes {
		return showChanges(all_, f, *asJSON, *limit)
	}
	if *spikes {
		return showSpikes(f, *asJSON)
	}
```

Add `health = fs.Bool("health", false, "report whether the event log is being written")` to the
`var (...)` block, add its usage-string line to the `fs.Usage` block (`internal/cli/yield.go:23-33`,
following the `hugel yield --soil` line format: `  hugel yield --health                       whether the event log is being written`),
and add `if *health { return showHealth(*asJSON) }` alongside the other three view dispatches —
note `--health` does not need `sessions`/`since`/`bed` filters since it reads `events.HealthOf()`
directly, so it can dispatch early (before `all_`/`f` are built) unlike `--soil`/`--changes`.

`showHealth` itself should follow `showSoil`'s render shape (`internal/cli/yield.go:271-291`):
JSON branch uses `enc := json.NewEncoder(os.Stdout); enc.SetIndent("", "  "); return
enc.Encode(rep)`; plain-text branch uses `fmt.Printf`/`fmt.Println` per RESEARCH.md's recommended
output shape (`event log:  healthy` / `last write: ...` or `event log:  FAILING since ...`).

---

### Test: forcing a write failure (`internal/events/events_test.go`)

**Analog (existing, to be rewritten in place, not deleted):**
`internal/events/events_test.go:139-154`:

```go
// Every emitter is inside work that matters more than its own instrumentation.
// A log that cannot be written loses the event and nothing else.
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

**Rewrite direction:** keep the `filepath.Join(t.TempDir(), "unwritable")` + `os.WriteFile(garden,
[]byte("i am a file, not a directory"), 0o644)` + `t.Setenv("HUGEL_HOME", garden)` setup verbatim
(this is the file-as-directory-component technique RESEARCH.md names as portable across
macOS/Linux and safe under root-run CI). Change the assertion from "no panic" to "returns a
non-nil, descriptive error" — capture `Emit`'s return value inside the goroutine (e.g. via a
`chan error` instead of `chan bool`) and assert both no-panic and `err != nil` with a message
substring check (e.g. `strings.Contains(err.Error(), "event")` or similar, matching the
`fmt.Errorf("create garden dir: %w", err)` wrapping style from the target `Emit` shape above).
Reuse this exact same `garden`/`t.Setenv("HUGEL_HOME", garden)` setup as the seam for the new
`gate_test.go`/`tender_test.go` sub-tests that must prove criterion 4 (event write fails, but
`Run`/`Start` still returns success) — those tests are integration-level and belong in their own
packages, but should copy this exact failure-injection technique rather than inventing a new one.

## Shared Patterns

### Report-and-continue on a failed side-write
**Source:** `internal/cli/soil.go:85-101` (`recordDraw`)
**Apply to:** All 10 `Emit` call sites across `internal/tender/start.go`, `internal/gate/run.go`
(2 closures), `internal/cli/dispatch.go`
```go
if err := events.Emit(events.Event{ /* ... */ }); err != nil {
	fmt.Fprintf(os.Stderr, "hugel: event not recorded: %v\n", err)
}
// fall through to the function's existing next statement — no early return, no error propagation
```

### Sticky first-failure marker via O_EXCL
**Source:** `internal/cli/dispatch.go:198-208` (`lockGarden`, for the `O_EXCL` idiom) combined
with `internal/cli/compost.go:256-270` (`compostMark`/`markComposted`, for the marker-file/mtime-
as-payload idiom, with the create-mode inverted from `O_WRONLY` to `O_CREATE|O_EXCL`)
**Apply to:** `internal/events/events.go`'s new `markFailing()`/`clearFailing()`/`failMarkPath()`

### Durable single-writer append (open/write/sync/close per call, no long-lived handle)
**Source:** `internal/draws/draws.go:47-66` (`Append`) for the shape; **no existing analog for
the `f.Sync()` line itself** — add it fresh per RESEARCH.md's "Recommended code shape," as a
checked (non-deferred) statement between `Write` and success return.
**Apply to:** `internal/events/events.go:Emit`

### Flag-selected report view on an existing multi-view CLI command
**Source:** `internal/cli/yield.go:37-49` (flag declarations) and `:85-96` (dispatch), plus
`:271-291` (`showSoil`, for the JSON/plain-text render split)
**Apply to:** `internal/cli/yield.go`'s new `--health` flag and `showHealth` function

### "Missing is OK" filesystem read
**Source:** `.planning/codebase/CONVENTIONS.md` Error Handling item 5, exemplified by
`internal/cli/compost.go:172-179`'s `if st, err := os.Stat(mark); err == nil { ... }` (no `else`
branch — a missing mark file is simply "no fact yet," not an error)
**Apply to:** `events.HealthOf()`'s reads of `events.jsonl`'s mtime and the failure marker's mtime

## No Analog Found

None. Every file/function in this phase's blast radius has a concrete, already-shipped analog in
this codebase — this is a small, internally-consistent refactor of an existing package, not new
surface area, so no file needed to fall back to RESEARCH.md's stdlib-only guidance without a
codebase precedent (the one true gap, `f.Sync()`, is called out explicitly above rather than
papered over with a false analog).

## Metadata

**Analog search scope:** `internal/events/`, `internal/draws/`, `internal/cli/` (compost.go,
soil.go, dispatch.go, yield.go, tender.go, gate.go), `internal/tender/start.go`,
`internal/gate/run.go`
**Files scanned:** 12 source files read this session (in addition to the 3 required-reading docs
and RESEARCH.md, which had already verified most of the same excerpts)
**Pattern extraction date:** 2026-09-08
