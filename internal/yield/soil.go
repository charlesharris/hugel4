package yield

import (
	"sort"
	"time"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/transcript"
)

// SoilReport answers the two questions soil and its skill shipped without.
//
// Reach is whether the pile gets asked at all: an agent that never draws makes
// every argument about ranking and budget moot. Precision is whether what came
// back was worth keeping, judged by what a human later did to those entries.
// Both are deliberately unflattering by default — an unreviewed entry counts
// as neither good nor bad, so precision cannot be inflated by not looking.
type SoilReport struct {
	Since    time.Time
	Beds     []SoilBed
	Draws    int
	Sessions int
	Reached  int // sessions with at least one draw

	Tokens    int
	Delivered int // entries handed over, counting repeats
	Distinct  int // entries handed over at least once

	Accepted   int
	Rejected   int
	Unreviewed int
	Missing    int // drawn, then composted away or renamed out of the pile
}

// SoilBed is one project's use of the pile.
type SoilBed struct {
	Name     string
	Draws    int
	Sessions int
	Reached  int
	Tokens   int
}

// Reach is the share of sessions that asked the pile anything. It is the miss
// rate of pull delivery, stated as its complement.
func (r SoilReport) Reach() float64 {
	if r.Sessions == 0 {
		return 0
	}
	return float64(r.Reached) / float64(r.Sessions)
}

// Precision is the share of judged deliveries a human kept. Deliveries nobody
// has judged are excluded rather than assumed good: this number is worth
// having only if it can come out badly.
func (r SoilReport) Precision() float64 {
	judged := r.Accepted + r.Rejected
	if judged == 0 {
		return 0
	}
	return float64(r.Accepted) / float64(judged)
}

// Judged is how many distinct delivered entries anyone has ruled on.
func (r SoilReport) Judged() int { return r.Accepted + r.Rejected }

// Soil accounts what the pile was asked and what came of it. Draws are matched
// to sessions by time and bed, because a draw runs in a shell that has no way
// to name the session it happens inside.
func Soil(sessions []*transcript.Session, log []draws.Draw, entries []*pile.Entry, f Filter) SoilReport {
	rep := SoilReport{Since: f.Since}
	cutoff := f.Since

	standing := map[string]pile.Review{}
	for _, e := range entries {
		standing[e.ID] = e.Review
	}

	beds := map[string]*SoilBed{}
	bedOf := func(name string) *SoilBed {
		b := beds[name]
		if b == nil {
			b = &SoilBed{Name: name}
			beds[name] = b
		}
		return b
	}

	// A session counts as reached if any draw falls inside its span. Sessions
	// are the denominator, so they are counted whether or not they drew.
	type span struct {
		s       *transcript.Session
		reached bool
	}
	var spans []*span
	for _, s := range sessions {
		if !f.match(s) {
			continue
		}
		spans = append(spans, &span{s: s})
	}

	seen := map[string]bool{}
	for _, d := range log {
		if !cutoff.IsZero() && d.At.Before(cutoff) {
			continue
		}
		if f.Bed != "" && d.Bed != f.Bed {
			continue
		}
		rep.Draws++
		rep.Tokens += d.Tokens
		b := bedOf(d.Bed)
		b.Draws++
		b.Tokens += d.Tokens

		for _, sp := range spans {
			if sp.s.Bed != d.Bed {
				continue
			}
			if d.At.Before(sp.s.Start) || d.At.After(sp.s.End) {
				continue
			}
			sp.reached = true
			break
		}

		for _, id := range d.Entries {
			rep.Delivered++
			if seen[id] {
				continue
			}
			seen[id] = true
			rep.Distinct++
			switch standing[id] {
			case pile.Accepted:
				rep.Accepted++
			case pile.Rejected:
				rep.Rejected++
			case pile.Unreviewed:
				rep.Unreviewed++
			default:
				rep.Missing++
			}
		}
	}

	for _, sp := range spans {
		rep.Sessions++
		b := bedOf(sp.s.Bed)
		b.Sessions++
		if sp.reached {
			rep.Reached++
			b.Reached++
		}
	}

	for _, b := range beds {
		if b.Name == "" {
			b.Name = "—"
		}
		rep.Beds = append(rep.Beds, *b)
	}
	sort.Slice(rep.Beds, func(i, j int) bool {
		if rep.Beds[i].Draws != rep.Beds[j].Draws {
			return rep.Beds[i].Draws > rep.Beds[j].Draws
		}
		return rep.Beds[i].Name < rep.Beds[j].Name
	})
	return rep
}
