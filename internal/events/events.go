// Package events records what the garden did, one wide event per unit of work.
//
// The model is a canonical event rather than a narrative log: one line emitted
// at the boundary of a piece of work, carrying every field known at that
// moment, with no pre-aggregation. Counts are derived from events; events are
// never replaced by counts. The draw log already proves why — it stores the ids
// of the entries a draw delivered rather than how many, which is the only
// reason draw precision can be computed at all. Store the count and the
// measurement does not exist.
//
// High cardinality is the point, not a cost to manage. Bead, session, sha,
// model, entry ids: those are the fields that answer questions nobody thought
// to ask in advance, and they are exactly the ones a metrics system cannot
// hold.
//
// Hugel-shaped rather than OTLP-shaped, with names chosen so that translation
// stays mechanical if a day comes when these should be shipped somewhere:
// name maps to a span name, time to timeUnixNano, duration_ms to the span's
// extent, outcome to a status, bead to a trace id, and everything in Fields to
// attributes. Nothing here depends on that ever happening.
//
// There are no spans and no SDK. Causality in a garden does not cross machines,
// so the bead is the trace id and a flat line is the whole model.
//
// The log begins on 2026-09-01. Everything before that date was test exhaust:
// the gate and tender tests ran without HUGEL_HOME set and emitted into the
// gardener's own log, 495 events under the bead x-1 or under no bead at all,
// against which not one real event had ever been recorded. They were removed
// rather than kept, because a log that has to be filtered before every query is
// a log nobody queries, and a filter written once is a filter that outlives the
// reason for it. The removed lines are at ~/.hugel/events.jsonl.test-exhaust-2026-09-01.bak
// on the machine they came from; nothing reads them and nothing should.
package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charris/hugel/internal/config"
)

// now is the clock, replaced in tests.
var now = time.Now

// F is a bag of fields. Anything JSON can carry belongs in it: the whole point
// is that a field nobody planned for costs nothing to add.
type F map[string]any

// Event is one unit of work, as wide as what was known when it finished.
type Event struct {
	Name     string        `json:"name"`
	Time     time.Time     `json:"time"`
	Duration time.Duration `json:"-"`

	// Bead is the correlation key. A tender run, its soil draw, each gate
	// stage, the reviewer, the merge: all one bead, which is what a trace id
	// would have been for.
	Bead    string `json:"bead,omitempty"`
	Bed     string `json:"bed,omitempty"`
	Session string `json:"session,omitempty"`

	// Outcome says how it went in one word. Absent means the event records
	// something that happened rather than something that could fail.
	Outcome string `json:"outcome,omitempty"`

	Fields F `json:"-"`
}

// MarshalJSON writes the event flat: the core fields and every extra field at
// the same level, because a wide event is meant to be read with jq by a person
// and nesting attributes one level down doubles the work of every query.
//
// Core names win a collision. A caller that puts "bead" in Fields meant the
// bead, and quietly shadowing the real one would corrupt the correlation key
// that everything else joins on.
func (e Event) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range e.Fields {
		out[k] = v
	}
	out["name"] = e.Name
	out["time"] = e.Time.UTC().Format(time.RFC3339Nano)
	if e.Duration > 0 {
		out["duration_ms"] = e.Duration.Milliseconds()
	}
	for k, v := range map[string]string{
		"bead": e.Bead, "bed": e.Bed, "session": e.Session, "outcome": e.Outcome,
	} {
		if v != "" {
			out[k] = v
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads an event back, putting everything that is not a core
// field into Fields, so a round trip loses nothing.
func (e *Event) UnmarshalJSON(b []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	str := func(k string) string {
		if v, ok := raw[k].(string); ok {
			return v
		}
		return ""
	}
	e.Name, e.Bead, e.Bed = str("name"), str("bead"), str("bed")
	e.Session, e.Outcome = str("session"), str("outcome")
	if t, err := time.Parse(time.RFC3339Nano, str("time")); err == nil {
		e.Time = t
	}
	if ms, ok := raw["duration_ms"].(float64); ok {
		e.Duration = time.Duration(ms) * time.Millisecond
	}
	for _, k := range []string{"name", "time", "duration_ms", "bead", "bed", "session", "outcome"} {
		delete(raw, k)
	}
	if len(raw) > 0 {
		e.Fields = F(raw)
	}
	return nil
}

// Path is where events are recorded.
func Path() (string, error) {
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "events.jsonl"), nil
}

var mu sync.Mutex

// failMarkPath is the marker whose modification time is the whole payload: a
// zero-byte file at $HUGEL_HOME/events.failing-since, answering "since when"
// rather than "how many". It follows compostMark's idiom
// (internal/cli/compost.go) of a timestamp carried by mtime rather than by
// content — one syscall to read, and touching it is the entire write.
func failMarkPath() (string, error) {
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "events.failing-since"), nil
}

// markFailing records that a write just failed, but only if this is the first
// failure of the current streak.
//
// It takes nothing and returns nothing, because there is nothing a caller
// could do with its own failure: it runs from inside a write path that has
// already failed. It resolves the marker's path and gives up quietly if that
// fails -- the chicken-and-egg case HealthOf's doc comment covers, where a
// garden that cannot be written cannot record its own failure either.
//
// The create uses O_CREATE|O_EXCL, not the O_CREATE|O_WRONLY compostMark and
// markComposted use for a most-recent-write-wins success marker: this marker
// wants the opposite, first-write-wins. An exists error from O_EXCL (the same
// call lockGarden already uses, internal/cli/dispatch.go:202) means the
// streak is already marked and its first-failure time must be left alone; any
// other error means the garden cannot be written here either.
func markFailing() {
	p, err := failMarkPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Close()
}

// clearFailing ends a failing streak, unconditionally, on the next successful
// write.
//
// One os.Remove whose not-exist error is ignored: a streak that was never
// marked has nothing to clear, and stat-then-remove would be two syscalls to
// save one at ~5 events/day.
func clearFailing() {
	p, err := failMarkPath()
	if err != nil {
		return
	}
	_ = os.Remove(p)
}

// Emit records an event, and hands the failure back rather than absorbing it.
//
// It is the caller's job to report a returned error without letting it fail
// the work being instrumented — a tender mid-run, a gate about to merge, must
// not stop or retry because its own log could not be written. An instrument
// still must not break the thing it measures, but silence is no longer how
// that is achieved: only the caller knows whether its own work can tolerate a
// warning, so the package hands the failure up rather than deciding on the
// caller's behalf that it does not matter.
//
// It also syncs before it returns: the event has reached the file, not a
// buffer, by the time a caller sees nil, which is what makes silence in the
// log mean that nothing ran. On macOS Go's Sync issues fsync(2) rather than
// F_FULLFSYNC, so the drive's own write cache sits outside this guarantee
// (golang/go#26650) — Emit claims that its data left the process and the OS
// accepted it, and nothing stronger.
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
	// Only these four failure paths mark: a marshal failure above is a bad
	// event, not a broken log, and a Path() failure above means the garden's
	// location is unknown, so there is nowhere to put a marker. Marking on
	// either would make health report a filesystem problem that does not
	// exist.
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
	// One write per event: an append of a single line is what keeps concurrent
	// emitters from interleaving halves of each other's events.
	if _, err := f.Write(append(b, '\n')); err != nil {
		markFailing()
		return fmt.Errorf("write event: %w", err)
	}
	// Checked synchronously, not deferred: a deferred Sync could not influence
	// this return, and Emit would report success on data that never reached
	// the file. Close stays deferred — once Sync has succeeded the bytes are
	// durable, and a later close failure does not undo that.
	if err := f.Sync(); err != nil {
		markFailing()
		return fmt.Errorf("sync event log: %w", err)
	}
	// The first write that gets through ends the streak.
	clearFailing()
	return nil
}

// Timer measures a unit of work from here to Done.
type Timer struct {
	event Event
	start time.Time
}

// Start begins timing an event. The fields known at the start go in now; the
// ones known only at the end go in at Done.
func Start(name string, e Event) *Timer {
	e.Name = name
	return &Timer{event: e, start: now()}
}

// Done emits the event with its duration and how it went. Fields given here are
// merged over the ones given at Start.
//
// It returns whatever Emit returns. A caller must treat that error the way
// every emitter does: report it, do not let it fail the work being measured.
// A nil timer is what a caller holds when it decided not to measure
// something, so calling Done on it emits nothing and returns nil.
func (t *Timer) Done(outcome string, fields F) error {
	if t == nil {
		return nil
	}
	e := t.event
	e.Outcome = outcome
	e.Duration = now().Sub(t.start)
	if len(fields) > 0 {
		merged := F{}
		for k, v := range e.Fields {
			merged[k] = v
		}
		for k, v := range fields {
			merged[k] = v
		}
		e.Fields = merged
	}
	return Emit(e)
}

// Load reads every recorded event, oldest first. A missing log is no events,
// not an error: a garden that has done nothing yet is a normal garden.
func Load() ([]Event, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // one bad line costs one event, never the history
		}
		out = append(out, e)
	}
	return out, sc.Err()
}
