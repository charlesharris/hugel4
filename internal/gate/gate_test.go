package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/events"
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

// A spike lands no diff by design, so every stage below the first would read
// its success as failure. The gate has to decline before it starts measuring,
// and say why in words that name what a spike is.
func TestTheGateRefusesASpikeBeforeItLooksForWork(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bed"), 0o755)
	td := tender.Tender{Bead: "x-1", Worktree: filepath.Join(dir, "bed"), Spike: true}
	// A finished, done spike: everything a tender needs to be gated, so what
	// stops it can only be that it is a spike.
	os.WriteFile(td.ResultPath(), []byte("## Outcome\ndone -- found the answer\n"), 0o644)

	rep, err := Run(Options{Tender: td})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed {
		t.Error("a spike passed the gate; it has nothing to land")
	}
	if !rep.Refused {
		t.Error("refusing to judge a spike was recorded as judging it and saying no")
	}
	if rep.Reached != StageKind {
		t.Errorf("reached %q, want the gate to stop at %q before testing anything", rep.Reached, StageKind)
	}
	if !strings.Contains(rep.Why, "spike") {
		t.Errorf("why = %q, want a reason naming what a spike is", rep.Why)
	}
	// The result file said done. If the gate got as far as reading it, it was
	// measuring a spike against a tender's yardstick.
	for _, st := range rep.Stages {
		if st.Stage != StageKind {
			t.Errorf("gate reached stage %q on a spike", st.Stage)
		}
	}
}

// An ordinary tender must still be judged, or the guard above has turned the
// gate off for everything.
func TestTheGateStillJudgesATender(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "bed"), 0o755)
	td := tender.Tender{Bead: "x-1", Worktree: filepath.Join(dir, "bed")}
	os.WriteFile(td.ResultPath(), []byte("## Outcome\ndone -- it works\n"), 0o644)

	rep, _ := Run(Options{Tender: td})
	if rep.Refused {
		t.Error("an ordinary tender was refused as though it were a spike")
	}
	if rep.Reached == StageKind {
		t.Error("a tender stopped at the spike guard")
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

// The acceptance this bead was filed against: a refusal has to be
// reconstructable from events alone, without the worktree, the terminal it ran
// in, or anyone's memory of three weeks ago.
func TestARefusalIsReconstructableFromEventsAlone(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	dir := t.TempDir()
	work := filepath.Join(dir, "bed")
	os.MkdirAll(work, 0o755)
	td := tender.Tender{
		Bead: "x-1", Bed: "somebed", Worktree: work,
		Branch: "hugel/x-1", Started: time.Now().Add(-time.Minute),
	}
	os.WriteFile(td.ResultPath(), []byte("## Outcome\ndone\n"), 0o644)

	rep, err := Run(Options{
		Tender: td, Test: "echo 'FAIL: the thing is broken' && exit 1",
		Into: "main", Remote: "origin",
	})
	if err != nil || rep.Passed {
		t.Fatalf("expected a refusal, got %+v %v", rep, err)
	}

	log, err := events.Load()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]events.Event{}
	for _, e := range log {
		byName[e.Name] = e
		if e.Bead != "x-1" || e.Bed != "somebed" {
			t.Errorf("%s lost its correlation keys: %+v", e.Name, e)
		}
	}

	// Which stage refused, and on what evidence.
	stage, ok := byName["gate.stage"]
	if !ok || stage.Outcome != "failed" {
		t.Fatalf("no failed stage event: %+v", byName)
	}
	if d, _ := stage.Fields["detail"].(string); !strings.Contains(d, "the thing is broken") {
		t.Errorf("the stage event did not carry the evidence it refused on: %v", stage.Fields["detail"])
	}

	// What was run, and against what.
	test, ok := byName["gate.test"]
	if !ok || test.Outcome != "failed" || test.Fields["on"] != "branch" {
		t.Errorf("gate.test = %+v, want a failed run on the branch", test)
	}

	// One findable row for the whole run, so the question does not require
	// walking the stages.
	run, ok := byName["gate.run"]
	if !ok || run.Outcome != "failed" {
		t.Fatalf("no gate.run event: %+v", byName)
	}
	if run.Fields["reached"] != string(StageTest) {
		t.Errorf("gate.run reached = %v, want the stage it stopped at", run.Fields["reached"])
	}
	if why, _ := run.Fields["why"].(string); !strings.Contains(why, "tests fail") {
		t.Errorf("gate.run why = %v", why)
	}
	// The tender's own extent, so a done tender's life closes here rather than
	// being unrecorded until someone reads its worktree.
	if ms, _ := run.Fields["tender_duration_ms"].(float64); ms < 1000 {
		t.Errorf("tender_duration_ms = %v, want the tender's extent", ms)
	}
}

// A gate that never started an agent must not claim it reviewed anything.
func TestNoReviewEventWhenTheGateStopsFirst(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	dir := t.TempDir()
	work := filepath.Join(dir, "bed")
	os.MkdirAll(work, 0o755)
	td := tender.Tender{Bead: "x-1", Worktree: work}
	os.WriteFile(td.ResultPath(), []byte("## Outcome\nblocked\n"), 0o644)

	Run(Options{Tender: td, Test: "true"})

	log, _ := events.Load()
	for _, e := range log {
		if e.Name == "gate.review" || e.Name == "gate.land" {
			t.Errorf("emitted %s after refusing at the result stage", e.Name)
		}
	}
}
