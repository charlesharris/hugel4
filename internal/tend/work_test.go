package tend

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/yield"
)

func bead(id, title, status string, ready bool) beads.Bead {
	return beads.Bead{ID: id, Title: title, Status: status, Ready: ready, Type: "task"}
}

func work(bed string, bs ...beads.Bead) *beads.Work {
	return &beads.Work{Bed: bed, Dir: "/tmp/" + bed, Beads: bs}
}

// The order is the question a gardener is asking: what is half-finished and
// waiting on me, what could be started, and what cannot move.
func TestWorkOrdersByWhatItNeeds(t *testing.T) {
	g := Garden{Beds: []*beads.Work{work("hugel4",
		bead("c", "blocked one", "open", false),
		bead("b", "ready one", "open", true),
		bead("a", "in flight", "in_progress", true),
	)}}
	rows := g.Rows(0)

	var got []string
	for _, r := range rows {
		if r.Kind == Work {
			got = append(got, r.Bead.ID)
		}
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("order = %v, want in-flight, ready, blocked", got)
	}
}

// One bed with thirty ready beads must not bury the others, and what is left
// out has to be stated -- the same refusal the knowledge side makes.
func TestWorkIsCappedPerBedAndSaysSo(t *testing.T) {
	var bs []beads.Bead
	for i := 0; i < 30; i++ {
		bs = append(bs, bead(string(rune('a'+i%26))+string(rune('0'+i/26)), "a task", "open", true))
	}
	g := Garden{Beds: []*beads.Work{work("tourdesource", bs...), work("hugel4", bead("h", "one", "open", true))}}
	rows := g.Rows(5)

	shown := map[string]int{}
	for _, r := range rows {
		if r.Kind == Work {
			shown[r.Bed]++
		}
	}
	if shown["tourdesource"] != 5 {
		t.Errorf("showed %d of the busy bed, want 5", shown["tourdesource"])
	}
	if shown["hugel4"] != 1 {
		t.Errorf("the quiet bed was buried: %d rows", shown["hugel4"])
	}
	var heading string
	for _, r := range rows {
		if r.Kind == Heading && strings.HasPrefix(r.Label, "TOURDESOURCE") {
			heading = r.Label
		}
	}
	if !strings.Contains(heading, "5 of 30") {
		t.Errorf("heading %q should say what was left out", heading)
	}
}

func TestTotalsAddUpEveryBed(t *testing.T) {
	g := Garden{Beds: []*beads.Work{
		work("a", bead("1", "x", "open", true), bead("2", "y", "in_progress", true)),
		work("b", bead("3", "z", "open", false)),
	}}
	tl := g.Totals()
	if tl.Ready != 1 || tl.Active != 1 || tl.Blocked != 1 {
		t.Errorf("totals = %+v, want 1 ready, 1 active, 1 blocked", tl)
	}
}

// A garden with no tracked work anywhere still has to render something that
// says so, rather than an empty box.
func TestEmptyGardenSaysSo(t *testing.T) {
	rows := Garden{}.Rows(10)
	if len(rows) != 1 || rows[0].Kind != Heading || !strings.Contains(rows[0].Label, "no bed") {
		t.Errorf("rows = %+v, want one heading saying nothing is tracked", rows)
	}
}

func gardenModel(t *testing.T) Model {
	t.Helper()
	es := []*pile.Entry{ent("k1", "some knowledge", -60, "s1")}
	g := Garden{Beds: []*beads.Work{work("hugel4",
		bead("hugel4-1", "in flight here", "in_progress", true),
		bead("hugel4-2", "ready here", "open", true),
	)}}
	a := Gather(es, nil, yield.SoilReport{Sessions: 3, Reached: 1}, now.Add(-24*3600*1e9), nil)
	m := NewGarden(g, a, newFake(es...), 12)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	return next.(Model)
}

// Work and knowledge are two views of one sitting, so tab moves between them
// rather than between programs.
func TestTabSwitchesPanes(t *testing.T) {
	m := gardenModel(t)
	if m.row().Bead == nil {
		t.Fatal("garden did not open on work")
	}
	m = press(m, "tab")
	if m.row().Entry == nil {
		t.Fatal("tab did not reach knowledge")
	}
	m = press(m, "tab")
	if m.row().Bead == nil {
		t.Error("tab did not come back to work")
	}
}

// Losing your place because you glanced at the other pane is the kind of small
// rudeness that stops a surface being sat in front of.
func TestPanesKeepTheirPlace(t *testing.T) {
	m := gardenModel(t)
	m = press(m, "j")
	where := m.cursor
	m = press(m, "tab", "tab")
	if m.cursor != where {
		t.Errorf("cursor came back at %d, want %d", m.cursor, where)
	}
}

// A verdict belongs to knowledge. A bead's standing is bd's business, and a
// keystroke that judges must do nothing while the cursor is on work.
func TestVerdictKeysDoNothingOnWork(t *testing.T) {
	m := gardenModel(t)
	f := m.store.(*fakeStore)
	m = press(m, "a", "r", "x", "u", "s")
	if m.judged != 0 {
		t.Errorf("judged %d work rows, want none", m.judged)
	}
	if len(f.commits) != 0 {
		t.Errorf("work rows produced commits: %v", f.commits)
	}
	if m.pendingSupersede != nil {
		t.Error("supersede armed on a bead")
	}
}

func TestGardenViewShowsBothSides(t *testing.T) {
	m := gardenModel(t)
	out := m.View()
	for _, want := range []string{"hugel garden", "HUGEL4", "in flight", "1 ready", "tab knowledge"} {
		if !strings.Contains(out, want) {
			t.Errorf("work view is missing %q", want)
		}
	}
	out = press(m, "tab").View()
	for _, want := range []string{"NEW", "to judge", "tab work"} {
		if !strings.Contains(out, want) {
			t.Errorf("knowledge view is missing %q", want)
		}
	}
	t.Logf("\n%s", m.View())
}

// What you owe comes before what is running, because a tender will pick up
// ready work unattended and will never pick up work waiting on a person.
func TestWorkYouOweComesFirst(t *testing.T) {
	yours := bead("y", "waiting on you", "open", true)
	yours.Labels = []string{beads.NeedsAttention}
	g := Garden{Beds: []*beads.Work{work("hugel4",
		bead("c", "blocked one", "open", false),
		bead("b", "ready one", "open", true),
		bead("a", "in flight", "in_progress", true),
		yours,
	)}}

	var got []string
	for _, r := range g.Rows(0) {
		if r.Kind == Work {
			got = append(got, r.Bead.ID)
		}
	}
	if len(got) != 4 || got[0] != "y" {
		t.Errorf("order = %v, want what needs a person first", got)
	}
	if got[1] != "a" || got[2] != "b" || got[3] != "c" {
		t.Errorf("order = %v, want yours, in flight, ready, blocked", got)
	}
}

// The reason a bead stopped is the whole point of looking at it, so it comes
// before the description -- which is what you already knew when you filed it.
func TestStoppedBeadShowsWhyBeforeWhat(t *testing.T) {
	b := bead("y", "waiting on you", "open", true)
	b.Labels = []string{beads.NeedsAttention}
	b.Notes = "A tender stopped short. blocked -- the schema has no column for this."
	b.Body = "the description written when it was filed"

	// Joined on spaces: the preview wraps, so asserting on a phrase that
	// survives a line break is testing the wrapper rather than the ordering.
	lines := strings.Join(strings.Fields(strings.Join(workDetail(&b, "hugel4", 60), " ")), " ")
	if !strings.Contains(lines, "no column for this") {
		t.Fatal("the reason it stopped is not shown")
	}
	if !strings.Contains(lines, "needs you") {
		t.Error("the bead does not say it is waiting on a person")
	}
	if strings.Index(lines, "no column for this") > strings.Index(lines, "written when it was filed") {
		t.Error("the description came before the reason")
	}
}

// A bead nobody handed back must not grow an empty reason section.
func TestOrdinaryBeadHasNoReasonSection(t *testing.T) {
	b := bead("x", "ordinary", "open", true)
	b.Body = "a description"
	if strings.Contains(strings.Join(workDetail(&b, "hugel4", 60), "\n"), "why it stopped") {
		t.Error("an empty reason section was rendered")
	}
}
