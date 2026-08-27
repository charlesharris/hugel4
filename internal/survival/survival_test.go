package survival

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/events"
)

func at(hoursAgo int) time.Time { return time.Now().Add(-time.Duration(hoursAgo) * time.Hour) }

// The acceptance this bead was filed against: one report of what fraction of
// gate-approved work survived.
func TestTheRateCountsOnlyWorkGivenTimeToFail(t *testing.T) {
	now := time.Now()
	landings := []Landing{
		{Bead: "held-1", At: at(24 * 30)},
		{Bead: "held-2", At: at(24 * 20)},
		{Bead: "gone", At: at(24 * 15)},
		{Bead: "back", At: at(24 * 10)},
		{Bead: "fresh", At: at(2)},
	}
	facts := map[string]Fact{
		"gone": {RevertedBy: "abcdef1234", Subject: `Revert "the thing"`, RevertedAt: at(24 * 14)},
		"back": {Status: "open"},
	}
	rep := Grade(landings, facts, now, 7*24*time.Hour)

	if rep.Held != 2 || rep.Reverted != 1 || rep.Reopened != 1 || rep.Young != 1 {
		t.Fatalf("held/reverted/reopened/young = %d/%d/%d/%d",
			rep.Held, rep.Reverted, rep.Reopened, rep.Young)
	}
	// Four judged, two of them held: a fresh landing is not a survivor, and
	// counting it as one would let a burst of new work flatter the gate.
	if rep.Judged() != 4 || rep.Rate() != 0.5 {
		t.Errorf("rate = %v over %d judged, want 0.5 over 4", rep.Rate(), rep.Judged())
	}
	if rep.Verdicts[0].Fate != Reverted {
		t.Errorf("report opens with %s; the approvals that did not hold are the ones worth reading",
			rep.Verdicts[0].Fate)
	}
	// The age of a revert is how long the work lasted, not how old it is now.
	if d := rep.Verdicts[0].Age; d < 20*time.Hour || d > 30*time.Hour {
		t.Errorf("reverted after %v, want about a day", d)
	}
}

func TestNothingJudgedIsSaidRatherThanReportedAsZero(t *testing.T) {
	rep := Grade([]Landing{{Bead: "x", At: at(1)}}, nil, time.Now(), 7*24*time.Hour)
	if rep.Measurable() {
		t.Error("a rate with no denominator is not a measurement")
	}
}

// A revert outranks the maturity rule: evidence that arrived beats a rule about
// how long to wait for it.
func TestAChangeRevertedTheSameDayIsNotYoung(t *testing.T) {
	rep := Grade([]Landing{{Bead: "x", At: at(2)}},
		map[string]Fact{"x": {RevertedBy: "deadbeef"}}, time.Now(), 7*24*time.Hour)
	if rep.Reverted != 1 || rep.Young != 0 {
		t.Errorf("reverted/young = %d/%d", rep.Reverted, rep.Young)
	}
}

func TestLandingsTakeTheirRepositoryFromTheTenderThatDidTheWork(t *testing.T) {
	log := []events.Event{
		{Name: "tender.start", Bead: "a-1", Bed: "garden", Time: at(50),
			Fields: events.F{"repo": "/src/garden"}},
		{Name: "gate.stage", Bead: "a-1", Time: at(49)},
		{Name: "gate.land", Bead: "a-1", Bed: "garden", Time: at(48),
			Fields: events.F{"sha": "ffff", "base": "eeee", "into": "main"}},
		{Name: "gate.land", Bead: "old-1", Time: at(500), Fields: events.F{"sha": "1111"}},
	}
	got := Landings(log, at(100))
	if len(got) != 1 {
		t.Fatalf("got %d landings, want the one inside the window", len(got))
	}
	l := got[0]
	if l.Repo != "/src/garden" || l.SHA != "ffff" || l.Base != "eeee" || l.Into != "main" {
		t.Errorf("landing = %+v", l)
	}
}

// A branch that landed three commits and was reverted by its middle one is
// still work that did not survive.
func TestARevertOfAnyCommitTheLandingIntroducedCountsAgainstIt(t *testing.T) {
	repo := gitRepo(t)
	base := commitFile(t, repo, "base.txt", "one")
	mid := commitFile(t, repo, "mid.txt", "two")
	head := commitFile(t, repo, "head.txt", "three")

	run(t, repo, "revert", "--no-edit", mid)

	facts, unattributed := Look([]Landing{
		{Bead: "b-1", Repo: repo, SHA: head, Base: base, Into: branchOf(t, repo), At: at(1)},
	})
	f := facts["b-1"]
	if f.RevertedBy == "" {
		t.Fatalf("a commit the landing introduced was reverted and nothing recorded it: %+v %d",
			facts, unattributed)
	}
	if !strings.Contains(strings.ToLower(f.Subject), "revert") {
		t.Errorf("subject = %q, want the reverting commit's own words", f.Subject)
	}
	if unattributed != 0 {
		t.Errorf("unattributed = %d, want the revert attributed", unattributed)
	}
}

// A revert hugel cannot tie to a landing is still a revert. Dropping it would
// let the rate flatter itself by excluding what it could not explain.
func TestARevertOfWorkTheGateNeverLandedIsReportedRatherThanDropped(t *testing.T) {
	repo := gitRepo(t)
	commitFile(t, repo, "base.txt", "one")
	// Committed to the branch by hand, so it is what the base had become by the
	// time the gate landed anything -- outside the range the landing introduced.
	stray := commitFile(t, repo, "stray.txt", "by hand")
	landed := commitFile(t, repo, "landed.txt", "through the gate")
	run(t, repo, "revert", "--no-edit", stray)

	facts, unattributed := Look([]Landing{
		{Bead: "b-1", Repo: repo, SHA: landed, Base: stray, Into: branchOf(t, repo), At: at(1)},
	})
	if f := facts["b-1"]; f.RevertedBy != "" {
		t.Errorf("blamed the gate for a revert of work it never landed: %+v", f)
	}
	if unattributed != 1 {
		t.Errorf("unattributed = %d, want the revert counted somewhere", unattributed)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %s", out)
	}
	run(t, dir, "config", "user.email", "t@t")
	run(t, dir, "config", "user.name", "t")
	return dir
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func commitFile(t *testing.T, repo, name, body string) string {
	t.Helper()
	if err := exec.Command("sh", "-c",
		"printf %s "+shquote(body)+" > "+repo+"/"+name).Run(); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", name)
	run(t, repo, "commit", "-q", "-m", "add "+name)
	return strings.TrimSpace(run(t, repo, "rev-parse", "HEAD"))
}

func shquote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func branchOf(t *testing.T, repo string) string {
	t.Helper()
	return strings.TrimSpace(run(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
}

// Landings recorded before the gate wrote down its base still grade, on the one
// sha they have. A revert of exactly what landed is the common case and the
// only one they can answer.
func TestALandingWithNoRecordedBaseIsStillGradedOnItsOwnSHA(t *testing.T) {
	repo := gitRepo(t)
	commitFile(t, repo, "base.txt", "one")
	landed := commitFile(t, repo, "landed.txt", "through the gate")
	run(t, repo, "revert", "--no-edit", landed)

	facts, _ := Look([]Landing{
		{Bead: "b-1", Repo: repo, SHA: landed, Into: branchOf(t, repo), At: at(1)},
	})
	if facts["b-1"].RevertedBy == "" {
		t.Errorf("the landed sha itself was reverted and nothing recorded it: %+v", facts)
	}
}

// Work filed because of a landing is reported beside it and never counted
// against it. Plenty of work that held perfectly well turned something up on
// the way, and treating a follow-up as a failure would grade thoroughness as
// harm.
func TestWorkFoundBecauseOfALandingDoesNotCountAgainstIt(t *testing.T) {
	rep := Grade(
		[]Landing{{Bead: "quiet", At: at(24 * 20)}, {Bead: "fruitful", At: at(24 * 20)}},
		map[string]Fact{"fruitful": {Found: []Found{
			{Bead: "bug-1", Title: "it drops the last row", Type: "bug", Open: true},
		}}},
		time.Now(), 7*24*time.Hour)

	if rep.Held != 2 || rep.Rate() != 1 {
		t.Fatalf("held %d at rate %v, want both landings held", rep.Held, rep.Rate())
	}
	// It still leads the report: a landing that held but turned something up is
	// worth reading before one with nothing to say.
	if rep.Verdicts[0].Bead != "fruitful" || len(rep.Verdicts[0].Found) != 1 {
		t.Errorf("report opens with %+v", rep.Verdicts[0])
	}
	if len(rep.Verdicts[1].Found) != 0 {
		t.Errorf("a landing nothing came from carries %v", rep.Verdicts[1].Found)
	}
}
