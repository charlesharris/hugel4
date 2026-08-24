package yield

import (
	"testing"
	"time"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/transcript"
)

var base = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func span(id, bed string, startMin, endMin int) *transcript.Session {
	return &transcript.Session{
		ID: id, Bed: bed,
		Start: base.Add(time.Duration(startMin) * time.Minute),
		End:   base.Add(time.Duration(endMin) * time.Minute),
	}
}

func draw(bed string, atMin int, ids ...string) draws.Draw {
	return draws.Draw{
		At:  base.Add(time.Duration(atMin) * time.Minute),
		Bed: bed, Tokens: 100, Entries: ids,
	}
}

func judged(id string, r pile.Review) *pile.Entry {
	return &pile.Entry{ID: id, Review: r}
}

// Reach is the number the skill shipped without. A session counts as reached
// only if a draw happened inside it, in its own bed -- another project's draw
// says nothing about whether this one asked.
func TestReachCountsSessionsNotDraws(t *testing.T) {
	sessions := []*transcript.Session{
		span("a", "hugel4", 0, 60),
		span("b", "hugel4", 120, 180),
		span("c", "other", 0, 60),
	}
	log := []draws.Draw{
		draw("hugel4", 10),
		draw("hugel4", 20), // same session drawing twice must not reach twice
		draw("other", 300), // outside any session's span
	}

	rep := Soil(sessions, log, nil, Filter{})
	if rep.Sessions != 3 {
		t.Errorf("sessions = %d, want 3", rep.Sessions)
	}
	if rep.Draws != 3 {
		t.Errorf("draws = %d, want 3", rep.Draws)
	}
	if rep.Reached != 1 {
		t.Errorf("reached = %d, want 1 (session a only)", rep.Reached)
	}
	if got := rep.Reach(); got < 0.32 || got > 0.34 {
		t.Errorf("reach = %.2f, want ~0.33", got)
	}
}

// Precision counts only entries a human has ruled on. If unreviewed entries
// counted as good, the number would be highest exactly when nobody is looking,
// which is the opposite of what it is for.
func TestPrecisionIgnoresTheUnjudged(t *testing.T) {
	sessions := []*transcript.Session{span("a", "hugel4", 0, 60)}
	log := []draws.Draw{draw("hugel4", 10, "keep1", "keep2", "toss1", "dunno")}
	entries := []*pile.Entry{
		judged("keep1", pile.Accepted),
		judged("keep2", pile.Accepted),
		judged("toss1", pile.Rejected),
		judged("dunno", pile.Unreviewed),
	}

	rep := Soil(sessions, log, entries, Filter{})
	if rep.Delivered != 4 || rep.Distinct != 4 {
		t.Errorf("delivered/distinct = %d/%d, want 4/4", rep.Delivered, rep.Distinct)
	}
	if rep.Judged() != 3 {
		t.Errorf("judged = %d, want 3", rep.Judged())
	}
	if rep.Unreviewed != 1 {
		t.Errorf("unreviewed = %d, want 1", rep.Unreviewed)
	}
	if got := rep.Precision(); got < 0.66 || got > 0.67 {
		t.Errorf("precision = %.2f, want ~0.67", got)
	}
}

// An entry delivered twice is one entry's worth of evidence about quality, but
// two entries' worth of tokens paid. Both numbers are worth having.
func TestRepeatDeliveriesCountOnceForQuality(t *testing.T) {
	sessions := []*transcript.Session{span("a", "hugel4", 0, 60)}
	log := []draws.Draw{
		draw("hugel4", 10, "same", "other"),
		draw("hugel4", 20, "same"),
	}
	entries := []*pile.Entry{judged("same", pile.Accepted), judged("other", pile.Rejected)}

	rep := Soil(sessions, log, entries, Filter{})
	if rep.Delivered != 3 {
		t.Errorf("delivered = %d, want 3", rep.Delivered)
	}
	if rep.Distinct != 2 {
		t.Errorf("distinct = %d, want 2", rep.Distinct)
	}
	if rep.Accepted != 1 || rep.Rejected != 1 {
		t.Errorf("accepted/rejected = %d/%d, want 1/1", rep.Accepted, rep.Rejected)
	}
}

// An entry can be drawn and later vanish -- retitled by a recompost, or filed
// under a different bed. That is not a rejection and must not read as one.
func TestVanishedEntriesAreNotRejections(t *testing.T) {
	sessions := []*transcript.Session{span("a", "hugel4", 0, 60)}
	log := []draws.Draw{draw("hugel4", 10, "gone")}

	rep := Soil(sessions, log, nil, Filter{})
	if rep.Missing != 1 {
		t.Errorf("missing = %d, want 1", rep.Missing)
	}
	if rep.Rejected != 0 || rep.Judged() != 0 {
		t.Errorf("a vanished entry was judged: rejected=%d judged=%d", rep.Rejected, rep.Judged())
	}
}

func TestSoilRespectsTheWindow(t *testing.T) {
	sessions := []*transcript.Session{
		span("old", "hugel4", -600, -540),
		span("new", "hugel4", 0, 60),
	}
	log := []draws.Draw{draw("hugel4", -580, "x"), draw("hugel4", 10, "y")}

	rep := Soil(sessions, log, nil, Filter{Since: base.Add(-time.Hour)})
	if rep.Sessions != 1 || rep.Draws != 1 {
		t.Errorf("sessions/draws = %d/%d, want 1/1", rep.Sessions, rep.Draws)
	}
	if rep.Reached != 1 {
		t.Errorf("reached = %d, want 1", rep.Reached)
	}
}

// With nothing drawn, precision must report as unknown rather than as zero or
// as perfect. A rate with no denominator is not a measurement.
func TestNoDrawsIsUnknownNotZero(t *testing.T) {
	rep := Soil([]*transcript.Session{span("a", "hugel4", 0, 60)}, nil, nil, Filter{})
	if rep.Draws != 0 || rep.Reached != 0 || rep.Judged() != 0 {
		t.Errorf("unexpected counts on an empty log: %+v", rep)
	}
	if rep.Precision() != 0 || rep.Reach() != 0 {
		t.Error("rates should be zero-valued when there is nothing to rate")
	}
}
