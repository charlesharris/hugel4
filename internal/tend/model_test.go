package tend

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/yield"
)

type fakeStore struct {
	entries  map[string]*pile.Entry
	commits  []string
	supersed [][2]string
}

func newFake(es ...*pile.Entry) *fakeStore {
	f := &fakeStore{entries: map[string]*pile.Entry{}}
	for _, e := range es {
		f.entries[e.ID] = e
	}
	return f
}

func (f *fakeStore) SetReview(id string, r pile.Review) (*pile.Entry, pile.Result, error) {
	e := f.entries[id]
	if e.Review == r {
		return e, pile.Unchanged, nil
	}
	c := *e
	c.Review = r
	return &c, pile.Updated, nil
}

func (f *fakeStore) SetStatus(id string, s pile.Status) (*pile.Entry, pile.Result, error) {
	e := f.entries[id]
	c := *e
	c.Status = s
	return &c, pile.Updated, nil
}

func (f *fakeStore) Supersede(oldID, newID string) (*pile.Entry, *pile.Entry, error) {
	f.supersed = append(f.supersed, [2]string{oldID, newID})
	o, n := *f.entries[oldID], *f.entries[newID]
	o.Status = pile.Superseded
	return &o, &n, nil
}

func (f *fakeStore) Commit(msg string) error {
	f.commits = append(f.commits, msg)
	return nil
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.Msg
		switch k {
		case "ctrl+c", "ctrl+d", "ctrl+u":
			msg = tea.KeyMsg{Type: map[string]tea.KeyType{
				"ctrl+c": tea.KeyCtrlC, "ctrl+d": tea.KeyCtrlD, "ctrl+u": tea.KeyCtrlU,
			}[k]}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func surface(t *testing.T, es []*pile.Entry, log []draws.Draw) (Model, *fakeStore) {
	t.Helper()
	f := newFake(es...)
	a := Gather(es, log, yield.SoilReport{Sessions: 4, Reached: 1, Draws: len(log)}, now.Add(-24*time.Hour))
	m := New(a, f, 25)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	return next.(Model), f
}

// The cursor must never land on a heading: a group label is not work.
func TestCursorSkipsHeadings(t *testing.T) {
	es := []*pile.Entry{ent("a", "drawn one", -60, "s1"), ent("b", "fresh one", -60, "s1")}
	m, _ := surface(t, es, []draws.Draw{drew(-30, "a")})

	if m.rows[m.cursor].Kind == Heading {
		t.Fatal("cursor started on a heading")
	}
	for i := 0; i < 10; i++ {
		m = press(m, "j")
		if m.rows[m.cursor].Kind == Heading {
			t.Fatalf("cursor landed on a heading after %d moves down", i+1)
		}
	}
	for i := 0; i < 10; i++ {
		m = press(m, "k")
		if m.rows[m.cursor].Kind == Heading {
			t.Fatalf("cursor landed on a heading after %d moves up", i+1)
		}
	}
}

// Judging advances. Holding the context for one entry, spending it, and being
// handed the next is the whole ergonomic argument for a surface.
func TestJudgingAdvancesAndSticks(t *testing.T) {
	es := []*pile.Entry{ent("a", "first", -60, "s1"), ent("b", "second", -60, "s1")}
	m, _ := surface(t, es, nil)

	first := m.current()
	m = press(m, "a")
	if first.Review != pile.Accepted {
		t.Errorf("entry review = %q, want accepted", first.Review)
	}
	if m.current() == first {
		t.Error("cursor did not advance after judging")
	}
	m = press(m, "r")
	if m.judged != 2 {
		t.Errorf("judged = %d, want 2", m.judged)
	}
}

// A mis-keyed verdict has to be undoable, or the surface is a hazard.
func TestUndoReturnsToUnreviewed(t *testing.T) {
	es := []*pile.Entry{ent("a", "first", -60, "s1")}
	m, _ := surface(t, es, nil)
	e := m.current()

	m = press(m, "a")
	if e.Review != pile.Accepted {
		t.Fatalf("review = %q", e.Review)
	}
	m = press(m, "k", "u")
	if e.Review != pile.Unreviewed {
		t.Errorf("review = %q after undo, want unreviewed", e.Review)
	}
}

// Superseding takes two keystrokes on two entries and no typed id.
func TestSupersedeIsTwoKeystrokes(t *testing.T) {
	es := []*pile.Entry{ent("a", "the old way", -60, "s1"), ent("b", "the new way", -60, "s1")}
	m, f := surface(t, es, nil)

	m = press(m, "g")
	armed := m.current().ID
	m = press(m, "s")
	if m.pendingSupersede == nil {
		t.Fatal("first s did not arm the supersede")
	}
	if !strings.Contains(m.status, "superseded by?") {
		t.Errorf("status = %q, want a prompt", m.status)
	}
	m = press(m, "j")
	replacement := m.current().ID
	m = press(m, "s")
	if len(f.supersed) != 1 || f.supersed[0] != [2]string{armed, replacement} {
		t.Fatalf("supersede calls = %v, want %s superseded by %s", f.supersed, armed, replacement)
	}
	if m.pendingSupersede != nil {
		t.Error("supersede stayed armed after completing")
	}
}

func TestSupersedeRefusesItself(t *testing.T) {
	es := []*pile.Entry{ent("a", "only entry", -60, "s1")}
	m, f := surface(t, es, nil)
	m = press(m, "s", "s")
	if len(f.supersed) != 0 {
		t.Error("an entry superseded itself")
	}
	if m.pendingSupersede != nil {
		t.Error("supersede stayed armed after being refused")
	}
}

// A sitting is one act of gardening and reads better as one commit.
func TestQuitCommitsOnce(t *testing.T) {
	es := []*pile.Entry{ent("a", "first", -60, "s1"), ent("b", "second", -60, "s1")}
	m, f := surface(t, es, nil)
	m = press(m, "a", "r", "q")
	if len(f.commits) != 1 {
		t.Fatalf("commits = %v, want exactly one", f.commits)
	}
	if !strings.Contains(f.commits[0], "2") {
		t.Errorf("commit message %q should say how many", f.commits[0])
	}
}

// Quitting without judging anything must leave no commit at all.
func TestQuitWithoutJudgingCommitsNothing(t *testing.T) {
	es := []*pile.Entry{ent("a", "first", -60, "s1")}
	m, f := surface(t, es, nil)
	press(m, "q")
	if len(f.commits) != 0 {
		t.Errorf("commits = %v, want none", f.commits)
	}
}

// The surface must render on an empty garden, a narrow terminal, and an entry
// with no body to preview -- all of which happen on day one.
func TestViewSurvivesEdges(t *testing.T) {
	empty, _ := surface(t, nil, nil)
	if !strings.Contains(empty.View(), "nothing") {
		t.Error("empty surface should say so")
	}

	es := []*pile.Entry{ent("a", strings.Repeat("a very long title ", 20), -60, "s1")}
	es[0].Body = ""
	m, _ := surface(t, es, nil)
	for _, w := range []int{40, 80, 200} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 10})
		if out := next.(Model).View(); out == "" {
			t.Errorf("width %d rendered nothing", w)
		}
	}
}

func TestViewShowsTheSurface(t *testing.T) {
	es := []*pile.Entry{
		ent("a", "Add hugel pile review: what a human decided about an entry", -60, "s1"),
		ent("b", "TDS-104/106: stop the site lying about how to open a tour", -120, "s2"),
	}
	es[0].Type, es[0].Bed = pile.Decision, "hugel4"
	es[0].Body = "The schema has had Accepted and Rejected since the pile was built, and " +
		"soil already reads all of it. None of it was reachable."
	es[1].Type, es[1].Bed = pile.Decision, "tourdesource"
	es[1].Body = "A changelog line, not knowledge."

	m, _ := surface(t, es, []draws.Draw{drew(-30, "a")})
	out := m.View()
	for _, want := range []string{"hugel tend", "DELIVERED", "NEW", "to judge", "reach 1/4"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q", want)
		}
	}
	t.Logf("\n%s", out)
}
