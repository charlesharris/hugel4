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
	ready, active, blocked := g.Totals()
	if ready != 1 || active != 1 || blocked != 1 {
		t.Errorf("totals = %d/%d/%d, want 1/1/1", ready, active, blocked)
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
