package yield

import (
	"sort"
	"time"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
)

// SpikeReport settles the bet each spike was.
//
// A spike is cache warming: reading done outside a session so that several
// later sessions do not each pay for it. The analogy carries its own test.
// Warming a cache nobody reads is not a win, it is the same work in two steps,
// and a spike whose entries were never drawn was speculative after all.
type SpikeReport struct {
	Since  time.Time
	Spikes []SpikeWorth
}

// SpikeWorth is one spike's return.
type SpikeWorth struct {
	Bead     string
	Produced int // entries in the pile this spike is named on
	Reached  int // of those, how many were ever delivered to anyone
	Draws    int // deliveries, counting an entry once per draw it appeared in

	Accepted   int
	Rejected   int
	Unreviewed int
}

// Verdict says what the numbers came to, in the vocabulary the epic argued in.
//
// Rejected outranks never-drawn deliberately. An entry nobody drew cost the
// tokens to produce and nothing since; a drawn entry that was thrown out also
// competed in the rankings and pushed something better down, so it charged
// every session it lost a place in.
func (w SpikeWorth) Verdict() string {
	switch {
	case w.Produced == 0:
		return "found nothing"
	case w.Rejected > w.Accepted:
		return "drawn and thrown out"
	case w.Accepted > 0:
		return "drawn and kept"
	case w.Reached == 0:
		return "never drawn"
	}
	return "drawn, not yet judged"
}

// Paid reports whether a spike has visibly earned its keep. Unreviewed
// deliveries are not counted for it: this is worth having only if it can come
// out badly, and a spike nobody has ruled on has not been shown to pay.
func (w SpikeWorth) Paid() bool { return w.Accepted > 0 && w.Accepted >= w.Rejected }

// Spikes accounts what each spike put into the pile and what became of it.
//
// Entries are the unit rather than draws, because what a spike produced is a
// set of claims and what happened to them is a verdict on each. A draw is how
// a claim reached somebody; several draws of one entry are the same claim
// arriving again, which is reach, not extra worth.
func Spikes(log []draws.Draw, entries []*pile.Entry, f Filter) SpikeReport {
	rep := SpikeReport{Since: f.Since}

	// Which spike, if any, is named on each entry -- and how each has fared.
	spikeOf := map[string]string{}
	for _, e := range entries {
		if e.Source.Spike == "" {
			continue
		}
		if f.Bed != "" && e.Bed != f.Bed {
			continue
		}
		if !f.Since.IsZero() && e.OccurredAt.Before(f.Since) {
			continue
		}
		spikeOf[e.ID] = e.Source.Spike
	}

	worth := map[string]*SpikeWorth{}
	of := func(bead string) *SpikeWorth {
		w := worth[bead]
		if w == nil {
			w = &SpikeWorth{Bead: bead}
			worth[bead] = w
		}
		return w
	}
	for _, e := range entries {
		bead, ok := spikeOf[e.ID]
		if !ok {
			continue
		}
		w := of(bead)
		w.Produced++
		switch e.Review {
		case pile.Accepted:
			w.Accepted++
		case pile.Rejected:
			w.Rejected++
		default:
			w.Unreviewed++
		}
	}

	// Reach is counted per entry across the whole log, so an entry drawn twice
	// is one entry reached and two draws.
	reached := map[string]bool{}
	for _, d := range log {
		if !f.Since.IsZero() && d.At.Before(f.Since) {
			continue
		}
		if f.Bed != "" && d.Bed != f.Bed {
			continue
		}
		for _, id := range d.Entries {
			bead, ok := spikeOf[id]
			if !ok {
				continue
			}
			w := of(bead)
			w.Draws++
			if !reached[id] {
				reached[id] = true
				w.Reached++
			}
		}
	}

	for _, w := range worth {
		rep.Spikes = append(rep.Spikes, *w)
	}
	// Most drawn first: the spikes that paid are the ones worth reading about,
	// and a spike that produced nothing sorts to the bottom where it belongs.
	sort.Slice(rep.Spikes, func(i, j int) bool {
		a, b := rep.Spikes[i], rep.Spikes[j]
		if a.Draws != b.Draws {
			return a.Draws > b.Draws
		}
		if a.Produced != b.Produced {
			return a.Produced > b.Produced
		}
		return a.Bead < b.Bead
	})
	return rep
}
