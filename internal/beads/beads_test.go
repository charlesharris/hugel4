package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func repo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".beads", "config.yaml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRepoOfWalksUp(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	deep := filepath.Join(project, "internal", "thing")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	repo(t, project)

	if got := RepoOf(deep); got != project {
		t.Errorf("RepoOf(deep) = %q, want %q", got, project)
	}
	if got := RepoOf(project); got != project {
		t.Errorf("RepoOf(project) = %q, want %q", got, project)
	}
}

// bd keeps global state in ~/.beads -- shared-server settings and formulas, no
// issues. Matching on the directory alone made every bed on the machine report
// the home directory as its repository.
func TestRepoOfIgnoresABeadsDirWithNoProjectInIt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".beads", "shared-server"), 0o755); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "src", "somewhere")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RepoOf(work); got != "" {
		t.Errorf("RepoOf = %q, want nothing: that .beads holds no project", got)
	}
}

func TestRepoOfReturnsNothingWhenThereIsNoTracker(t *testing.T) {
	if got := RepoOf(t.TempDir()); got != "" {
		t.Errorf("RepoOf = %q, want empty", got)
	}
}

// Blocked is derived, not stored: bd keeps a blocked bead as open and works out
// the blockage from dependencies. Deriving it the same way keeps hugel from
// inventing a fourth status nobody else uses.
func TestBlockedIsDerivedFromReadiness(t *testing.T) {
	cases := []struct {
		name    string
		bead    Bead
		blocked bool
	}{
		{"open and ready", Bead{Status: "open", Ready: true}, false},
		{"open and not ready", Bead{Status: "open"}, true},
		{"in progress", Bead{Status: "in_progress"}, false},
		{"closed", Bead{Status: "closed"}, false},
	}
	for _, c := range cases {
		if got := c.bead.Blocked(); got != c.blocked {
			t.Errorf("%s: Blocked() = %v, want %v", c.name, got, c.blocked)
		}
	}
}

func TestCountsSeparateTheThreeStates(t *testing.T) {
	w := Work{Beads: []Bead{
		{Status: "open", Ready: true},
		{Status: "open", Ready: true},
		{Status: "in_progress"},
		{Status: "open"},
		{Status: "closed"},
	}}
	ready, active, blocked := w.Counts()
	if ready != 2 || active != 1 || blocked != 1 {
		t.Errorf("counts = %d ready, %d active, %d blocked; want 2/1/1", ready, active, blocked)
	}
}

// An in-progress bead also appears in bd ready. It must be counted as active
// once, not as both active and available for a tender to pick up.
func TestInProgressIsNotAlsoCountedReady(t *testing.T) {
	w := Work{Beads: []Bead{{Status: "in_progress", Ready: true}}}
	ready, active, _ := w.Counts()
	if ready != 0 || active != 1 {
		t.Errorf("counts = %d ready, %d active; want 0/1", ready, active)
	}
}

// The fields hugel reads have to survive bd's real output shape.
func TestBeadDecodesFromBdJSON(t *testing.T) {
	raw := `[{"id":"hugel4-sd7.1","title":"a bridge to bd","status":"in_progress",
	  "priority":1,"issue_type":"task","updated_at":"2026-08-24T14:52:22Z",
	  "dependencies":[{"issue_id":"x","depends_on_id":"y","type":"blocks"}]}]`
	var got []Bead
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("decoded %d beads", len(got))
	}
	b := got[0]
	if b.ID != "hugel4-sd7.1" || b.Type != "task" || b.Status != "in_progress" || b.Priority != 1 {
		t.Errorf("decoded %+v", b)
	}
	if b.Updated.IsZero() {
		t.Error("updated_at did not decode")
	}
}

func ready(id, bed string, prio int, kind string, isReady bool) Bead {
	return Bead{ID: id, Title: id, Priority: prio, Type: kind, Status: "open", Ready: isReady}
}

// Highest priority wins wherever it lives: a P0 in a quiet bed goes before a P3
// in a busy one, or a bed with a long backlog starves every other bed.
func TestQueueTakesPriorityAcrossBeds(t *testing.T) {
	busy := &Work{Bed: "tourdesource", Beads: []Bead{
		ready("t-1", "", 2, "task", true), ready("t-2", "", 3, "task", true),
	}}
	quiet := &Work{Bed: "hugel4", Beads: []Bead{ready("h-1", "", 0, "task", true)}}

	q := Queue([]*Work{busy, quiet}, "", nil)
	if len(q) != 3 {
		t.Fatalf("queued %d, want 3", len(q))
	}
	if q[0].Bead.ID != "h-1" {
		t.Errorf("first = %s, want the P0 from the quiet bed", q[0].Bead.ID)
	}
}

// Two dispatches must see the same queue, or they race for different beads and
// the ordering means nothing.
func TestQueueIsStable(t *testing.T) {
	w := &Work{Bed: "b", Beads: []Bead{
		ready("c", "", 1, "task", true), ready("a", "", 1, "task", true), ready("b", "", 1, "task", true),
	}}
	first := Queue([]*Work{w}, "", nil)
	for i := 0; i < 5; i++ {
		again := Queue([]*Work{w}, "", nil)
		for j := range first {
			if first[j].Bead.ID != again[j].Bead.ID {
				t.Fatalf("queue changed between runs: %s vs %s", first[j].Bead.ID, again[j].Bead.ID)
			}
		}
	}
}

// An epic is a container for work rather than work. A tender handed one would
// try to do all of it at once.
func TestQueueExcludesWhatCannotBeStarted(t *testing.T) {
	w := &Work{Bed: "b", Beads: []Bead{
		ready("epic", "", 0, "epic", true),
		ready("blocked", "", 0, "task", false),
		ready("claimed", "", 0, "task", true),
		ready("good", "", 1, "task", true),
	}}
	w.Beads[2].Status = "in_progress"

	q := Queue([]*Work{w}, "", nil)
	if len(q) != 1 || q[0].Bead.ID != "good" {
		var got []string
		for _, r := range q {
			got = append(got, r.Bead.ID)
		}
		t.Errorf("queue = %v, want only the startable one", got)
	}
}

// A bead already tended once must not be picked up again by a later dispatch:
// two agents on one branch is the collision the pool exists to avoid.
func TestQueueSkipsWhatIsAlreadyTended(t *testing.T) {
	w := &Work{Bed: "b", Beads: []Bead{
		ready("done-once", "", 0, "task", true), ready("fresh", "", 1, "task", true),
	}}
	q := Queue([]*Work{w}, "", func(id string) bool { return id == "done-once" })
	if len(q) != 1 || q[0].Bead.ID != "fresh" {
		t.Errorf("queue = %+v, want the untended bead only", q)
	}
}

func TestQueueRestrictsToABed(t *testing.T) {
	a := &Work{Bed: "a", Beads: []Bead{ready("a-1", "", 0, "task", true)}}
	b := &Work{Bed: "b", Beads: []Bead{ready("b-1", "", 0, "task", true)}}
	q := Queue([]*Work{a, b}, "b", nil)
	if len(q) != 1 || q[0].Bead.ID != "b-1" {
		t.Errorf("queue = %+v, want only bed b", q)
	}
}

// A bead handed back to a person is not available to a tender, however ready
// bd says it is. Without this the feedback edge is a loop: hand back, pick up,
// fail the same way, hand back.
func TestQueueLeavesWorkThatNeedsAPerson(t *testing.T) {
	w := &Work{Bed: "b", Beads: []Bead{
		ready("waiting", "", 0, "task", true),
		ready("free", "", 1, "task", true),
	}}
	w.Beads[0].Labels = []string{NeedsAttention}

	q := Queue([]*Work{w}, "", nil)
	if len(q) != 1 || q[0].Bead.ID != "free" {
		var got []string
		for _, r := range q {
			got = append(got, r.Bead.ID)
		}
		t.Errorf("queue = %v, want the marked bead left out", got)
	}
}

func TestLabeledIsCaseInsensitive(t *testing.T) {
	b := Bead{Labels: []string{"Needs-Attention"}}
	if !b.Labeled(NeedsAttention) {
		t.Error("a label differing only in case was not recognised")
	}
	if b.Labeled("spike") {
		t.Error("matched a label it does not carry")
	}
}
