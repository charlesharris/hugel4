package tend

import (
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/yield"
)

var now = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func ent(id, title string, madeMin int, session string) *pile.Entry {
	return &pile.Entry{
		ID: id, Title: title, Review: pile.Unreviewed, Status: pile.Active,
		CreatedAt: now.Add(time.Duration(madeMin) * time.Minute),
		Source:    pile.Source{Session: session},
	}
}

func drew(atMin int, ids ...string) draws.Draw {
	return draws.Draw{At: now.Add(time.Duration(atMin) * time.Minute), Entries: ids}
}

func titles(rows []Row, k Kind) []string {
	var out []string
	for _, r := range rows {
		if r.Kind == k {
			out = append(out, r.Entry.Title)
		}
	}
	return out
}

// Drawn entries are judged first: they are the only ones that have cost tokens
// rather than disk, so they lead regardless of when they were composted.
func TestDeliveredComesFirstAndIsNotRepeatedBelow(t *testing.T) {
	entries := []*pile.Entry{
		ent("a", "an old entry, freshly drawn", -10_000, "s1"),
		ent("b", "composted today", -60, "s1"),
	}
	log := []draws.Draw{drew(-30, "a")}

	a := Gather(entries, log, yield.SoilReport{}, now.Add(-24*time.Hour))
	if got := titles(a.Rows(0), Drawn); len(got) != 1 || got[0] != "an old entry, freshly drawn" {
		t.Errorf("delivered = %v", got)
	}
	if got := titles(a.Rows(0), Composted); len(got) != 1 || got[0] != "composted today" {
		t.Errorf("new = %v, want the drawn entry not repeated here", got)
	}
}

// An entry drawn three times poses one question, not three.
func TestRepeatedDrawsListOnce(t *testing.T) {
	entries := []*pile.Entry{ent("a", "drawn a lot", -60, "s1")}
	log := []draws.Draw{drew(-50, "a"), drew(-40, "a"), drew(-30, "a")}

	a := Gather(entries, log, yield.SoilReport{}, now.Add(-24*time.Hour))
	if len(a.Delivered) != 1 {
		t.Errorf("delivered %d rows, want 1", len(a.Delivered))
	}
}

// The window is the whole discipline: sitting down after three days away must
// show three days, not the whole pile.
func TestWindowBoundsBothGroups(t *testing.T) {
	entries := []*pile.Entry{
		ent("old", "composted last month", -60_000, "s0"),
		ent("new", "composted this morning", -300, "s1"),
		ent("drawnold", "drawn last month", -60_000, "s0"),
	}
	log := []draws.Draw{drew(-59_000, "drawnold"), drew(-100, "new")}

	a := Gather(entries, log, yield.SoilReport{}, now.Add(-24*time.Hour))
	if len(a.Delivered) != 1 || a.Delivered[0].ID != "new" {
		t.Errorf("delivered = %+v, want only the recent draw", a.Delivered)
	}
	if len(a.Fresh) != 0 {
		t.Errorf("fresh = %+v, want none (the only recent entry was drawn)", a.Fresh)
	}
}

// A draw whose entry has since left the pile must not crash the surface or
// invent a row for something that cannot be read.
func TestVanishedEntriesAreDropped(t *testing.T) {
	a := Gather(nil, []draws.Draw{drew(-30, "gone")}, yield.SoilReport{}, now.Add(-24*time.Hour))
	if len(a.Delivered) != 0 {
		t.Errorf("delivered = %+v, want nothing", a.Delivered)
	}
	if len(a.Rows(0)) != 2 {
		t.Errorf("rows = %d, want the two headings", len(a.Rows(0)))
	}
}

// Sessions that compost to nothing are the finding that motivated this surface,
// so they are counted rather than quietly omitted.
func TestBarrenSessionsAreCounted(t *testing.T) {
	entries := []*pile.Entry{
		ent("a", "from one session", -60, "s1"),
		ent("b", "also from it", -60, "s1"),
	}
	a := Gather(entries, nil, yield.SoilReport{Sessions: 12}, now.Add(-24*time.Hour))
	if a.Producing != 1 {
		t.Errorf("producing = %d, want 1", a.Producing)
	}
	if a.Barren != 11 {
		t.Errorf("barren = %d, want 11", a.Barren)
	}
}

// An empty group says something. Hiding it would turn "the pile was never
// asked" into a blank space.
func TestEmptyGroupsKeepTheirHeadings(t *testing.T) {
	a := Gather(nil, nil, yield.SoilReport{}, now.Add(-24*time.Hour))
	rows := a.Rows(0)
	if len(rows) != 2 || rows[0].Kind != Heading || rows[1].Kind != Heading {
		t.Fatalf("rows = %+v, want two headings", rows)
	}
	if rows[0].Label == "DELIVERED" {
		t.Error("an empty group should say it is empty, not just show its name")
	}
}

func TestUnjudgedCountsOnlyWhatNeedsAVerdict(t *testing.T) {
	entries := []*pile.Entry{
		ent("a", "needs judging", -60, "s1"),
		ent("b", "already kept", -60, "s1"),
		ent("c", "already dead", -60, "s1"),
	}
	entries[1].Review = pile.Accepted
	entries[2].Status = pile.Abandoned

	a := Gather(entries, nil, yield.SoilReport{}, now.Add(-24*time.Hour))
	if got := Unjudged(a.Rows(0)); got != 1 {
		t.Errorf("unjudged = %d, want 1", got)
	}
}

// A bulk import or a first compost run puts hundreds of entries inside any
// window. The surface must cap what it shows and say what it left out, or it
// becomes the queue this refuses to be.
func TestGroupsAreCappedAndSaySo(t *testing.T) {
	var es []*pile.Entry
	for i := 0; i < 249; i++ {
		es = append(es, ent(string(rune('a'+i%26))+string(rune('a'+i/26)), "entry", -60, "s1"))
	}
	a := Gather(es, nil, yield.SoilReport{}, now.Add(-24*time.Hour))
	rows := a.Rows(25)

	shown := 0
	for _, r := range rows {
		if r.Kind == Composted {
			shown++
		}
	}
	if shown != 25 {
		t.Errorf("showed %d entries, want 25", shown)
	}
	if Unjudged(rows) != 25 {
		t.Errorf("unjudged = %d, want the shown rows only", Unjudged(rows))
	}
	var label string
	for _, r := range rows {
		if r.Kind == Heading && strings.HasPrefix(r.Label, "NEW") {
			label = r.Label
		}
	}
	if !strings.Contains(label, "249") || !strings.Contains(label, "25") {
		t.Errorf("heading %q should say how many were left out", label)
	}
}
