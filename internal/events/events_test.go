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

	Emit(Event{
		Name: "gate.stage", Bead: "hugel4-51v.1", Bed: "hugel4",
		Session: "sess-1", Outcome: "ok", Duration: 1500 * time.Millisecond,
		Fields: F{"stage": "retest", "test": "make test", "entries": []string{"a", "b"}},
	})

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
	Emit(Event{Name: "first"})
	p, _ := Path()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{not json at all\n")
	f.Close()
	Emit(Event{Name: "third"})

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "third" {
		t.Errorf("loaded %+v, want the two good events", got)
	}
}

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

// Dispatch runs tenders concurrently, so two emitters can meet. Halves of two
// events interleaved would corrupt both.
func TestConcurrentEmittersDoNotInterleave(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			Emit(Event{Name: "concurrent", Bead: "x", Fields: F{
				"i": i, "padding": strings.Repeat("wide events are wide ", 300),
			}})
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
	tm.Done("failed", F{"stage": "overridden", "added": "late"})

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

// A nil timer is what a caller holds when it decided not to measure something.
func TestNilTimerIsHarmless(t *testing.T) {
	var tm *Timer
	tm.Done("ok", nil)
}
