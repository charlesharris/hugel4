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
package events

import (
	"bufio"
	"encoding/json"
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
func (t *Timer) Done(outcome string, fields F) {
	if t == nil {
		return
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
	Emit(e)
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
