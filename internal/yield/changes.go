package yield

import (
	"sort"
	"strings"
	"time"

	"github.com/charris/hugel/internal/events"
	"github.com/charris/hugel/internal/pricing"
	"github.com/charris/hugel/internal/transcript"
)

// Attempt is one tender run, as much of it as accounting needs. It is passed in
// rather than read here so that this package stays free of bd and tmux and can
// be tested without either.
type Attempt struct {
	Bead     string
	Bed      string
	Worktree string
	Landed   bool // the change was accepted

	// Started is when the tender began. Accounting does not use it: it is here
	// so that a caller reading runs from more than one source can tell whether
	// two records describe the same run.
	Started time.Time
}

// Attempts reads tender runs out of the event log.
//
// The log is the record. A tender writes tender.start when it begins and the
// gate writes gate.land when the work goes in, and neither can be tidied away
// by deleting a directory the way tender state can.
//
// Landing is taken from the gate rather than from the tracker. A bead can be
// closed for any number of reasons -- superseded, abandoned, finished by hand --
// and only gate.land says the change was tested, reviewed and merged. Beads the
// gate never landed are left unlanded here for the caller to decide about,
// because work merged by hand is accepted change too.
func Attempts(log []events.Event) []Attempt {
	var out []Attempt
	landed := map[string]bool{}
	for _, e := range log {
		switch e.Name {
		case "tender.start":
			// A tender that could not be launched left no worktree and spent
			// nothing, so it is not an attempt at anything.
			if e.Outcome == "failed" {
				continue
			}
			w, _ := e.Fields["worktree"].(string)
			out = append(out, Attempt{
				Bead: e.Bead, Bed: e.Bed, Worktree: w, Started: e.Time,
			})
		case "gate.land":
			landed[e.Bead] = true
		}
	}
	// Landing is a property of the bead rather than of one run: a bead tended
	// twice and then landed did land, and both runs are what it cost.
	for i := range out {
		out[i].Landed = landed[out[i].Bead]
	}
	return out
}

// Change is what one bead cost to land, or to not land.
//
// Attempts counts tender runs rather than beads: a bead handed back once and
// tended again cost twice, and averaging that away would hide the thing most
// worth seeing.
type Change struct {
	Bead     string
	Bed      string
	Attempts int
	Sessions int
	Cost     pricing.Cost
	Usage    transcript.Usage
	Landed   bool
}

// Total is what this change cost, in equivalent API rates.
func (c Change) Total() float64 { return c.Cost.Total() }

// ChangeReport answers what a landed change costs, and what was spent without
// landing anything.
type ChangeReport struct {
	Changes []Change

	Landed   int
	Unlanded int

	LandedCost   float64
	UnlandedCost float64

	// Attributed is spend hugel can tie to a bead; Total is all spend in the
	// window. The difference is interactive work, which produces plenty of
	// accepted change and simply cannot be attributed this way. Reporting the
	// gap is the difference between a measurement and a misleading one.
	Attributed float64
	Total      float64
}

// PerChange is the number the design constraint names. It averages only over
// changes that actually landed, because dividing by work that did not land
// would make abandoning things look like efficiency.
func (r ChangeReport) PerChange() float64 {
	if r.Landed == 0 {
		return 0
	}
	return r.LandedCost / float64(r.Landed)
}

// Coverage is the share of spend this report can speak for.
func (r ChangeReport) Coverage() float64 {
	if r.Total == 0 {
		return 0
	}
	return r.Attributed / r.Total
}

// Changes accounts what each bead cost, by matching sessions to the worktrees
// they ran in.
//
// A tender and the reviewer that gated it both run in the same worktree, so
// both land on the same bead without either being told to. Which worktree is
// now read from the log the tender wrote when it started, rather than from the
// directory it left behind; the last inference is the path itself, since
// nothing records which transcript belongs to which tmux session.
func Changes(sessions []*transcript.Session, attempts []Attempt, f Filter) ChangeReport {
	var rep ChangeReport
	byBead := map[string]*Change{}
	seen := map[*transcript.Session]bool{}

	for _, a := range attempts {
		c := byBead[a.Bead]
		if c == nil {
			c = &Change{Bead: a.Bead, Bed: a.Bed, Landed: a.Landed}
			byBead[a.Bead] = c
		}
		c.Attempts++
		// Landing is a property of the bead, not of one attempt: a bead tended
		// twice and then landed did land.
		c.Landed = c.Landed || a.Landed

		for _, s := range sessions {
			if !f.match(s) || !under(s.CWD, a.Worktree) || seen[s] {
				continue
			}
			seen[s] = true
			e := Price(s)
			c.Sessions++
			c.Cost.Add(e.Cost)
			c.Usage.Add(e.Usage)
		}
	}

	for _, s := range sessions {
		if !f.match(s) {
			continue
		}
		rep.Total += Price(s).Cost.Total()
	}

	for _, c := range byBead {
		rep.Changes = append(rep.Changes, *c)
		rep.Attributed += c.Total()
		if c.Landed {
			rep.Landed++
			rep.LandedCost += c.Total()
			continue
		}
		rep.Unlanded++
		rep.UnlandedCost += c.Total()
	}
	sort.Slice(rep.Changes, func(i, j int) bool {
		if rep.Changes[i].Total() != rep.Changes[j].Total() {
			return rep.Changes[i].Total() > rep.Changes[j].Total()
		}
		return rep.Changes[i].Bead < rep.Changes[j].Bead
	})
	return rep
}

// under reports whether a session ran inside a worktree. Prefix matching on a
// path separator, so that a worktree at .../hugel4 does not claim sessions from
// .../hugel4-something-else.
func under(cwd, worktree string) bool {
	if cwd == "" || worktree == "" {
		return false
	}
	return cwd == worktree || strings.HasPrefix(cwd, strings.TrimSuffix(worktree, "/")+"/")
}
