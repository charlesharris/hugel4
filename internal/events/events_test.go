package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func at(t *testing.T, when time.Time) {
	t.Helper()
	old := now
	now = func() time.Time { return when }
	t.Cleanup(func() { now = old })
}

func TestEmitAndLoadRoundTrip(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	when := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	at(t, when)

	if got, err := Load(); err != nil || got != nil {
		t.Fatalf("Load on a fresh garden = %v, %v; want nothing and no error", got, err)
	}

	if err := Emit(Event{
		Name: "gate.stage", Bead: "hugel4-51v.1", Bed: "hugel4",
		Session: "sess-1", Outcome: "ok", Duration: 1500 * time.Millisecond,
		Fields: F{"stage": "retest", "test": "make test", "entries": []string{"a", "b"}},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("loaded %d events, want 1", len(got))
	}
	e := got[0]
	if e.Name != "gate.stage" || e.Bead != "hugel4-51v.1" || e.Outcome != "ok" {
		t.Errorf("core fields lost: %+v", e)
	}
	if !e.Time.Equal(when) {
		t.Errorf("time = %v, want %v", e.Time, when)
	}
	if e.Duration != 1500*time.Millisecond {
		t.Errorf("duration = %v, want 1.5s", e.Duration)
	}
	if e.Fields["stage"] != "retest" || e.Fields["test"] != "make test" {
		t.Errorf("extra fields lost: %+v", e.Fields)
	}
	// High cardinality is the point: an array of ids has to survive, because
	// that is the shape that makes a measurement possible at all.
	ids, ok := e.Fields["entries"].([]any)
	if !ok || len(ids) != 2 {
		t.Errorf("entries = %#v, want the two ids", e.Fields["entries"])
	}
}

// Flat, so a person can read it with jq without descending into an attributes
// object on every query.
func TestEventsAreWrittenFlat(t *testing.T) {
	b, err := json.Marshal(Event{
		Name: "tender.start", Bead: "x-1", Fields: F{"branch": "hugel/x-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if _, nested := raw["fields"]; nested {
		t.Error("fields were nested rather than flattened")
	}
	if raw["branch"] != "hugel/x-1" {
		t.Errorf("extra field not at the top level: %v", raw)
	}
	if raw["name"] != "tender.start" || raw["bead"] != "x-1" {
		t.Errorf("core fields missing: %v", raw)
	}
}

// A caller that puts "bead" in Fields meant the bead. Shadowing the real one
// would corrupt the key everything else joins on.
func TestCoreFieldsWinACollision(t *testing.T) {
	b, _ := json.Marshal(Event{
		Name: "x", Bead: "the-real-one", Fields: F{"bead": "an-impostor", "name": "also-wrong"},
	})
	var raw map[string]any
	json.Unmarshal(b, &raw)
	if raw["bead"] != "the-real-one" {
		t.Errorf("bead = %v, want the core field to win", raw["bead"])
	}
	if raw["name"] != "x" {
		t.Errorf("name = %v, want the core field to win", raw["name"])
	}
}

// Empty core fields are omitted rather than written as empty strings, so a line
// says what it knows and nothing else.
func TestEmptyCoreFieldsAreOmitted(t *testing.T) {
	b, _ := json.Marshal(Event{Name: "x"})
	s := string(b)
	for _, absent := range []string{`"bead"`, `"bed"`, `"session"`, `"outcome"`, `"duration_ms"`} {
		if strings.Contains(s, absent) {
			t.Errorf("%s was written when it was not known: %s", absent, s)
		}
	}
}

// A garden running for months will eventually have a truncated or half-written
// line. One bad line must cost one event, never the history.
func TestCorruptLineCostsOneEvent(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	if err := Emit(Event{Name: "first"}); err != nil {
		t.Fatal(err)
	}
	p, _ := Path()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{not json at all\n")
	f.Close()
	if err := Emit(Event{Name: "third"}); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "third" {
		t.Errorf("loaded %+v, want the two good events", got)
	}
}

// Every emitter is inside work that matters more than its own instrumentation.
// A log that cannot be written must not panic the caller -- but a silently
// broken write can no longer pass as though nothing happened either.
func TestEmitReturnsAnErrorWhenTheLogCannotBeWritten(t *testing.T) {
	garden := filepath.Join(t.TempDir(), "unwritable")
	if err := os.WriteFile(garden, []byte("i am a file, not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUGEL_HOME", garden)

	errs := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Emit panicked when the log could not be written: %v", r)
				errs <- nil
			}
		}()
		errs <- Emit(Event{Name: "into the void", Bead: "x-1"})
	}()
	if err := <-errs; err == nil || !strings.Contains(err.Error(), "create garden dir") {
		t.Fatalf("Emit err = %v, want an error mentioning %q", err, "create garden dir")
	}
}

// This proves the narrower, testable claim: nothing of ours is holding the
// event in a buffer between Emit returning and a fresh reader looking for it.
// The fsync itself is a platform promise a power-loss test cannot observe from
// go test on either macOS or Linux, and a failing fsync cannot be provoked
// portably either -- the presence and checking of the Sync call are held by
// the source assertions in this test's plan task instead of by this test.
func TestEmitIsDurableBeforeItReturns(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())

	if err := Emit(Event{Name: "durable-before-return", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}

	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	// A fresh handle that shares nothing with the one Emit used: os.ReadFile
	// opens, reads and closes its own file descriptor.
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Event.MarshalJSON builds a map[string]any, and encoding/json sorts map
	// keys, so the quoted key is immediately followed by its quoted value with
	// no space between them.
	if !strings.Contains(string(b), `"name":"durable-before-return"`) {
		t.Fatalf("log after Emit returned = %q, want it to already contain the event's name", b)
	}
}

// Dispatch runs tenders concurrently, so two emitters can meet. Halves of two
// events interleaved would corrupt both.
func TestConcurrentEmittersDoNotInterleave(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := Emit(Event{Name: "concurrent", Bead: "x", Fields: F{
				"i": i, "padding": strings.Repeat("wide events are wide ", 300),
			}}); err != nil {
				t.Errorf("Emit: %v", err)
			}
		}(i)
	}
	wg.Wait()

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 50 {
		t.Errorf("loaded %d events, want 50 -- lines were interleaved or lost", len(got))
	}
}

func TestTimerMeasuresAndMerges(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	start := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	at(t, start)

	tm := Start("gate.stage", Event{Bead: "x-1", Fields: F{"stage": "retest", "keep": "me"}})
	at(t, start.Add(3*time.Second))
	if err := tm.Done("failed", F{"stage": "overridden", "added": "late"}); err != nil {
		t.Fatal(err)
	}

	got, _ := Load()
	if len(got) != 1 {
		t.Fatalf("loaded %d events", len(got))
	}
	e := got[0]
	if e.Name != "gate.stage" || e.Outcome != "failed" {
		t.Errorf("event = %+v", e)
	}
	if e.Duration != 3*time.Second {
		t.Errorf("duration = %v, want 3s", e.Duration)
	}
	if e.Fields["stage"] != "overridden" {
		t.Errorf("late fields did not win: %v", e.Fields["stage"])
	}
	if e.Fields["keep"] != "me" || e.Fields["added"] != "late" {
		t.Errorf("fields lost in the merge: %v", e.Fields)
	}
}

// A nil timer is what a caller holds when it decided not to measure
// something. "Harmless" now means Done on it emits nothing and returns nil.
func TestNilTimerIsHarmless(t *testing.T) {
	var tm *Timer
	if err := tm.Done("ok", nil); err != nil {
		t.Errorf("Done on a nil timer = %v, want nil", err)
	}
}

// A write that fails while the garden is writable leaves a marker beside the
// log, dated to the failure.
func TestAFailedWriteMarksTheGardenAsFailing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	// Standing in for the log with a directory makes every open of it fail
	// with "is a directory", while leaving the garden itself writable -- the
	// partial-failure seam, as opposed to an unreachable garden.
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Emit(Event{Name: "into the log"}); err == nil {
		t.Fatal("Emit = nil error, want one: the log path is a directory")
	}

	mark, err := failMarkPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mark); err != nil {
		t.Errorf("failure marker missing after a failed write: %v", err)
	}
}

// Without the exclusive create, every failure would re-stamp the marker and
// health would forever report "failing since just now" -- the one fact the
// marker exists to carry.
func TestTheFailureMarkKeepsTheFirstFailuresTime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Emit(Event{Name: "first failure"}); err == nil {
		t.Fatal("Emit = nil error, want one")
	}
	mark, err := failMarkPath()
	if err != nil {
		t.Fatal(err)
	}
	backdated := time.Now().Add(-time.Hour)
	if err := os.Chtimes(mark, backdated, backdated); err != nil {
		t.Fatalf("could not backdate the marker (was it created?): %v", err)
	}

	if err := Emit(Event{Name: "second failure, an hour later"}); err == nil {
		t.Fatal("Emit = nil error, want one")
	}

	st, err := os.Stat(mark)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().Equal(backdated) {
		t.Errorf("marker mtime = %v, want it pinned at the backdated %v", st.ModTime(), backdated)
	}
}

// The first write that gets through ends the streak.
func TestASuccessfulWriteClearsTheFailureMark(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Emit(Event{Name: "failing"}); err == nil {
		t.Fatal("Emit = nil error, want one")
	}
	mark, err := failMarkPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mark); err != nil {
		t.Fatalf("marker missing before the fix, can't test that it clears: %v", err)
	}
	// Stand the log path back up as an ordinary file so writes can succeed.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	if err := Emit(Event{Name: "succeeding"}); err != nil {
		t.Fatalf("Emit = %v, want nil now that the log path is writable", err)
	}

	if _, err := os.Stat(mark); !os.IsNotExist(err) {
		t.Errorf("marker stat err = %v, want IsNotExist -- the successful write should have cleared it", err)
	}
}

// The chicken-and-egg case: a log that cannot be written cannot record its
// own failure either. The stderr notice at the time of the command is the
// only channel that works here; the marker is a best-effort retrospective
// signal, not a substitute for it.
func TestAnUnreachableGardenCannotBeMarked(t *testing.T) {
	garden := filepath.Join(t.TempDir(), "unwritable")
	if err := os.WriteFile(garden, []byte("i am a file, not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUGEL_HOME", garden)

	if err := Emit(Event{Name: "into the void"}); err == nil {
		t.Fatal("Emit = nil error, want one")
	}

	mark, err := failMarkPath()
	if err != nil {
		t.Fatal(err)
	}
	// The garden itself is unreachable, so os.Stat on any path beneath it
	// fails with "not a directory" rather than "not exist" -- what matters is
	// that no marker was written, not the errno's exact shape.
	if _, err := os.Stat(mark); err == nil {
		t.Error("stat on the failure marker succeeded -- an unreachable garden has nowhere to put one")
	}
}

// Sandbox's own guard against resolving the gardener's real garden inside a
// test. Every other error an emitter meets is now returned and reported by
// its caller, but a test that resolved the real garden must not be allowed to
// continue: a returned error would be too easy to ignore inside a test binary,
// so this one refusal stays a panic instead of a value a caller could drop.
func TestEmittingWithoutATemporaryGardenFailsTheTest(t *testing.T) {
	t.Setenv("HUGEL_HOME", "") // the omission that leaked

	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		_ = Emit(Event{Name: "gate.stage", Bead: "x-1"})
	}()
	if r := <-done; r == nil {
		t.Fatal("Emit wrote to the gardener's real events log instead of failing")
	}
}

// A garden with a written log and no marker: healthy, reachable, LastWrite
// set, FailingSince nil.
func TestHealthOfReportsAWrittenLogAsHealthy(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	before := time.Now()
	if err := Emit(Event{Name: "gate.stage"}); err != nil {
		t.Fatal(err)
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if !h.Healthy || !h.Reachable {
		t.Errorf("Healthy = %v, Reachable = %v, want both true", h.Healthy, h.Reachable)
	}
	if h.LastWrite == nil || h.LastWrite.Before(before) {
		t.Errorf("LastWrite = %v, want non-nil and recent (after %v)", h.LastWrite, before)
	}
	if h.FailingSince != nil {
		t.Errorf("FailingSince = %v, want nil", h.FailingSince)
	}
}

// A garden that exists but has never been written: healthy, reachable, both
// times nil -- "nothing has run", not "something is wrong".
func TestHealthOfSaysNothingHasRunInAFreshGarden(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if !h.Healthy || !h.Reachable {
		t.Errorf("Healthy = %v, Reachable = %v, want both true", h.Healthy, h.Reachable)
	}
	if h.LastWrite != nil {
		t.Errorf("LastWrite = %v, want nil in a fresh garden", h.LastWrite)
	}
	if h.FailingSince != nil {
		t.Errorf("FailingSince = %v, want nil in a fresh garden", h.FailingSince)
	}
}

// A garden with a marker: not healthy, FailingSince set to the marker's
// modification time. The marker's creation is task 1's business -- this test
// constructs the state directly rather than provoking it through Emit.
func TestHealthOfReportsAFailingStreak(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	mark, err := failMarkPath()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(mark, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	since := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(mark, since, since); err != nil {
		t.Fatal(err)
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if h.Healthy {
		t.Error("Healthy = true, want false with an open failure streak")
	}
	if h.FailingSince == nil || !h.FailingSince.Equal(since) {
		t.Errorf("FailingSince = %v, want %v", h.FailingSince, since)
	}
}

// A garden that cannot be read: not reachable, not healthy, both times nil,
// Home naming the path that could not be read -- and no error returned,
// because this is an answer the command must be able to print.
func TestHealthOfRefusesToGuessWhenTheGardenCannotBeRead(t *testing.T) {
	garden := filepath.Join(t.TempDir(), "unwritable")
	if err := os.WriteFile(garden, []byte("i am a file, not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUGEL_HOME", garden)

	h, err := HealthOf()
	if err != nil {
		t.Fatalf("HealthOf err = %v, want nil -- an unreachable garden is an answer, not a failure", err)
	}
	if h.Reachable {
		t.Error("Reachable = true, want false")
	}
	if h.Healthy {
		t.Error("Healthy = true, want false")
	}
	if h.LastWrite != nil || h.FailingSince != nil {
		t.Errorf("LastWrite = %v, FailingSince = %v, want both nil", h.LastWrite, h.FailingSince)
	}
	if h.Home != garden {
		t.Errorf("Home = %q, want %q", h.Home, garden)
	}
}
