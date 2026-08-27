package yield

import (
	"testing"
	"time"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
)

func spikeEntry(id, bead string, review pile.Review) *pile.Entry {
	return &pile.Entry{
		ID: id, Bed: "hugel4", Review: review,
		OccurredAt: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		Source:     pile.Source{Spike: bead},
	}
}

// The whole report exists to separate three fates that look alike from inside
// a spike and are worth very different amounts from outside it.
func TestSpikesSeparateWhatWasKeptFromWhatWasNeverRead(t *testing.T) {
	entries := []*pile.Entry{
		spikeEntry("kept", "s-paid", pile.Accepted),
		spikeEntry("cold", "s-cold", pile.Unreviewed),
		spikeEntry("bad", "s-bad", pile.Rejected),
		// Ordinary work, no spike: it must not appear anywhere below.
		{ID: "work", Bed: "hugel4", Review: pile.Accepted},
	}
	log := []draws.Draw{
		{At: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC), Bed: "hugel4",
			Entries: []string{"kept", "bad", "work"}},
		// The same entry again: reach counts it once, draws twice.
		{At: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC), Bed: "hugel4",
			Entries: []string{"kept"}},
	}

	rep := Spikes(log, entries, Filter{})
	if len(rep.Spikes) != 3 {
		t.Fatalf("got %d spikes, want 3: %+v", len(rep.Spikes), rep.Spikes)
	}
	by := map[string]SpikeWorth{}
	for _, w := range rep.Spikes {
		by[w.Bead] = w
	}

	paid := by["s-paid"]
	if paid.Produced != 1 || paid.Reached != 1 || paid.Draws != 2 || paid.Accepted != 1 {
		t.Errorf("s-paid = %+v, want one entry reached once over two draws and kept", paid)
	}
	if paid.Verdict() != "drawn and kept" || !paid.Paid() {
		t.Errorf("s-paid verdict = %q, paid = %v", paid.Verdict(), paid.Paid())
	}

	cold := by["s-cold"]
	if cold.Reached != 0 || cold.Draws != 0 {
		t.Errorf("s-cold = %+v, want nothing drawn", cold)
	}
	if cold.Verdict() != "never drawn" {
		t.Errorf("s-cold verdict = %q, want never drawn", cold.Verdict())
	}
	if cold.Paid() {
		t.Error("a spike nobody drew was counted as having paid")
	}

	bad := by["s-bad"]
	if bad.Verdict() != "drawn and thrown out" || bad.Paid() {
		t.Errorf("s-bad verdict = %q, paid = %v", bad.Verdict(), bad.Paid())
	}
	// The entry that belongs to no spike was in the same draw as the others.
	for _, w := range rep.Spikes {
		if w.Bead == "" {
			t.Error("work that was not a spike was attributed to one")
		}
	}
}

// An unjudged spike must not read as a success. This report is worth having
// only if it can come out badly, and not looking is not a good outcome.
func TestAnUnjudgedSpikeHasNotPaid(t *testing.T) {
	entries := []*pile.Entry{spikeEntry("e", "s", pile.Unreviewed)}
	log := []draws.Draw{{At: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC), Entries: []string{"e"}}}

	rep := Spikes(log, entries, Filter{})
	if len(rep.Spikes) != 1 {
		t.Fatalf("got %+v", rep.Spikes)
	}
	w := rep.Spikes[0]
	if w.Paid() {
		t.Error("a spike nobody has ruled on was counted as having paid")
	}
	if w.Verdict() != "drawn, not yet judged" {
		t.Errorf("verdict = %q", w.Verdict())
	}
}

// Being thrown out is worse than never being drawn, because a rejected entry
// also competed in the rankings and pushed a better one down. The verdict has
// to say so even when the spike had a kept entry to its name as well.
func TestThrownOutOutranksKeptWhenItLosesMore(t *testing.T) {
	entries := []*pile.Entry{
		spikeEntry("a", "s", pile.Accepted),
		spikeEntry("b", "s", pile.Rejected),
		spikeEntry("c", "s", pile.Rejected),
	}
	rep := Spikes(nil, entries, Filter{})
	if got := rep.Spikes[0].Verdict(); got != "drawn and thrown out" {
		t.Errorf("verdict = %q, want the rejections to dominate", got)
	}
	if rep.Spikes[0].Paid() {
		t.Error("a spike that lost more entries than it kept was counted as having paid")
	}
}
