package compost

import (
	"regexp"
	"strings"

	"github.com/charris/hugel/internal/pile"
)

// revertSubject pulls the original subject out of a revert commit's own. git
// writes this form itself, which is why it can be relied on; a hand-written
// "Revert the thing" names nothing findable and is left alone.
var revertSubject = regexp.MustCompile(`^Revert\s+"(.+)"\s*$`)

// LinkReverts writes the contradicts edge from a revert to the decision it took
// back, and reports how many it wrote.
//
// The join is the title. A revert commit's subject is the original subject
// quoted, an entry's id is derived from its title, and so the entry a revert
// falsifies is computable without git, a sha, or a lookup service. That is a
// weaker join than a commit id and it is the one the pile can actually make:
// entries do not record the commit they came from.
//
// known says whether an id is in the pile, and nothing is linked without it. A
// dangling edge is worse than no edge -- it reads as evidence until someone
// tries to follow it -- and a revert of work that was never composted is the
// ordinary case, not an error.
func LinkReverts(entries []*pile.Entry, known func(id string) bool) int {
	written := 0
	for _, e := range entries {
		if e.Type != pile.Failure {
			continue
		}
		m := revertSubject.FindStringSubmatch(strings.TrimSpace(e.Title))
		if m == nil {
			continue
		}
		target := resolve(e.Bed, cleanTitle(m[1]), known)
		if target == "" || target == e.Identity() {
			continue
		}
		if linked(e, target) {
			continue
		}
		e.Links = append(e.Links, pile.Link{Rel: pile.RelContradicts, ID: target})
		written++
	}
	return written
}

// resolve finds the entry a title would have made. Scope is tried both ways
// because the extractor decides it per entry from the text, so a decision and
// the revert that takes it back can disagree about whether it travelled.
//
// Only decisions are looked for. A revert takes back a change that was made,
// and a change that was made composts as a decision; a pattern or a constraint
// is a claim about how things are, which a revert is not evidence against.
func resolve(bed, title string, known func(id string) bool) string {
	for _, scope := range []pile.Scope{pile.ScopeBed, pile.ScopeGeneral} {
		probe := &pile.Entry{Type: pile.Decision, Scope: scope, Bed: bed, Title: title}
		if id := probe.Identity(); known(id) {
			return id
		}
	}
	return ""
}

func linked(e *pile.Entry, id string) bool {
	for _, l := range e.Links {
		if l.ID == id && l.Rel == pile.RelContradicts {
			return true
		}
	}
	return false
}
