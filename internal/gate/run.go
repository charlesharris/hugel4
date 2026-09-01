package gate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/events"
)

// Run takes a finished tender through the gate.
//
// Ordered so that the cheap refusals come first: there is no point starting a
// reviewing agent on work whose tests do not pass, and no point merging work a
// reviewer rejected.
func Run(o Options) (Report, error) {
	t := o.Tender
	rep := Report{Bead: t.Bead}
	began := time.Now()

	// One event per stage, carrying the detail the refusal rests on rather than
	// a summary of it. A gate that refused three weeks ago has to be as legible
	// as one that refused now, and the detail is the part that goes stale in a
	// person's memory first.
	step := func(s Stage, ok bool, detail string, took time.Duration) {
		rep.Reached = s
		rep.Stages = append(rep.Stages, StageResultRecord{Stage: s, OK: ok, Detail: detail, Took: took})
		events.Emit(events.Event{
			Name: "gate.stage", Bead: t.Bead, Bed: t.Bed,
			Outcome: outcomeOf(ok), Duration: took,
			Fields: events.F{
				"stage": string(s), "detail": detail,
				"branch": t.Branch, "into": o.Into, "remote": o.Remote,
			},
		})
	}
	// A gate run is itself a unit of work, so it gets its own event. The stages
	// make a refusal reconstructable; this makes it findable without walking
	// them, which is the difference between a question that is answerable and
	// one that is worth asking.
	finishAs := func(r Report, outcome string) Report {
		events.Emit(events.Event{
			Name: "gate.run", Bead: t.Bead, Bed: t.Bed,
			Outcome: outcome, Duration: time.Since(began),
			Fields: events.F{
				"reached": string(r.Reached), "why": r.Why,
				"stages": len(r.Stages), "branch": t.Branch,
				"dry_run": o.DryRun, "into": o.Into, "remote": o.Remote,
				"tender_duration_ms": time.Since(t.Started).Milliseconds(),
				"spike":              t.Spike,
			},
		})
		return r
	}
	finish := func(r Report) Report { return finishAs(r, outcomeOf(r.Passed)) }
	stop := func(why string) (Report, error) {
		rep.Passed, rep.Why = false, why
		return finish(rep), nil
	}

	// A spike is not work this gate can judge. It lands no diff by design, so
	// every stage below would read its success as failure -- and the better the
	// spike, the more confidently the gate would report it as failed work.
	//
	// Refused rather than failed, and the word matters where it is counted: a
	// refusal is the gate saying this is not its question, not a verdict on the
	// work. A spike closes on findings recorded, which is somebody else's job.
	if t.Spike {
		step(StageKind, false, "spike", 0)
		rep.Passed, rep.Refused = false, true
		rep.Why = "a spike is not gated: it explores and records its findings with " +
			"bd remember, and leaves no diff to test, review or land"
		return finishAs(rep, "refused"), nil
	}

	// The tender's own account. A tender that reported itself blocked has said
	// the work is not done, and the gate believes it rather than reading the
	// diff and deciding otherwise.
	_, err := os.ReadFile(t.ResultPath())
	if err != nil {
		step(StageResult, false, "no result file", 0)
		return stop("the tender has not finished: no result written")
	}
	outcome := t.Outcome()
	step(StageResult, outcome == "done", outcome, 0)
	if outcome != "done" {
		return stop(fmt.Sprintf("the tender reported %q, not done", outcome))
	}

	test := o.Test
	if test == "" {
		if test = DiscoverTest(t.Worktree); test == "" {
			step(StageTest, false, "no test command found", 0)
			return stop("cannot find how this project runs its tests; pass --test")
		}
	}

	o.say("testing the branch: %s", test)
	start := time.Now()
	out, err := runTests(t.Worktree, test)
	events.Emit(events.Event{
		Name: "gate.test", Bead: t.Bead, Bed: t.Bed, Outcome: outcomeOf(err == nil),
		Duration: time.Since(start), Fields: events.F{"command": test, "on": "branch"},
	})
	step(StageTest, err == nil, tail(out, 12), time.Since(start))
	if err != nil {
		return stop("tests fail on the tender's branch")
	}

	// Read the bead fresh rather than trusting anything captured when the work
	// started. A gate that reviews against stale criteria is reviewing against
	// a spec nobody holds any more.
	accept := ""
	if b, err := beads.Get(t.Repo, t.Bead); err == nil {
		accept = b.Accept
	} else {
		o.say("could not read %s from bd: %v", t.Bead, err)
	}
	if strings.TrimSpace(accept) == "" {
		o.say("%s states no acceptance criteria; the review has nothing to answer against", t.Bead)
	}

	o.say("starting the review")
	start = time.Now()
	verdict, why, err := review(o, accept, out)
	if err != nil {
		step(StageReview, false, err.Error(), time.Since(start))
		return stop("the review could not be run: " + err.Error())
	}
	events.Emit(events.Event{
		Name: "gate.review", Bead: t.Bead, Bed: t.Bed, Outcome: string(verdict),
		Duration: time.Since(start),
		Fields: events.F{
			"why": why, "reviewer_session": t.Session + "-review",
			"had_criteria": strings.TrimSpace(accept) != "",
		},
	})
	step(StageReview, verdict == Pass, string(verdict)+": "+why, time.Since(start))
	if verdict != Pass {
		return stop(fmt.Sprintf("the review said %s: %s", verdict, why))
	}

	if o.DryRun {
		rep.Passed, rep.Why = true, "would land; stopped before merging (--dry-run)"
		return finish(rep), nil
	}

	// Merge onto whatever the base has become, in the tender's own worktree, so
	// a conflict is left in a directory nobody else is working in.
	//
	// The base is the local branch this lands on, never the remote's copy of
	// it. Merging one ref and moving another is only harmless while the two
	// agree, and the first real gate run proved what happens when they do not:
	// origin/main was four commits behind, so merging it was a no-op that
	// reported "clean", and the landing then moved main back onto the branch
	// tip and dropped three commits while every stage said ok.
	o.say("merging %s into %s", t.Branch, o.Into)
	start = time.Now()
	if o.Remote != "" {
		if _, err := git(t.Worktree, "fetch", "--quiet", o.Remote, o.Into); err != nil {
			o.say("could not fetch %s: %v", o.Remote, err)
		}
		if err := checkBase(t.Worktree, o.Into, baseRef(o)); err != nil {
			step(StageMerge, false, err.Error(), time.Since(start))
			return stop(err.Error())
		}
	}
	if out, err := git(t.Worktree, "merge", "--no-edit", o.Into); err != nil {
		// Abort rather than leaving the worktree mid-merge. A half-merged
		// worktree cannot be gated again and reads as broken, while the
		// conflict itself is one command away from being reproduced by whoever
		// comes to look.
		_, _ = git(t.Worktree, "merge", "--abort")
		step(StageMerge, false, tail(out+err.Error(), 12), time.Since(start))
		return stop(fmt.Sprintf("%s conflicts with %s; rebase it in %s and gate it again",
			t.Branch, o.Into, t.Worktree))
	}
	step(StageMerge, true, "clean", time.Since(start))

	// The test that matters. The first ran on work in isolation; this runs on
	// what is actually about to land.
	o.say("testing the merged tree")
	start = time.Now()
	out, err = runTests(t.Worktree, test)
	events.Emit(events.Event{
		Name: "gate.test", Bead: t.Bead, Bed: t.Bed, Outcome: outcomeOf(err == nil),
		Duration: time.Since(start), Fields: events.F{"command": test, "on": "merged", "base": o.Into},
	})
	step(StageRetest, err == nil, tail(out, 12), time.Since(start))
	if err != nil {
		return stop("tests fail once merged with " + o.Into + ", though they passed on the branch")
	}

	// Land it. The base branch is moved in the main repository rather than the
	// worktree, because a worktree cannot check out a branch another worktree
	// holds.
	o.say("landing on %s", o.Into)
	start = time.Now()
	head, err := git(t.Worktree, "rev-parse", "HEAD")
	if err != nil {
		step(StagePush, false, err.Error(), time.Since(start))
		return stop("cannot read the merged head")
	}
	// What the base was before it moved, so the commits this landing introduced
	// stay computable later. Without it, all a revert can be matched against is
	// the one sha recorded here, and a branch of three commits reverted by its
	// middle one reads as work that survived.
	base, err := land(t.Repo, o.Into, strings.TrimSpace(head))
	head = strings.TrimSpace(head)
	if err != nil {
		step(StagePush, false, err.Error(), time.Since(start))
		return stop(err.Error())
	}
	if o.Remote != "" {
		if out, err := git(t.Repo, "push", o.Remote, o.Into); err != nil {
			step(StagePush, false, tail(out+err.Error(), 8), time.Since(start))
			return stop(o.Into + " moved locally but the push failed; it is not on " + o.Remote)
		}
	}
	step(StagePush, true, pushDetail(o), time.Since(start))
	events.Emit(events.Event{
		Name: "gate.land", Bead: t.Bead, Bed: t.Bed, Outcome: "ok",
		Fields: events.F{
			"sha": head, "base": base,
			"into": o.Into, "remote": o.Remote, "pushed": o.Remote != "",
		},
	})

	reason := closeReason(t.Reason(), why)
	if err := beads.Close(t.Repo, t.Bead, reason); err != nil {
		step(StageClose, false, err.Error(), 0)
		return stop("landed, but the bead could not be closed: " + err.Error())
	}
	step(StageClose, true, "closed", 0)

	rep.Passed = true
	rep.Why = "landed on " + o.Into
	return finish(rep), nil
}

// outcomeOf is the one word an event carries for how a stage went.
func outcomeOf(ok bool) string {
	if ok {
		return "ok"
	}
	return "failed"
}

// checkBase refuses to gate onto a branch that is behind its remote.
//
// The gardener's to reconcile, not the gate's: pulling would move a branch
// somebody else may be standing on, and landing anyway means testing against
// one base and pushing to another, which is the bug this whole guard exists
// for. Being ahead is fine and ordinary -- that is just work not pushed yet.
//
// A remote ref that does not exist is not a refusal. A branch never pushed has
// nothing to be behind.
func checkBase(dir, into, remoteRef string) error {
	if _, err := git(dir, "rev-parse", "--verify", "-q", remoteRef); err != nil {
		return nil
	}
	behind, err := isAncestor(dir, into, remoteRef)
	if err != nil || !behind {
		return nil
	}
	same, err := sameCommit(dir, into, remoteRef)
	if err != nil || same {
		return nil
	}
	return fmt.Errorf("%s is behind %s: pull it before gating, or the gate would test one base and push to another",
		into, remoteRef)
}

// land moves a branch to a commit, and refuses to do it destructively.
//
// Returns where the branch was, which the landing event records so the commits
// a landing introduced stay computable afterwards.
//
// Two refusals, both learned from one run. The first is that the new head must
// descend from the current one: with a correct base that can never fire, which
// is exactly the argument for it, since the check costs one git call and is
// the only thing between a wrong base and commits that exist solely in a
// reflog nobody thought to read. The second is compare-and-swap -- the
// three-argument update-ref only moves the ref if it still holds the value
// just read -- so a branch that moved while the gate was testing is a refusal
// rather than a silent overwrite.
func land(repo, into, head string) (string, error) {
	base, err := git(repo, "rev-parse", into)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", into, err)
	}
	base = strings.TrimSpace(base)

	ff, err := isAncestor(repo, base, head)
	if err != nil {
		return base, fmt.Errorf("cannot tell whether %s descends from %s: %w", short(head), short(base), err)
	}
	if !ff {
		return base, fmt.Errorf("refusing to move %s to %s, which does not descend from %s: that would discard work",
			into, short(head), short(base))
	}
	if out, err := git(repo, "update-ref", "refs/heads/"+into, head, base); err != nil {
		return base, fmt.Errorf("cannot move %s to the merged head: %s", into, tail(out+err.Error(), 8))
	}
	return base, nil
}

// isAncestor reports whether a is reachable from b.
func isAncestor(dir, a, b string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", a, b)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return false, nil // a definite no, not a broken call
		}
		return false, err
	}
	return true, nil
}

// sameCommit reports whether two refs name the same commit, which is the case
// isAncestor cannot distinguish from being behind.
func sameCommit(dir, a, b string) (bool, error) {
	x, err := git(dir, "rev-parse", a)
	if err != nil {
		return false, err
	}
	y, err := git(dir, "rev-parse", b)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(x) == strings.TrimSpace(y), nil
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// baseRef names the remote's copy of the branch, which is what the gate
// reconciles against. It is deliberately not what the gate merges: see the
// merge stage.
func baseRef(o Options) string {
	if o.Remote == "" {
		return o.Into
	}
	return o.Remote + "/" + o.Into
}

func pushDetail(o Options) string {
	if o.Remote == "" {
		return "landed on " + o.Into + " (local only)"
	}
	return "pushed to " + o.Remote + "/" + o.Into
}

// closeReason is what the bead records. It is composted afterwards, so it says
// why rather than that.
func closeReason(tenderSaid, reviewWhy string) string {
	var b strings.Builder
	b.WriteString("Tended and reviewed. ")
	if tenderSaid != "" {
		b.WriteString(tenderSaid + " ")
	}
	if reviewWhy != "" {
		b.WriteString("Review: " + reviewWhy)
	}
	return strings.TrimSpace(b.String())
}
