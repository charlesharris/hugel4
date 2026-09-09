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

// syncFile is the flush, replaced in tests. A real fsync is not observable
// from go test -- a power-loss test cannot be written and a failing fsync
// cannot be provoked portably -- so without a seam the sync is held by nothing
// but a grep over this file, and deleting the call leaves every test green.
// The seam does not test the platform's promise, which is not ours to test; it
// tests ours: that Emit flushes before it returns, and that a flush that fails
// is reported and marks the garden rather than passing for success.
var syncFile = (*os.File).Sync

// The two halves of the writability probe, replaced in tests. A filesystem
// with no room left cannot be built inside go test, and a permission-denied
// path can -- which is why the permission half is pinned by real chmod tests
// below and the free-space half needs a seam to be pinned at all. Without one,
// deleting the entire Statfs call leaves the suite green, which is the defect
// that produced syncFile one plan earlier and reappeared here.
var (
	permitsWrite    = platformPermitsWrite
	blocksAvailable = platformBlocksAvailable
)

// writable answers whether a write at path could succeed, without writing
// there. It is the question Emit will ask the kernel a moment later, asked
// early so health can tell "nothing has run" apart from "nothing could be
// recorded" -- two absences that look identical on disk.
//
// Permission is not the only way a write gets refused, and the other way is the
// one Success Criterion 1 names: a full disk. A filesystem that reports no
// available blocks is one whose next write may not land, and whose failure
// could not be marked either, since a marker needs an inode too.
//
// A filesystem that will not answer is not a filesystem in trouble. Permission
// already said yes, and manufacturing a failure from a missing second opinion
// would put "unknown" on healthy gardens.
func writable(path string) bool {
	if !permitsWrite(path) {
		return false
	}
	avail, err := blocksAvailable(path)
	if err != nil {
		return true
	}
	return avail > 0
}

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
	if err := syncFile(f); err != nil {
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

// Health answers the question a gardener cannot answer by eye: is the garden
// recording, and if not, since when. LastWrite and FailingSince are the two
// facts that keep "nothing has run since a date" separate from "writes have
// been failing since a date" -- together, not as one collapsed boolean.
type Health struct {
	// Healthy is true only when the garden was reachable and no failure
	// streak is open.
	Healthy bool `json:"healthy"`
	// Reachable says whether the garden directory itself could be read. A
	// field rather than an inference: under a wholly inaccessible garden
	// there is neither a log nor a marker, and reading those two absences as
	// "healthy, nothing has run" would be the exact wrong answer to the
	// question this type exists to answer. A log that cannot be written
	// cannot record its own failure, so health has to be able to say that it
	// does not know.
	Reachable bool `json:"reachable"`
	// Home is the garden directory the answer was read from.
	Home string `json:"home"`
	// LastWrite is when the log was last successfully appended to, nil when
	// it has never been written -- a fact absent rather than a timestamp in
	// the year 1.
	LastWrite *time.Time `json:"last_write,omitempty"`
	// FailingSince is the first failure of the current streak, nil when none
	// is open.
	FailingSince *time.Time `json:"failing_since,omitempty"`
}

// HealthOf reads the garden's health without writing anything.
//
// It returns an error only when the garden's location cannot be resolved at
// all -- a garden whose location is unknown has no state to report. An
// unreachable garden is not that: it is an answer, and hugel yield --health
// must be able to print it and exit 0.
func HealthOf() (Health, error) {
	p, err := Path()
	if err != nil {
		return Health{}, err
	}
	home := filepath.Dir(p)
	h := Health{Home: home}

	// A not-exist error means a garden that has not been created yet -- fine,
	// and still reachable. Any other error, or a successful stat of something
	// that is not a directory, means it cannot be read.
	st, err := os.Stat(home)
	switch {
	case os.IsNotExist(err):
		h.Reachable = true
	case err != nil:
		h.Reachable = false
	case !st.IsDir():
		h.Reachable = false
	default:
		h.Reachable = true
	}
	if !h.Reachable {
		return h, nil
	}

	if st, err := os.Stat(p); err == nil {
		if !st.Mode().IsRegular() {
			// Something that is not a file stands where the log belongs: a
			// directory, a socket, a device. Emit cannot have written it and
			// cannot write it now, so its mtime is the intruder's and not a
			// write's, and trusting it reports a healthy log for a garden
			// that has never recorded one event -- the exact failure this
			// whole surface exists to make visible, stated backwards. Demote
			// rather than guess, the same way the home stat above does when
			// something that is not a directory stands there.
			h.Reachable = false
			return h, nil
		}
		t := st.ModTime()
		h.LastWrite = &t
	} else if !os.IsNotExist(err) {
		// A permission-denied garden looks like this: reachable enough to
		// stat the directory, but its contents cannot be read. Demote the
		// whole answer to unreachable rather than reporting a partial one.
		h.Reachable = false
		return h, nil
	}

	if mark, err := failMarkPath(); err == nil {
		if st, err := os.Stat(mark); err == nil {
			if !st.Mode().IsRegular() {
				// markFailing only ever creates a regular file, by an
				// exclusive create, so anything else standing here was not
				// written by us and its mtime dates nothing. Reading it would
				// put a fabricated start on a streak, or -- worse -- ignoring
				// it would call the garden healthy on the strength of a
				// marker we cannot read. Refuse the answer instead.
				h.Reachable = false
				return h, nil
			}
			t := st.ModTime()
			h.FailingSince = &t
		}
	}

	// Only now, and only if the garden did not already answer. A marker on
	// disk IS the answer -- it carries the date SUB-03 asks for -- and
	// replacing that with "unknown" would throw away the good sentence while
	// holding it. The probe exists for the other case: no marker, which means
	// either no failure happened or none could be written down, and those two
	// are indistinguishable from the outside of a garden that refuses writes.
	if h.FailingSince == nil && !writable(writeTarget(home, p, h.LastWrite != nil)) {
		h.Reachable = false
		return h, nil
	}

	h.Healthy = h.Reachable && h.FailingSince == nil
	return h, nil
}

// writeTarget is the path Emit's next write would really need permission on.
//
// The log itself once it exists, because an append asks nothing of the
// directory holding it. Otherwise the nearest ancestor that exists, because
// Emit creates the garden with MkdirAll, and MkdirAll needs the first
// directory already on disk to be writable -- everything below it is its own
// to make. A garden that has not been created yet is not an unwritable one;
// that is simply a fresh install's first run, and calling it unknown would
// make hugel's first answer to a new gardener a shrug.
func writeTarget(home, log string, logExists bool) string {
	if logExists {
		return log
	}
	for dir := home; ; {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
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
