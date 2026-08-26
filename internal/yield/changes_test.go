package yield

import (
	"testing"
	"time"

	"github.com/charris/hugel/internal/transcript"
)

func inWorktree(bed, cwd string, endMin int, rs ...transcript.Request) *transcript.Session {
	return &transcript.Session{
		Bed: bed, CWD: cwd,
		Start:    base.Add(time.Duration(endMin-10) * time.Minute),
		End:      base.Add(time.Duration(endMin) * time.Minute),
		Requests: rs,
	}
}

func spend(dollars float64) transcript.Request {
	// opus-5 output is $25/MTok, so tokens are chosen to make the arithmetic
	// legible in the assertions rather than realistic.
	return transcript.Request{
		Model: "claude-opus-5",
		Usage: transcript.Usage{Output: int(dollars / 25 * 1e6)},
	}
}

// The number the design constraint names, averaged only over changes that
// landed: dividing by abandoned work would make giving up look like efficiency.
func TestPerChangeAveragesOnlyWhatLanded(t *testing.T) {
	attempts := []Attempt{
		{Bead: "a-1", Bed: "b", Worktree: "/w/a-1/b", Landed: true},
		{Bead: "a-2", Bed: "b", Worktree: "/w/a-2/b", Landed: true},
		{Bead: "a-3", Bed: "b", Worktree: "/w/a-3/b", Landed: false},
	}
	sessions := []*transcript.Session{
		inWorktree("b", "/w/a-1/b", 0, spend(10)),
		inWorktree("b", "/w/a-2/b", 0, spend(30)),
		inWorktree("b", "/w/a-3/b", 0, spend(60)),
	}

	rep := Changes(sessions, attempts, Filter{})
	if rep.Landed != 2 || rep.Unlanded != 1 {
		t.Fatalf("landed/unlanded = %d/%d, want 2/1", rep.Landed, rep.Unlanded)
	}
	if got := rep.PerChange(); got < 19.9 || got > 20.1 {
		t.Errorf("PerChange = %.2f, want ~20 (40 over two landed)", got)
	}
	if got := rep.UnlandedCost; got < 59.9 || got > 60.1 {
		t.Errorf("UnlandedCost = %.2f, want ~60", got)
	}
}

// A bead tended twice cost twice, and averaging that away hides the thing most
// worth seeing.
func TestAttemptsAreCountedPerRunNotPerBead(t *testing.T) {
	attempts := []Attempt{
		{Bead: "a-1", Bed: "b", Worktree: "/w/a-1/b", Landed: false},
		{Bead: "a-1", Bed: "b", Worktree: "/w/a-1.1/b", Landed: true},
	}
	sessions := []*transcript.Session{
		inWorktree("b", "/w/a-1/b", 0, spend(10)),
		inWorktree("b", "/w/a-1.1/b", 0, spend(15)),
	}

	rep := Changes(sessions, attempts, Filter{})
	if len(rep.Changes) != 1 {
		t.Fatalf("changes = %d, want the two attempts folded into one bead", len(rep.Changes))
	}
	c := rep.Changes[0]
	if c.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", c.Attempts)
	}
	if !c.Landed {
		t.Error("a bead tended twice and then landed did land")
	}
	if got := c.Total(); got < 24.9 || got > 25.1 {
		t.Errorf("cost = %.2f, want both attempts counted", got)
	}
}

// A worktree at .../hugel4 must not claim sessions from .../hugel4-scratch.
func TestAttributionDoesNotClaimNeighbouringPaths(t *testing.T) {
	attempts := []Attempt{{Bead: "a-1", Bed: "b", Worktree: "/w/a-1/hugel4", Landed: true}}
	sessions := []*transcript.Session{
		inWorktree("b", "/w/a-1/hugel4", 0, spend(10)),
		inWorktree("b", "/w/a-1/hugel4-scratch", 0, spend(90)),
		inWorktree("b", "/w/a-1/hugel4/internal/deep", 0, spend(5)),
	}

	rep := Changes(sessions, attempts, Filter{})
	if got := rep.Changes[0].Total(); got < 14.9 || got > 15.1 {
		t.Errorf("cost = %.2f, want 15: the worktree and a directory inside it, not the neighbour", got)
	}
	if rep.Changes[0].Sessions != 2 {
		t.Errorf("sessions = %d, want 2", rep.Changes[0].Sessions)
	}
}

// Most spend is interactive and cannot be attributed to a bead this way.
// Reporting the gap is the difference between a measurement and a misleading
// one -- without it, a report covering 5% of spend reads as if it covered all.
func TestCoverageReportsWhatIsLeftOut(t *testing.T) {
	attempts := []Attempt{{Bead: "a-1", Bed: "b", Worktree: "/w/a-1/b", Landed: true}}
	sessions := []*transcript.Session{
		inWorktree("b", "/w/a-1/b", 0, spend(10)),
		inWorktree("b", "/home/me/src/b", 0, spend(90)),
	}

	rep := Changes(sessions, attempts, Filter{})
	if got := rep.Total; got < 99.9 || got > 100.1 {
		t.Errorf("Total = %.2f, want all spend in the window", got)
	}
	if got := rep.Coverage(); got < 0.09 || got > 0.11 {
		t.Errorf("Coverage = %.2f, want ~0.1", got)
	}
}

// With nothing landed the average must not read as zero, which would say a
// change costs nothing.
func TestNothingLandedIsNotFree(t *testing.T) {
	attempts := []Attempt{{Bead: "a-1", Bed: "b", Worktree: "/w/a-1/b"}}
	sessions := []*transcript.Session{inWorktree("b", "/w/a-1/b", 0, spend(10))}

	rep := Changes(sessions, attempts, Filter{})
	if rep.Landed != 0 || rep.PerChange() != 0 {
		t.Errorf("report = %+v, want no landed changes and no average", rep)
	}
	if rep.UnlandedCost < 9.9 {
		t.Error("the spend that landed nothing was not counted")
	}
}

func TestChangesRespectTheWindow(t *testing.T) {
	attempts := []Attempt{{Bead: "a-1", Bed: "b", Worktree: "/w/a-1/b", Landed: true}}
	sessions := []*transcript.Session{
		inWorktree("b", "/w/a-1/b", -600, spend(50)),
		inWorktree("b", "/w/a-1/b", 0, spend(10)),
	}

	rep := Changes(sessions, attempts, Filter{Since: base.Add(-time.Hour)})
	if got := rep.Changes[0].Total(); got < 9.9 || got > 10.1 {
		t.Errorf("cost = %.2f, want only the session inside the window", got)
	}
}
