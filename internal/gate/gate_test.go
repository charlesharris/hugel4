package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/tender"
)

// A reviewer that rambled, crashed, or answered a different question has not
// approved anything. Treating an unreadable review as a pass makes the review
// decorative, which is worse than having none.
func TestAnUnreadableReviewIsARefusal(t *testing.T) {
	for _, review := range []string{
		"", "looks fine to me!", "## Findings\nnothing much",
		"## Verdict\n\n", "I would probably merge this",
	} {
		if v, _ := ReadVerdict(review); v != Reject {
			t.Errorf("ReadVerdict(%q) = %q, want reject", review, v)
		}
	}
}

func TestVerdictsAreRead(t *testing.T) {
	cases := map[string]Verdict{
		"## Verdict\npass\nit does what the bead asked":           Pass,
		"## Verdict\nchanges-needed\nthe error path is untested":  Changes,
		"## Verdict\nreject\nwrong solution to the wrong problem": Reject,
		"## Verdict\nPASS\nshouting is still an answer":           Pass,
		"# Review\n\n## Verdict\n\n  pass  \nafter some preamble": Pass,
	}
	for review, want := range cases {
		if got, _ := ReadVerdict(review); got != want {
			t.Errorf("ReadVerdict(%q) = %q, want %q", review, got, want)
		}
	}
	if _, why := ReadVerdict("## Verdict\npass\nit does what the bead asked"); why != "it does what the bead asked" {
		t.Errorf("why = %q", why)
	}
}

// A verdict elsewhere in the document must not be mistaken for the answer: a
// findings section that mentions rejecting something is not a rejection.
func TestOnlyTheVerdictSectionDecides(t *testing.T) {
	review := `## Verdict
pass
sound

## Findings
I considered whether to reject the naming but it matches the package.`
	if v, _ := ReadVerdict(review); v != Pass {
		t.Errorf("verdict = %q, want pass", v)
	}
}

// A tender that reported itself blocked has said the work is not done. The gate
// believes it rather than reading the diff and deciding otherwise.
func TestOutcomeIsReadFromTheTendersOwnAccount(t *testing.T) {
	cases := map[string]string{
		"## Outcome\ndone -- all of it":      "done",
		"## Outcome\n\nblocked, no database": "blocked",
		"## Outcome\npartial":                "partial",
		"## Outcome\nit went okay I think":   "unstated",
		"no headings at all":                 "unstated",
	}
	for result, want := range cases {
		if got := outcomeIn(result); got != want {
			t.Errorf("outcomeIn(%q) = %q, want %q", result, got, want)
		}
	}
}

// hugel cannot gate a project whose tests it cannot find, and must say so
// rather than pass work nothing checked.
func TestDiscoverTestNeverGuessesItsWayToAPass(t *testing.T) {
	empty := t.TempDir()
	if got := DiscoverTest(empty); got != "" {
		t.Errorf("DiscoverTest on an empty dir = %q, want nothing", got)
	}

	goish := t.TempDir()
	os.WriteFile(filepath.Join(goish, "go.mod"), []byte("module x\n"), 0o644)
	if got := DiscoverTest(goish); got != "go test ./..." {
		t.Errorf("DiscoverTest = %q", got)
	}

	// A Makefile wins, being what the project itself says to run.
	made := t.TempDir()
	os.WriteFile(filepath.Join(made, "go.mod"), []byte("module x\n"), 0o644)
	os.WriteFile(filepath.Join(made, "Makefile"), []byte("build:\n\tgo build\n\ntest:\n\tgo test ./...\n"), 0o644)
	if got := DiscoverTest(made); got != "make test" {
		t.Errorf("DiscoverTest = %q, want the project's own target", got)
	}

	// A Makefile without a test target is not a test command.
	noTest := t.TempDir()
	os.WriteFile(filepath.Join(noTest, "Makefile"), []byte("build:\n\tgo build\n"), 0o644)
	if got := DiscoverTest(noTest); got != "" {
		t.Errorf("DiscoverTest = %q, want nothing", got)
	}
}

// The gate refuses before it starts an agent when the tender never finished.
func TestGateStopsAtAnUnfinishedTender(t *testing.T) {
	dir := t.TempDir()
	td := tender.Tender{Bead: "x-1", Worktree: filepath.Join(dir, "bed")}
	rep, err := Run(Options{Tender: td})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed || rep.Reached != StageResult {
		t.Errorf("report = %+v, want a refusal at the result stage", rep)
	}
	if !strings.Contains(rep.Why, "not finished") {
		t.Errorf("why = %q", rep.Why)
	}
}

func TestGateStopsWhenTheTenderReportsBlocked(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bed"), 0o755)
	td := tender.Tender{Bead: "x-1", Worktree: filepath.Join(dir, "bed")}
	os.WriteFile(td.ResultPath(), []byte("## Outcome\nblocked -- no test database\n"), 0o644)

	rep, _ := Run(Options{Tender: td})
	if rep.Passed || !strings.Contains(rep.Why, "blocked") {
		t.Errorf("report = %+v, want a refusal naming the tender's own outcome", rep)
	}
}

// Work that cannot be tested cannot be landed.
func TestGateStopsWhenTestsCannotBeFound(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bed"), 0o755)
	td := tender.Tender{Bead: "x-1", Worktree: filepath.Join(dir, "bed")}
	os.WriteFile(td.ResultPath(), []byte("## Outcome\ndone\n"), 0o644)

	rep, _ := Run(Options{Tender: td})
	if rep.Passed || rep.Reached != StageTest {
		t.Errorf("report = %+v, want a refusal at the test stage", rep)
	}
}

// Failing tests stop the gate before it spends an agent on a review.
func TestFailingTestsStopBeforeTheReview(t *testing.T) {
	dir := t.TempDir()
	work := filepath.Join(dir, "bed")
	os.MkdirAll(work, 0o755)
	td := tender.Tender{Bead: "x-1", Worktree: work}
	os.WriteFile(td.ResultPath(), []byte("## Outcome\ndone\n"), 0o644)

	rep, _ := Run(Options{Tender: td, Test: "echo 'FAIL: everything' && exit 1"})
	if rep.Passed || rep.Reached != StageTest {
		t.Errorf("report = %+v, want a refusal at the test stage", rep)
	}
	if !strings.Contains(rep.Stages[len(rep.Stages)-1].Detail, "FAIL") {
		t.Error("the refusal did not keep the test output it rests on")
	}
}

// The review brief must forbid the reviewer repairing what it finds: naming a
// problem and fixing it are two jobs, and doing both means nobody reviewed the
// repair.
func TestReviewBriefForbidsRepair(t *testing.T) {
	td := tender.Tender{Bead: "x-1", Title: "a bead", Branch: "hugel/x-1", Worktree: "/w/bed"}
	b := ReviewBrief(td, "", "ok")
	for _, want := range []string{"Change nothing", "do not fix what you find", "Read the code", "## Verdict"} {
		if !strings.Contains(b, want) {
			t.Errorf("review brief is missing %q", want)
		}
	}
	if !strings.Contains(b, "You did not write this code") {
		t.Error("the reviewer is not told it is a separate pair of eyes")
	}
}

// A dry run does the work that finds problems and stops before the work that
// cannot be undone. The first version signalled it by leaving the base branch
// empty, and merged against a ref named "".
func TestDryRunStopsAfterTheReview(t *testing.T) {
	dir := t.TempDir()
	work := filepath.Join(dir, "bed")
	os.MkdirAll(work, 0o755)
	td := tender.Tender{Bead: "x-1", Worktree: work}
	os.WriteFile(td.ResultPath(), []byte("## Outcome\ndone\n"), 0o644)

	rep, err := Run(Options{
		Tender: td, Test: "true", DryRun: true, Into: "main", Remote: "origin",
		Wait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range rep.Stages {
		if s.Stage == StageMerge || s.Stage == StagePush || s.Stage == StageClose {
			t.Errorf("a dry run reached %s", s.Stage)
		}
	}
}

// The criteria are what turns "is this good work" -- which an agent asked about
// another agent's code will tend to answer yes -- into a question with an
// answer.
func TestReviewBriefCarriesTheAcceptanceCriteria(t *testing.T) {
	td := tender.Tender{Bead: "x-1", Title: "a bead", Branch: "hugel/x-1", Worktree: "/w/bed"}
	b := ReviewBrief(td, "The draw is recorded, and a failed write does not fail the caller.", "ok")

	if !strings.Contains(b, "a failed write does not fail the caller") {
		t.Error("the criteria did not reach the brief")
	}
	if !strings.Contains(b, "## Criteria") {
		t.Error("the reviewer is not asked to answer criterion by criterion")
	}
	if !strings.Contains(b, "Say pass only if every stated criterion is met") {
		t.Error("an unmet criterion is not stated to block a pass")
	}
}

// A bead with no criteria must say so. Silence leaves the reviewer to guess
// whether criteria existed and were withheld, and a reviewer that invents a
// standard is back to judging whether the work is nice.
func TestReviewBriefSaysWhenThereAreNoCriteria(t *testing.T) {
	td := tender.Tender{Bead: "x-1", Title: "a bead", Branch: "hugel/x-1", Worktree: "/w/bed"}
	b := ReviewBrief(td, "   \n  ", "ok")

	if !strings.Contains(b, "states none") {
		t.Error("a bead without criteria does not say so")
	}
	if !strings.Contains(b, "shipped without criteria") {
		t.Error("the reviewer is not asked to report the absence")
	}
}

// Criteria are a floor. Work can meet every one and still be wrong, and a
// reviewer told only to tick boxes stops looking at the code.
func TestReviewBriefKeepsTheReviewerLookingPastTheCriteria(t *testing.T) {
	td := tender.Tender{Bead: "x-1", Title: "a bead", Branch: "hugel/x-1", Worktree: "/w/bed"}
	b := ReviewBrief(td, "it compiles", "ok")

	if !strings.Contains(b, "floor and not a ceiling") {
		t.Error("the brief presents the criteria as sufficient")
	}
	if !strings.Contains(b, "Read the code") {
		t.Error("the brief stopped telling the reviewer to read the code")
	}
}
