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
