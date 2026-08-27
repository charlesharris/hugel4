package compost

import (
	"testing"
	"time"

	"github.com/charris/hugel/internal/pile"
)

func decision(bed, title string) *pile.Entry {
	e := &pile.Entry{Type: pile.Decision, Scope: pile.ScopeBed, Bed: bed, Title: title}
	e.Seal(time.Date(2026, 8, 19, 23, 0, 0, 0, time.UTC))
	return e
}

// The pile has carried the contradicts relation since it was built and nothing
// ever wrote one. A revert is what writes it.
func TestARevertLinksToTheDecisionItTookBack(t *testing.T) {
	original := decision("garden", "Trust the popularity score")
	revert := &pile.Entry{
		Type: pile.Failure, Scope: pile.ScopeBed, Bed: "garden",
		Title: `Revert "Trust the popularity score"`,
		Body:  "It picked the wrong film for every disc older than 1990.",
	}

	n := LinkReverts([]*pile.Entry{revert}, func(id string) bool { return id == original.ID })
	if n != 1 {
		t.Fatalf("wrote %d links, want 1", n)
	}
	got := revert.Contradicted()
	if len(got) != 1 || got[0] != original.ID {
		t.Errorf("links = %v, want the decision %s", revert.Links, original.ID)
	}
}

// An edge to nothing reads as evidence until someone tries to follow it, and a
// revert of work that was never composted is ordinary rather than an error.
func TestNoEdgeIsWrittenToADecisionThePileDoesNotHave(t *testing.T) {
	revert := &pile.Entry{
		Type: pile.Failure, Bed: "garden", Title: `Revert "something never composted"`,
	}
	if n := LinkReverts([]*pile.Entry{revert}, func(string) bool { return false }); n != 0 {
		t.Fatalf("wrote %d links, want none", n)
	}
	if len(revert.Links) != 0 {
		t.Errorf("links = %v, want none", revert.Links)
	}
}

// A hand-written revert subject names nothing findable. Guessing at which
// decision it meant would put an edge in the pile that nobody can check.
func TestARevertThatNamesNothingLinksToNothing(t *testing.T) {
	revert := &pile.Entry{Type: pile.Failure, Bed: "garden", Title: "Revert the scoring change"}
	if n := LinkReverts([]*pile.Entry{revert}, func(string) bool { return true }); n != 0 {
		t.Errorf("wrote %d links from a subject that names no decision", n)
	}
}

// Linking twice must not write the edge twice: composting a session again is
// meant to converge.
func TestLinkingIsIdempotent(t *testing.T) {
	original := decision("garden", "Trust the popularity score")
	revert := &pile.Entry{
		Type: pile.Failure, Bed: "garden", Title: `Revert "Trust the popularity score"`,
	}
	known := func(id string) bool { return id == original.ID }
	LinkReverts([]*pile.Entry{revert}, known)
	LinkReverts([]*pile.Entry{revert}, known)
	if len(revert.Links) != 1 {
		t.Errorf("links = %v, want one", revert.Links)
	}
}
