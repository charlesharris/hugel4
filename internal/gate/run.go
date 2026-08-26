package gate

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charris/hugel/internal/beads"
)

// Run takes a finished tender through the gate.
//
// Ordered so that the cheap refusals come first: there is no point starting a
// reviewing agent on work whose tests do not pass, and no point merging work a
// reviewer rejected.
func Run(o Options) (Report, error) {
	t := o.Tender
	rep := Report{Bead: t.Bead}
	step := func(s Stage, ok bool, detail string, took time.Duration) {
		rep.Reached = s
		rep.Stages = append(rep.Stages, StageResultRecord{Stage: s, OK: ok, Detail: detail, Took: took})
	}
	stop := func(why string) (Report, error) {
		rep.Passed, rep.Why = false, why
		return rep, nil
	}

	// The tender's own account. A tender that reported itself blocked has said
	// the work is not done, and the gate believes it rather than reading the
	// diff and deciding otherwise.
	result, err := os.ReadFile(t.ResultPath())
	if err != nil {
		step(StageResult, false, "no result file", 0)
		return stop("the tender has not finished: no result written")
	}
	outcome := outcomeIn(string(result))
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
	step(StageTest, err == nil, tail(out, 12), time.Since(start))
	if err != nil {
		return stop("tests fail on the tender's branch")
	}

	o.say("starting the review")
	start = time.Now()
	verdict, why, err := review(o, out)
	if err != nil {
		step(StageReview, false, err.Error(), time.Since(start))
		return stop("the review could not be run: " + err.Error())
	}
	step(StageReview, verdict == Pass, string(verdict)+": "+why, time.Since(start))
	if verdict != Pass {
		return stop(fmt.Sprintf("the review said %s: %s", verdict, why))
	}

	if o.DryRun {
		rep.Passed, rep.Why = true, "would land; stopped before merging (--dry-run)"
		return rep, nil
	}

	// Merge onto whatever the base has become, in the tender's own worktree, so
	// a conflict is left in a directory nobody else is working in.
	o.say("merging %s into %s", t.Branch, o.Into)
	start = time.Now()
	if _, err := git(t.Worktree, "fetch", "--quiet", o.Remote, o.Into); err != nil && o.Remote != "" {
		o.say("could not fetch %s: %v", o.Remote, err)
	}
	if out, err := git(t.Worktree, "merge", "--no-edit", baseRef(o)); err != nil {
		// Abort rather than leaving the worktree mid-merge. A half-merged
		// worktree cannot be gated again and reads as broken, while the
		// conflict itself is one command away from being reproduced by whoever
		// comes to look.
		_, _ = git(t.Worktree, "merge", "--abort")
		step(StageMerge, false, tail(out+err.Error(), 12), time.Since(start))
		return stop(fmt.Sprintf("%s conflicts with %s; rebase it in %s and gate it again",
			t.Branch, baseRef(o), t.Worktree))
	}
	step(StageMerge, true, "clean", time.Since(start))

	// The test that matters. The first ran on work in isolation; this runs on
	// what is actually about to land.
	o.say("testing the merged tree")
	start = time.Now()
	out, err = runTests(t.Worktree, test)
	step(StageRetest, err == nil, tail(out, 12), time.Since(start))
	if err != nil {
		return stop("tests fail once merged with " + baseRef(o) + ", though they passed on the branch")
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
	if out, err := git(t.Repo, "update-ref", "refs/heads/"+o.Into, strings.TrimSpace(head)); err != nil {
		step(StagePush, false, tail(out+err.Error(), 8), time.Since(start))
		return stop("cannot move " + o.Into + " to the merged head")
	}
	if o.Remote != "" {
		if out, err := git(t.Repo, "push", o.Remote, o.Into); err != nil {
			step(StagePush, false, tail(out+err.Error(), 8), time.Since(start))
			return stop(o.Into + " moved locally but the push failed; it is not on " + o.Remote)
		}
	}
	step(StagePush, true, pushDetail(o), time.Since(start))

	reason := closeReason(string(result), why)
	if err := beads.Close(t.Repo, t.Bead, reason); err != nil {
		step(StageClose, false, err.Error(), 0)
		return stop("landed, but the bead could not be closed: " + err.Error())
	}
	step(StageClose, true, "closed", 0)

	rep.Passed = true
	rep.Why = "landed on " + o.Into
	return rep, nil
}

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

// outcomeIn reads the one word the tender was asked to lead its result with.
func outcomeIn(result string) string {
	i := strings.Index(strings.ToLower(result), "## outcome")
	if i < 0 {
		return "unstated"
	}
	for _, line := range strings.Split(result[i+len("## outcome"):], "\n") {
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			continue
		}
		for _, word := range []string{"done", "partial", "blocked"} {
			if strings.HasPrefix(line, word) {
				return word
			}
		}
		return "unstated"
	}
	return "unstated"
}

// closeReason is what the bead records. It is composted afterwards, so it says
// why rather than that.
func closeReason(result, reviewWhy string) string {
	var b strings.Builder
	b.WriteString("Tended and reviewed. ")
	if s := sectionIn(result, "## for the reviewer"); s != "" {
		b.WriteString(s)
		b.WriteString(" ")
	} else if s := sectionIn(result, "## what changed"); s != "" {
		b.WriteString(s)
		b.WriteString(" ")
	}
	if reviewWhy != "" {
		b.WriteString("Review: " + reviewWhy)
	}
	return strings.TrimSpace(b.String())
}

func sectionIn(doc, heading string) string {
	i := strings.Index(strings.ToLower(doc), heading)
	if i < 0 {
		return ""
	}
	rest := doc[i+len(heading):]
	if j := strings.Index(rest, "\n##"); j >= 0 {
		rest = rest[:j]
	}
	return strings.Join(strings.Fields(rest), " ")
}
