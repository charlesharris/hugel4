package tender

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/events"
)

func fixedNow(t *testing.T) time.Time {
	t.Helper()
	when := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	old := now
	now = func() time.Time { return when }
	t.Cleanup(func() { now = old })
	return when
}

func sample(dir string) (Options, Tender) {
	o := Options{
		Bead: beads.Bead{ID: "hugel4-sd7.4", Title: "Tender: run one ready bead", Body: "the smallest dispatch"},
		Bed:  "hugel4", Repo: "/src/hugel4",
	}
	t := Tender{
		Bead: o.Bead.ID, Bed: o.Bed, Title: o.Bead.Title, Repo: o.Repo,
		Worktree: worktreeIn(dir, o.Bed), Branch: "hugel/" + o.Bead.ID,
		Session: sessionName(o.Bead.ID),
	}
	return o, t
}

// A harness names a session's project by the basename of its working directory,
// and that name is what hugel calls a bed. A worktree named for the bead would
// file the tender's whole transcript under a bed that is not the project --
// one new bed per bead, and the accounting scattered across all of them.
func TestWorktreeIsNamedForTheBedNotTheBead(t *testing.T) {
	got := worktreeIn("/garden/tenders/hugel4-sd7.4", "hugel4")
	if filepath.Base(got) != "hugel4" {
		t.Errorf("worktree basename = %q, want the bed name", filepath.Base(got))
	}
	if !strings.Contains(got, "hugel4-sd7.4") {
		t.Errorf("worktree %q lost the bead it belongs to", got)
	}
}

// tmux addresses panes within a window using dots, and bd's hierarchical ids
// are full of them.
func TestSessionNameSurvivesHierarchicalIDs(t *testing.T) {
	got := sessionName("hugel4-sd7.4")
	if strings.Contains(got, ".") {
		t.Errorf("session name %q keeps a dot tmux will read as a pane", got)
	}
	if !strings.HasPrefix(got, "hugel-") {
		t.Errorf("session name %q is not identifiable as hugel's", got)
	}
}

// Paperwork sits beside the worktree, never inside it: a tender's brief showing
// up as an untracked file in the project is a change it did not make.
func TestBriefAndResultLiveOutsideTheWorktree(t *testing.T) {
	_, td := sample("/garden/tenders/hugel4-sd7.4")
	for _, p := range []string{td.BriefPath(), td.ResultPath()} {
		if strings.HasPrefix(p, td.Worktree+string(filepath.Separator)) {
			t.Errorf("%q is inside the worktree", p)
		}
	}
}

// The brief has to say what not to do as clearly as what to do. A tender that
// pushes, or closes its own bead, has taken a decision the review exists to
// make.
func TestBriefForbidsTheReviewersDecisions(t *testing.T) {
	o, td := sample("/garden/tenders/hugel4-sd7.4")
	b := Brief(o, td)
	for _, want := range []string{
		"hugel4-sd7.4", "Tender: run one ready bead", "the smallest dispatch",
		td.Worktree, td.Branch, td.ResultPath(),
		"Do not push", "Do not close the bead", "Run the project's tests",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("brief is missing %q", want)
		}
	}
}

func TestBriefCarriesAnExtraNote(t *testing.T) {
	o, td := sample("/garden/tenders/x")
	o.Extra = "the pooler must stay in session mode"
	if !strings.Contains(Brief(o, td), "the pooler must stay in session mode") {
		t.Error("the note did not reach the brief")
	}
}

func TestSaveAndListRoundTrip(t *testing.T) {
	garden := t.TempDir()
	t.Setenv("HUGEL_HOME", garden)
	when := fixedNow(t)

	dir, err := Dir("hugel4-sd7.4")
	if err != nil {
		t.Fatal(err)
	}
	_, td := sample(dir)
	td.Started = when
	if err := td.save(); err != nil {
		t.Fatal(err)
	}

	all, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("listed %d tenders, want 1", len(all))
	}
	if all[0].Bead != td.Bead || all[0].Worktree != td.Worktree || !all[0].Started.Equal(when) {
		t.Errorf("round trip lost something: %+v", all[0])
	}

	got, err := Load("hugel4-sd7.4")
	if err != nil || got.Branch != td.Branch {
		t.Errorf("Load = %+v, %v", got, err)
	}
	if _, err := Load("nope"); err == nil {
		t.Error("Load found a tender that does not exist")
	}
}

// A tender that never ran leaves nothing to list, which is not an error.
func TestListOnAFreshGarden(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	all, err := List()
	if err != nil || len(all) != 0 {
		t.Errorf("List = %v, %v; want empty and no error", all, err)
	}
}

// Writing the result is how the garden learns a tender has finished, so that
// file and nothing else decides it.
func TestDoneFollowsTheResultFile(t *testing.T) {
	garden := t.TempDir()
	t.Setenv("HUGEL_HOME", garden)
	dir, _ := Dir("hugel4-sd7.4")
	_, td := sample(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if td.Done() {
		t.Fatal("done before anything was written")
	}
	if err := os.WriteFile(td.ResultPath(), []byte("## Outcome\ndone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !td.Done() {
		t.Error("result written and still not done")
	}
	if td.State() != "finished" {
		t.Errorf("state = %q, want finished", td.State())
	}
}

// For a tender the pull argument inverts: nobody is in the session to notice
// that prior knowledge would help, and the bead is already a query.
func TestBriefCarriesSoilWithItsCaveat(t *testing.T) {
	o, td := sample("/garden/tenders/x")
	o.Soil = "## an earlier decision · decision · id abc12345\nthe pooler must stay in session mode"
	b := Brief(o, td)

	if !strings.Contains(b, "the pooler must stay in session mode") {
		t.Error("soil did not reach the brief")
	}
	if !strings.Contains(b, "What the garden already knows") {
		t.Error("soil arrived without a heading saying what it is")
	}
	// A tender cannot ask whether an entry is still true, so the brief has to
	// say what soil is worth before the agent acts on it.
	for _, caveat := range []string{"survey", "unreviewed", "pile show"} {
		if !strings.Contains(b, caveat) {
			t.Errorf("brief presents soil without the caveat %q", caveat)
		}
	}
}

// A bead with nothing in the pile about it must produce a brief with no empty
// section pretending otherwise.
func TestBriefWithoutSoilHasNoSoilSection(t *testing.T) {
	o, td := sample("/garden/tenders/x")
	if strings.Contains(Brief(o, td), "What the garden already knows") {
		t.Error("an empty soil section was rendered")
	}
}

// The tender is shown the same criteria the review will be answered against.
// Without that the two disagree about a standard only one of them was told.
func TestBriefCarriesTheAcceptanceCriteria(t *testing.T) {
	o, td := sample("/garden/tenders/x")
	o.Bead.Accept = "A spike leaves no diff, and its findings reach the pile."
	b := Brief(o, td)

	if !strings.Contains(b, "A spike leaves no diff") {
		t.Error("the criteria did not reach the tender's brief")
	}
	if !strings.Contains(b, "Done when") {
		t.Error("the criteria arrived without saying what they are")
	}
	if !strings.Contains(b, "answered against exactly this") {
		t.Error("the tender is not told the review uses the same criteria")
	}
}

func TestBriefWithoutCriteriaHasNoDoneWhenSection(t *testing.T) {
	o, td := sample("/garden/tenders/x")
	if strings.Contains(Brief(o, td), "Done when") {
		t.Error("an empty criteria section was rendered")
	}
}

// A tender that would not say how it went has not said it went well. Reading a
// missing or vague outcome as "done" would let the gate merge work nobody
// claimed was finished.
func TestOutcomeIsReadFromTheTendersOwnAccount(t *testing.T) {
	cases := map[string]string{
		"## Outcome\ndone -- all of it":      "done",
		"## Outcome\n\nblocked, no database": "blocked",
		"## Outcome\npartial":                "partial",
		"## Outcome\nit went okay I think":   "unstated",
		"no headings at all":                 "unstated",
	}
	dir := t.TempDir()
	td := Tender{Bead: "x-1", Worktree: filepath.Join(dir, "bed")}
	os.MkdirAll(td.Worktree, 0o755)
	for result, want := range cases {
		os.WriteFile(td.ResultPath(), []byte(result), 0o644)
		if got := td.Outcome(); got != want {
			t.Errorf("Outcome(%q) = %q, want %q", result, got, want)
		}
	}
}

// The reason handed back to a bead is the tender's own words, because a person
// reading the bead later needs what it said rather than a paraphrase.
func TestReasonKeepsWhatTheTenderSaid(t *testing.T) {
	dir := t.TempDir()
	td := Tender{Bead: "x-1", Worktree: filepath.Join(dir, "bed")}
	os.MkdirAll(td.Worktree, 0o755)
	os.WriteFile(td.ResultPath(), []byte(
		"## Outcome\nblocked -- the bead asks for two incompatible things\n\n"+
			"## What changed\nnothing\n\n"+
			"## For the reviewer\nthe schema has no column for this\n"), 0o644)

	got := td.Reason()
	if !strings.Contains(got, "two incompatible things") {
		t.Errorf("Reason() = %q, want the outcome", got)
	}
	if !strings.Contains(got, "no column for this") {
		t.Errorf("Reason() = %q, want what it left for a reviewer", got)
	}
}

// A bead handed back for correction has to be workable again, or "while that
// mark stands" means "forever".
func TestArchiveMakesRoomForASecondAttempt(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	dir, _ := Dir("x-1")
	if err := os.MkdirAll(filepath.Join(dir, "bed"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "brief.md"), []byte("first attempt"), 0o644)

	if err := Archive("x-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the directory was not moved aside")
	}
	// The evidence survives: the second attempt at a bead is when the first is
	// worth reading.
	b, err := os.ReadFile(filepath.Join(dir+".1", "brief.md"))
	if err != nil || string(b) != "first attempt" {
		t.Errorf("the earlier attempt was lost: %v", err)
	}

	// A third attempt must not clobber the second.
	os.MkdirAll(dir, 0o755)
	if err := Archive("x-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + ".2"); err != nil {
		t.Error("a second archive overwrote the first")
	}
}

func TestArchiveOnAnUntendedBeadIsNotAnError(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	if err := Archive("never-tended"); err != nil {
		t.Errorf("Archive = %v, want nothing to do", err)
	}
}

// A tender's life has to be reconstructable without reading its worktree,
// because the worktree is the first thing thrown away when someone tidies up.
func TestStartRecordsEverythingKnownAtTheStart(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %s", out)
	}
	for _, args := range [][]string{
		{"-C", repo, "config", "user.email", "t@t"},
		{"-C", repo, "config", "user.name", "t"},
		{"-C", repo, "commit", "-q", "--allow-empty", "-m", "root"},
	} {
		exec.Command("git", args...).Run()
	}
	t.Setenv("HUGEL_AGENT", "/usr/bin/true")

	_, err := Start(Options{
		Bead: beads.Bead{ID: "x-1", Title: "a bead", Type: "task", Priority: 1,
			Accept: "it works"},
		Bed: "bedname", Repo: repo, Soil: "some soil delivered here",
	})
	if err != nil {
		t.Skipf("could not start a tender in this environment: %v", err)
	}

	log, err := events.Load()
	if err != nil {
		t.Fatal(err)
	}
	var start *events.Event
	for i, e := range log {
		if e.Name == "tender.start" {
			start = &log[i]
		}
	}
	if start == nil {
		t.Fatal("no tender.start event")
	}
	if start.Bead != "x-1" || start.Bed != "bedname" {
		t.Errorf("correlation keys lost: %+v", start)
	}
	for _, f := range []string{"branch", "worktree", "tmux", "title", "soil_tokens", "has_criteria"} {
		if _, ok := start.Fields[f]; !ok {
			t.Errorf("tender.start is missing %q: %v", f, start.Fields)
		}
	}
	if start.Fields["has_criteria"] != true {
		t.Error("a bead with acceptance criteria was recorded as having none")
	}
	if tk, _ := start.Fields["soil_tokens"].(float64); tk <= 0 {
		t.Errorf("soil_tokens = %v, want what the brief will cost to read", tk)
	}
	_ = Stop(Tender{Bead: "x-1", Repo: repo, Worktree: filepath.Join(repo, "nope")}, false)
}
