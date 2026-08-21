// Package yield accounts for what agent sessions cost and what they carried.
//
// The question it answers is not "how much did I spend" but "how much of what I
// spent was work, and how much was hauling the conversation around".
package yield

import (
	"sort"
	"time"

	"github.com/charris/hugel/internal/pricing"
	"github.com/charris/hugel/internal/transcript"
)

// Entry is the accounted form of one session.
type Entry struct {
	Session   *transcript.Session
	Cost      pricing.Cost
	Sidechain pricing.Cost
	Usage     transcript.Usage
	Unpriced  int // requests whose model had no known rate
	Models    []string
}

// Total is the session's whole bill.
func (e Entry) Total() float64 { return e.Cost.Total() }

// ContextTax is the fraction of spend that went to carrying context rather than
// producing output. A long session with a fat preamble drives this toward 1.
func (e Entry) ContextTax() float64 {
	t := e.Cost.Total()
	if t == 0 {
		return 0
	}
	return e.Cost.Context() / t
}

// CostPerPrompt is dollars per thing the gardener actually asked for.
func (e Entry) CostPerPrompt() float64 {
	if e.Session.Prompts == 0 {
		return 0
	}
	return e.Cost.Total() / float64(e.Session.Prompts)
}

// PriceRequest values one model request. ok is false when the model has no
// known rate.
func PriceRequest(r transcript.Request) (pricing.Cost, bool) {
	return pricing.Price(r.Model, r.Speed, pricing.Tokens{
		Input:        r.Usage.Input,
		Output:       r.Usage.Output,
		CacheRead:    r.Usage.CacheRead,
		CacheWrite5m: r.Usage.CacheWrite5m,
		CacheWrite1h: r.Usage.CacheWrite1h,
	})
}

// Price accounts a single session.
func Price(s *transcript.Session) Entry {
	e := Entry{Session: s, Models: s.Models()}
	for _, r := range s.Requests {
		e.Usage.Add(r.Usage)
		c, ok := PriceRequest(r)
		if !ok {
			if !pricing.Free(r.Model) {
				e.Unpriced++
			}
			continue
		}
		e.Cost.Add(c)
		if r.Sidechain {
			e.Sidechain.Add(c)
		}
	}
	return e
}

// Bed aggregates every session rooted in one project directory.
type Bed struct {
	Name      string
	Sessions  int
	Prompts   int
	Requests  int
	Usage     transcript.Usage
	Cost      pricing.Cost
	Sidechain pricing.Cost
	First     time.Time
	Last      time.Time
}

// ContextTax is the share of this bed's spend that went to carrying context.
func (b Bed) ContextTax() float64 {
	t := b.Cost.Total()
	if t == 0 {
		return 0
	}
	return b.Cost.Context() / t
}

// Report is a priced view over a set of sessions.
type Report struct {
	Entries  []Entry
	Beds     []Bed
	Cost     pricing.Cost
	Usage    transcript.Usage
	Unpriced int
	Since    time.Time
}

// Total is the whole bill for the report window.
func (r Report) Total() float64 { return r.Cost.Total() }

// ContextTax is the share of all spend that went to carrying context.
func (r Report) ContextTax() float64 {
	t := r.Cost.Total()
	if t == 0 {
		return 0
	}
	return r.Cost.Context() / t
}

// Filter narrows which sessions a report covers.
type Filter struct {
	Since time.Time // zero means no lower bound
	Bed   string    // empty means every bed
}

func (f Filter) match(s *transcript.Session) bool {
	if !f.Since.IsZero() && s.End.Before(f.Since) {
		return false
	}
	if f.Bed != "" && s.Bed != f.Bed {
		return false
	}
	return true
}

// Build prices the sessions that pass the filter and rolls them up by bed.
func Build(sessions []*transcript.Session, f Filter) Report {
	rep := Report{Since: f.Since}
	beds := map[string]*Bed{}

	for _, s := range sessions {
		if !f.match(s) {
			continue
		}
		e := Price(s)
		rep.Entries = append(rep.Entries, e)
		rep.Cost.Add(e.Cost)
		rep.Usage.Add(e.Usage)
		rep.Unpriced += e.Unpriced

		b := beds[s.Bed]
		if b == nil {
			b = &Bed{Name: s.Bed, First: s.Start, Last: s.End}
			beds[s.Bed] = b
		}
		b.Sessions++
		b.Prompts += s.Prompts
		b.Requests += len(s.Requests)
		b.Usage.Add(e.Usage)
		b.Cost.Add(e.Cost)
		b.Sidechain.Add(e.Sidechain)
		if s.Start.Before(b.First) {
			b.First = s.Start
		}
		if s.End.After(b.Last) {
			b.Last = s.End
		}
	}

	for _, b := range beds {
		rep.Beds = append(rep.Beds, *b)
	}
	sort.Slice(rep.Beds, func(i, j int) bool { return rep.Beds[i].Cost.Total() > rep.Beds[j].Cost.Total() })
	sort.Slice(rep.Entries, func(i, j int) bool { return rep.Entries[i].Total() > rep.Entries[j].Total() })
	return rep
}
