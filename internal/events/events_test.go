package events

import (
	"encoding/json"
	"errors"
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
// go test on either macOS or Linux. It is a weaker claim than it looks: a
// write(2) has already put the bytes in the page cache, so a fresh reader on
// this machine sees them whether or not anything was flushed, and this test
// passes with the Sync deleted. What holds the flush is the syncFile seam and
// the two tests below it.
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

// The seam earns itself here: without it, deleting Emit's flush leaves the
// whole package green, because a page-cache read cannot tell a synced write
// from an unsynced one. This asks the only question that is ours to ask --
// did Emit flush the handle it wrote, before it returned nil.
func TestEmitFlushesBeforeItReturns(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())

	var synced int
	restore := syncFile
	syncFile = func(f *os.File) error {
		synced++
		return restore(f)
	}
	t.Cleanup(func() { syncFile = restore })

	if err := Emit(Event{Name: "flushed-before-return", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}

	if synced != 1 {
		t.Errorf("flushes during one Emit = %d, want 1", synced)
	}
}

// A flush that fails is the case the log exists to survive: the bytes reached
// the OS but not the disk, and reporting nil there would make silence in the
// log mean the wrong thing. It cannot be provoked by any filesystem the test
// can build, so the seam provokes it directly.
func TestAFailedFlushIsReportedAndMarksTheGarden(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())

	restore := syncFile
	syncFile = func(*os.File) error { return errors.New("no space left on device") }
	t.Cleanup(func() { syncFile = restore })

	err := Emit(Event{Name: "flush-fails", Bead: "x-1"})
	if err == nil {
		t.Fatal("Emit = nil error, want one: the flush failed")
	}
	if !strings.Contains(err.Error(), "sync event log") {
		t.Errorf("Emit err = %v, want it to name %q", err, "sync event log")
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		t.Errorf("Emit err = %v, want it to carry the underlying flush failure", err)
	}

	// A write that reached the OS but not the disk is a failing garden, not a
	// healthy one: the marker is what lets health report the streak later.
	mark, err := failMarkPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mark); err != nil {
		t.Errorf("failure marker missing after a failed flush: %v", err)
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
// The failure this surface exists to catch, in the one filesystem state that
// used to defeat it: a log path occupied by a directory has never been written
// and cannot be written, and Emit says so plainly, so health must not answer
// "healthy, last written just now" off the directory's own mtime.
func TestHealthOfWillNotCallADirectoryAWrittenLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately before any Emit: nothing has ever been written here, and
	// no failure marker exists either, so the two absences that would
	// otherwise add up to "healthy" are both in place.
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if h.Healthy {
		t.Error("Health.Healthy = true, want false: the log path is a directory and has never been written")
	}
	if h.Reachable {
		t.Error("Health.Reachable = true, want false: nothing can read or write a log that is a directory")
	}
	if h.LastWrite != nil {
		t.Errorf("Health.LastWrite = %v, want nil: that timestamp is the directory's, not a write's", h.LastWrite)
	}
}

// The same shape, one path over. markFailing writes the marker by an exclusive
// create and nothing else ever writes it, so a non-regular file standing there
// dates nothing -- and a garden whose failure marker cannot be read must not
// come back healthy on the strength of it.
func TestHealthOfWillNotTakeAFailureDateFromSomethingItDidNotWrite(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())

	if err := Emit(Event{Name: "a real write", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}
	mark, err := failMarkPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mark, 0o755); err != nil {
		t.Fatal(err)
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if h.Healthy {
		t.Error("Health.Healthy = true, want false: the failure marker cannot be read")
	}
	if h.FailingSince != nil {
		t.Errorf("Health.FailingSince = %v, want nil: that timestamp is the directory's, not a failure's", h.FailingSince)
	}
}

// Root ignores the permission bits these tests turn off, so the states they
// build do not exist for it.
func notRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not apply")
	}
}

// The two absences that used to add up to a confident lie. Nothing has been
// written, and nothing CAN be written, so markFailing could not leave a marker
// either -- and health used to read the missing marker as "no failures" and
// answer "healthy, nothing has run yet" for a garden refusing every write.
// That is SC-3's own worked example answered with the wrong one of its two
// readings.
func TestHealthOfWillNotCallAnUnwritableGardenQuiet(t *testing.T) {
	notRoot(t)
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	if err := os.Chmod(home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(home, 0o700) })

	// The write really does fail here: that is what makes "healthy" a lie
	// rather than a harmless approximation.
	if err := Emit(Event{Name: "refused", Bead: "x-1"}); err == nil {
		t.Fatal("Emit = nil error, want one: the garden is not writable")
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if h.Healthy {
		t.Error("Health.Healthy = true, want false: every write to this garden is being refused")
	}
	if h.Reachable {
		t.Error("Health.Reachable = true, want false: health cannot tell 'nothing has run' from 'nothing could be recorded' here")
	}
}

// The same lie one state over, and the one the log's own mtime makes worse: a
// log that exists but cannot be appended to, in a garden that cannot take a
// marker either. Health used to report healthy and quote the last successful
// write as though it were recent news.
func TestHealthOfWillNotCallAnUnwritableLogHealthy(t *testing.T) {
	notRoot(t)
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	if err := Emit(Event{Name: "the last one that landed", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(home, 0o700); os.Chmod(p, 0o600) })

	if err := Emit(Event{Name: "refused", Bead: "x-1"}); err == nil {
		t.Fatal("Emit = nil error, want one: the log is not writable")
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if h.Healthy {
		t.Error("Health.Healthy = true, want false: the log cannot be appended to and the failure cannot be marked")
	}
}

// The guard against over-correcting. A read-only garden DIRECTORY does not
// stop an append to a log already inside it -- the directory's bits govern
// creating and removing entries, not writing through to a file that is already
// there -- so Emit still succeeds and healthy is the true answer. Demoting
// here would trade a false alarm for the false silence just fixed, and the
// probe deliberately asks about the log rather than the directory once the log
// exists for exactly this reason.
func TestHealthOfStillTrustsAWritableLogInAReadOnlyGarden(t *testing.T) {
	notRoot(t)
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	if err := Emit(Event{Name: "first", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(home, 0o700) })

	if err := Emit(Event{Name: "still lands", Bead: "x-1"}); err != nil {
		t.Fatalf("Emit = %v, want nil: an append needs nothing from the directory", err)
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if !h.Healthy {
		t.Error("Health.Healthy = false, want true: writes are landing, so this garden is healthy")
	}
	if h.LastWrite == nil {
		t.Error("Health.LastWrite = nil, want the time of the write that just succeeded")
	}
}

// A garden that does not exist yet is not a garden that refuses writes: Emit
// makes it with MkdirAll on first use. The writability probe once read the
// missing directory as unwritable and answered "unknown", which made hugel's
// very first answer to a new gardener a shrug where it used to be "nothing has
// run yet". Regression guard.
func TestHealthOfCallsAnUncreatedGardenFreshRatherThanUnknown(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not", "created", "yet")
	t.Setenv("HUGEL_HOME", home)

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if !h.Reachable {
		t.Error("Health.Reachable = false, want true: this garden can be created, it just has not been")
	}
	if !h.Healthy {
		t.Error("Health.Healthy = false, want true: nothing has run, and nothing has failed either")
	}
	if h.LastWrite != nil {
		t.Errorf("Health.LastWrite = %v, want nil", h.LastWrite)
	}
	// The claim above is only worth anything if a write really does land.
	if err := Emit(Event{Name: "first ever", Bead: "x-1"}); err != nil {
		t.Fatalf("Emit = %v, want nil: the garden should have been created", err)
	}
}

// A marker on disk IS the answer -- it carries the date SUB-03 asks for. The
// probe once ran ahead of the marker stat and replaced "FAILING since <date>,
// last write <date>" with "unknown", throwing away the exact sentence this
// requirement exists to produce while holding it. Regression guard.
func TestHealthOfKeepsADatedFailureRatherThanAnsweringUnknown(t *testing.T) {
	notRoot(t)
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)
	if err := Emit(Event{Name: "the last one that landed", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	// The log refuses writes, but the garden around it does not, so markFailing
	// can still record the streak -- and having recorded it, health must say so.
	if err := os.Chmod(p, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(p, 0o600) })
	if err := Emit(Event{Name: "refused", Bead: "x-1"}); err == nil {
		t.Fatal("Emit = nil error, want one: the log is not writable")
	}

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if h.FailingSince == nil {
		t.Error("Health.FailingSince = nil, want the streak's start: the marker is on disk and dated")
	}
	if !h.Reachable {
		t.Error("Health.Reachable = false, want true: the garden answered, so there is nothing to be unknown about")
	}
	if h.Healthy {
		t.Error("Health.Healthy = true, want false: a streak is open")
	}
	if h.LastWrite == nil {
		t.Error("Health.LastWrite = nil, want the last write that landed")
	}
}

// A full filesystem is the state Success Criterion 1 names and the one no test
// can build: go test cannot fill a disk, and mounting an image needs privileges
// and a platform. The seam is what makes the check pinnable at all -- without
// it the entire Statfs call could be deleted and every test here would still
// pass, which is exactly how the fsync check shipped hollow one plan earlier.
func TestHealthOfWillNotCallAFullFilesystemHealthy(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	if err := Emit(Event{Name: "landed while there was room", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}

	restore := blocksAvailable
	blocksAvailable = func(string) (uint64, error) { return 0, nil }
	t.Cleanup(func() { blocksAvailable = restore })

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if h.Healthy {
		t.Error("Health.Healthy = true, want false: the filesystem has no blocks left to give")
	}
	if h.Reachable {
		t.Error("Health.Reachable = true, want false: a write that cannot be promised cannot be called healthy")
	}
}

// The other half of that contract, and the easier one to get wrong: a
// filesystem that declines to answer is not a filesystem in trouble. Reading a
// failed Statfs as bad news would put "unknown" on healthy gardens, which is
// the over-demotion this package keeps having to avoid.
func TestAFilesystemThatWillNotAnswerIsNotABadOne(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	if err := Emit(Event{Name: "landed", Bead: "x-1"}); err != nil {
		t.Fatal(err)
	}

	restore := blocksAvailable
	blocksAvailable = func(string) (uint64, error) { return 0, errors.New("statfs: function not implemented") }
	t.Cleanup(func() { blocksAvailable = restore })

	h, err := HealthOf()
	if err != nil {
		t.Fatal(err)
	}
	if !h.Healthy {
		t.Error("Health.Healthy = false, want true: permission said yes and nothing contradicted it")
	}
}

// The seam is only worth having if the thing behind it is real. This is what
// catches a Statfs call replaced by a constant zero or a bare error -- either
// would demote every garden on the machine.
//
// It does NOT catch a mutation returning a hardcoded large number, and saying
// so is the honest limit: the Bavail > 0 predicate against a genuinely full
// filesystem is checked by hand, on a mounted image, not here.
func TestTheRealFreeSpaceProbeAnswersForAWritableDirectory(t *testing.T) {
	dir := t.TempDir()

	avail, err := platformBlocksAvailable(dir)
	if err != nil {
		t.Fatalf("platformBlocksAvailable(%q) = %v, want a working filesystem under a temp dir", dir, err)
	}
	if avail == 0 {
		t.Errorf("platformBlocksAvailable(%q) = 0, want blocks free: a temp dir this test just wrote to is not full", dir)
	}
}

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
