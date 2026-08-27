// Package survival grades the gate on what became of the work it approved.
//
// Work that passed review and was reverted three weeks later means the review
// was wrong. Nothing in the garden could say whether the reviewer is a real
// filter or a rubber stamp, and it holds merge and push rights on unattended
// runs, which makes that the most consequential unanswered question here.
//
// It grades more than the reviewer: the same edge grades the tender that wrote
// the work, the brief it was given, and the soil that fed it. The gate is named
// because the gate is the one that said yes.
//
// Reporting only. A reviewer whose approvals keep failing does not get a
// stricter brief here, and files with poor survival are not gated harder. That
// is the first place this system would start adapting on its own measurements
// rather than recording them, and it is deliberately not started.
//
// Grading is split from looking, the way accounting is: Grade is arithmetic
// over facts handed to it, so the judgement can be tested without a git
// repository or a bd installation anywhere near it.
package survival

import (
	"sort"
	"time"

	"github.com/charris/hugel/internal/events"
)

// Landing is one gate-approved change, as the event log recorded it.
type Landing struct {
	Bead string
	Bed  string
	Repo string
	SHA  string

	// Base is what the branch pointed at before this landed, which makes the
	// commits this landing introduced computable as base..sha. Landings
	// recorded before the gate emitted it have none, and can only be matched on
	// SHA itself.
	Base string

	Into string // the branch it landed on
	At   time.Time
}

// Fate is what became of an approved change.
type Fate string

const (
	Held     Fate = "held"     // landed, given time to fail, and did not
	Reverted Fate = "reverted" // taken back out of the tree
	Reopened Fate = "reopened" // the gate closed the bead and it is open again
	Young    Fate = "young"    // landed too recently to have survived anything
)

// Found is work somebody filed because of a landing. The relation is bd's own
// and is written by whoever filed the follow-up, which is what makes it worth
// reading: a person saying this exists because of that.
type Found struct {
	Bead  string `json:"bead"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Open  bool   `json:"open"`
}

// Fact is what the world said about a bead after its work landed. It is
// gathered by Look and passed in, so that what counts as survival is decided in
// one place and read in another.
type Fact struct {
	RevertedBy string
	RevertedAt time.Time
	Subject    string // the reverting commit's subject, which usually says why
	Status     string // the bead's status now; empty when bd could not be asked

	// Found is reported beside a landing and never counted against it. Plenty
	// of work that held perfectly well turned something up on the way, and a
	// rate that treated a follow-up as a failure would grade thoroughness as
	// harm.
	Found []Found
}

// Verdict is one landing and what became of it.
type Verdict struct {
	Landing
	Fate  Fate
	Why   string
	Age   time.Duration // landing to revert, or landing to now
	Found []Found       // work filed because of it, reported and not counted
}

// Report is the gate's record over a window.
type Report struct {
	Verdicts []Verdict

	Held     int
	Reverted int
	Reopened int
	Young    int

	// Unattributed counts reverts seen in the repositories that name no commit
	// any landing introduced. They are reported rather than dropped: a revert
	// hugel cannot attribute is still a revert, and a survival rate that
	// quietly excluded the ones it could not explain would flatter itself.
	Unattributed int

	Window time.Duration
	Mature time.Duration
}

// Judged is how many landings were old enough to have an answer.
func (r Report) Judged() int { return r.Held + r.Reverted + r.Reopened }

// Rate is the fraction of judged landings that held.
//
// Young landings are excluded rather than counted as survivors. A change that
// landed an hour ago has not survived anything; it has not yet been given the
// chance to fail, and counting it as held would make a burst of fresh work look
// like a gate that improved.
func (r Report) Rate() float64 {
	if r.Judged() == 0 {
		return 0
	}
	return float64(r.Held) / float64(r.Judged())
}

// Measurable reports whether anything has been graded at all. A rate with no
// denominator is not a measurement, and zero would say every approval failed.
func (r Report) Measurable() bool { return r.Judged() > 0 }

// Landings pulls the gate's approvals out of the event log.
//
// The repository comes from the tender that did the work, because gate.land
// records where the change went and not where it lives. Correlating them by
// bead is what the event log is for.
func Landings(log []events.Event, since time.Time) []Landing {
	repo := map[string]string{}
	var out []Landing
	for _, e := range log {
		switch e.Name {
		case "tender.start":
			if r, ok := e.Fields["repo"].(string); ok && r != "" {
				repo[e.Bead] = r
			}
		case "gate.land":
			if !since.IsZero() && e.Time.Before(since) {
				continue
			}
			l := Landing{Bead: e.Bead, Bed: e.Bed, Repo: repo[e.Bead], At: e.Time}
			l.SHA, _ = e.Fields["sha"].(string)
			l.Base, _ = e.Fields["base"].(string)
			l.Into, _ = e.Fields["into"].(string)
			out = append(out, l)
		}
	}
	return out
}

// Grade decides what became of each landing, from facts gathered elsewhere.
//
// A revert outranks everything, including age: evidence that arrived is worth
// more than a rule about how long to wait for it. A reopened bead counts
// against the gate too, and is kept as its own fate rather than folded into
// reverted, because the two say different things -- one is the tree rejecting
// the change, the other is a person saying the work was not done.
func Grade(landings []Landing, facts map[string]Fact, now time.Time, mature time.Duration) Report {
	rep := Report{Mature: mature}
	for _, l := range landings {
		f := facts[l.Bead]
		v := Verdict{Landing: l, Age: now.Sub(l.At), Found: f.Found}
		switch {
		case f.RevertedBy != "":
			v.Fate, v.Why = Reverted, short(f.RevertedBy)+" "+f.Subject
			if !f.RevertedAt.IsZero() {
				v.Age = f.RevertedAt.Sub(l.At)
			}
			rep.Reverted++
		case f.Status != "" && f.Status != "closed":
			v.Fate, v.Why = Reopened, "the gate closed it; it is "+f.Status+" again"
			rep.Reopened++
		case now.Sub(l.At) < mature:
			v.Fate, v.Why = Young, "landed too recently to have survived anything"
			rep.Young++
		default:
			v.Fate = Held
			rep.Held++
		}
		rep.Verdicts = append(rep.Verdicts, v)
	}
	// Worst first, then newest: a report read from the top should open with the
	// approvals that did not hold, which are the only ones worth reading.
	sort.SliceStable(rep.Verdicts, func(i, j int) bool {
		a, b := rep.Verdicts[i], rep.Verdicts[j]
		if rank(a.Fate) != rank(b.Fate) {
			return rank(a.Fate) < rank(b.Fate)
		}
		// A landing that held but turned something up is worth reading before
		// one that has nothing to say.
		if (len(a.Found) > 0) != (len(b.Found) > 0) {
			return len(a.Found) > len(b.Found)
		}
		return a.At.After(b.At)
	})
	return rep
}

func rank(f Fate) int {
	switch f {
	case Reverted:
		return 0
	case Reopened:
		return 1
	case Young:
		return 2
	}
	return 3
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
