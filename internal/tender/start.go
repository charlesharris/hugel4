package tender

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/events"
)

// Options is what starting a tender needs.
type Options struct {
	Bead beads.Bead
	Bed  string
	Repo string

	// SkipPermissions runs the agent without permission prompts. A tender is
	// unattended by definition, and an unattended agent that stops at a prompt
	// has not done the work -- it has parked in a pane nobody is looking at.
	// It stays an explicit choice because it lets the agent run anything.
	SkipPermissions bool

	// Extra is appended to the brief: anything the gardener wants this
	// particular tender to know.
	Extra string

	// Spike explores instead of building. Its product is knowledge -- findings
	// recorded with bd remember as it goes -- and it leaves no diff behind.
	//
	// The same machinery as a tender because the difference is the brief, not
	// the mechanism: a worktree, a detached session, a file at each end.
	Spike bool

	// Attach means a person will be sitting in the pane. It changes one thing
	// in the brief and it is the thing that matters most: whether asking a
	// question is the right move or a way of parking forever.
	Attach bool

	// Soil is what the pile already knows about this bead's subject.
	//
	// For a tender the pull argument inverts. An interactive session has a
	// person in it who notices that prior knowledge would help and asks for it;
	// a tender has nobody, and the bead text is a ready-made query nobody has
	// to write. So soil is pushed here and pulled everywhere else, for exactly
	// the same reason in both directions: where the tokens land, and who is
	// there to ask for them.
	Soil string
}

// Start prepares a worktree, writes the brief, and launches an agent in a
// detached tmux session.
//
// The prompt handed to the agent is one sentence pointing at the brief. The
// brief itself is a file, so it can be as long as the work requires without
// being retyped into a pane, it survives the session that read it, and a
// gardener reviewing the tender afterwards can see exactly what was asked.
func Start(o Options) (*Tender, error) {
	dir, err := Dir(o.Bead.ID)
	if err != nil {
		return nil, err
	}
	t := &Tender{
		Bead: o.Bead.ID, Bed: o.Bed, Title: o.Bead.Title, Repo: o.Repo, Spike: o.Spike,
		Worktree: worktreeIn(dir, o.Bed),
		Branch:   "hugel/" + o.Bead.ID,
		Session:  sessionName(o.Bead.ID),
		Started:  now(),
	}
	if t.Running() {
		return nil, fmt.Errorf("%s is already being tended in tmux session %s", o.Bead.ID, t.Session)
	}
	// A bead worked before gets a fresh directory, and the earlier attempt is
	// moved aside rather than removed. Refusing outright would mean a bead
	// handed back for correction could never be tended again.
	if _, err := os.Stat(t.Worktree); err == nil {
		if err := Archive(o.Bead.ID); err != nil {
			return nil, fmt.Errorf("archive the earlier tender of %s: %w", o.Bead.ID, err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create tender dir: %w", err)
	}

	// A branch per bead, from the repository's current head. The tender works
	// where nothing else is working, so a run that goes wrong is thrown away by
	// deleting a directory.
	if err := git(o.Repo, "worktree", "add", "-b", t.Branch, t.Worktree); err != nil {
		return nil, err
	}
	brief := Brief(o, *t)
	if err := os.WriteFile(t.BriefPath(), []byte(brief), 0o644); err != nil {
		return nil, err
	}
	if err := t.save(); err != nil {
		return nil, err
	}

	args := []string{"new-session", "-d", "-s", t.Session, "-c", t.Worktree, claudeBin()}
	if o.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	prompt := fmt.Sprintf(
		"Read %s and carry out the work it describes. Work autonomously: do not wait for confirmation.",
		t.BriefPath())
	if o.Spike {
		prompt = fmt.Sprintf(
			"Read %s and carry out the exploration it describes. Record what you find as you go; write no code.",
			t.BriefPath())
	}
	args = append(args, prompt)
	if err := tmux(args...); err != nil {
		_ = git(o.Repo, "worktree", "remove", "--force", t.Worktree)
		events.Emit(events.Event{
			Name: "tender.start", Bead: t.Bead, Bed: t.Bed, Outcome: "failed",
			Fields: events.F{"error": err.Error()},
		})
		return nil, err
	}
	// Everything known at the moment work begins. A tender's life has to be
	// reconstructable without reading its worktree, because the worktree is the
	// first thing thrown away when someone tidies up.
	events.Emit(events.Event{
		Name: "tender.start", Bead: t.Bead, Bed: t.Bed, Outcome: "ok",
		Fields: events.F{
			"title": o.Bead.Title, "type": o.Bead.Type, "priority": o.Bead.Priority,
			"branch": t.Branch, "worktree": t.Worktree, "tmux": t.Session,
			"repo": o.Repo, "soil_tokens": tokensIn(o.Soil), "spike": o.Spike,
			"has_criteria": strings.TrimSpace(o.Bead.Accept) != "",
			"brief_bytes":  len(brief), "skip_permissions": o.SkipPermissions,
		},
	})
	return t, nil
}

// tokensIn estimates what the soil in a brief will cost the tender to read.
// Characters over four, the same approximation the budgeter uses: being exactly
// right would mean asking a tokeniser, which costs more than the answer is
// worth.
func tokensIn(s string) int { return (len(s) + 3) / 4 }

func claudeBin() string {
	if b := os.Getenv("HUGEL_AGENT"); b != "" {
		return b
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	return "claude"
}

// Brief is what the tender is asked to do.
//
// It says what the work is, where to put the answer, and what not to touch. The
// last part matters most: a tender that pushes, or that closes its own bead, has
// taken a decision the gate exists to make.
func Brief(o Options, t Tender) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", o.Bead.ID, o.Bead.Title)
	if strings.TrimSpace(o.Bead.Body) != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(o.Bead.Body))
	}
	if strings.TrimSpace(o.Bead.Accept) != "" {
		// The same criteria the review will be answered against. A tender that
		// does not know what done looks like builds something a reviewer then
		// rejects, and the two of them disagree about a standard only one of
		// them was shown.
		fmt.Fprintf(&b, "## Done when\n\n%s\n\nThe review will be answered against exactly this, one item at a time.\n\n",
			strings.TrimSpace(o.Bead.Accept))
	}
	// The standard the review will hold this to, shown to the tender before it
	// starts. A criterion only the reviewer knows about is a trap: the tender
	// builds something, the reviewer refuses it, and the two of them disagree
	// about a standard only one of them was ever told.
	if !o.Spike {
		fmt.Fprintf(&b, "## Observability\n\n%s\n\nThe review answers this as a standing criterion, alongside any above.\n\n",
			events.Convention)
	}
	if strings.TrimSpace(o.Extra) != "" {
		fmt.Fprintf(&b, "## Also\n\n%s\n\n", strings.TrimSpace(o.Extra))
	}
	if strings.TrimSpace(o.Soil) != "" {
		fmt.Fprintf(&b, `## What the garden already knows

Drawn from the pile for this bead. It is a survey, not the truth: entries record
what was true when they were written, most are unreviewed, and some will be
wrong. Read one in full with "hugel pile show <id>" before relying on it, and
prefer the code in front of you where they disagree.

%s

`, strings.TrimSpace(o.Soil))
	}

	if o.Spike {
		spikeBrief(&b, o, t)
	} else {
		tenderBrief(&b, o, t)
	}
	return b.String()
}

// watching says who is in the pane, which decides whether a question is the
// right move or a way of parking forever. It is the one line that changes
// between an unattended run and an attached one, and it changes everything
// about how the agent should behave when it is unsure.
func watching(attach bool) string {
	if attach {
		return `Somebody is attached to this pane and can answer you. Ask early rather than
guessing: a question costs a minute and a wrong assumption costs the run.`
	}
	return `Nobody is watching the pane. Work through it and do not wait for
confirmation -- an agent that stops to ask, unattended, has not done the work,
it has parked.`
}

func tenderBrief(b *strings.Builder, o Options, t Tender) {
	fmt.Fprintf(b, `## Where you are

You are a tender: an agent working one bead of a garden, in a git worktree of
its own at %s, on branch %s. %s

## What to do

1. Do the work the bead describes.
2. Run the project's tests. Find how they are run rather than assuming: a
   Makefile target, a task runner, whatever the repository already uses.
3. Commit to this branch, in small commits, with messages that say why rather
   than what. Those messages are composted into the garden's pile afterwards
   and become the knowledge the next tender draws on, so a message that
   restates the diff is a wasted one.
4. Write your result to %s, as described below. Write it whether the work
   succeeded or not.

## What not to do

- Do not push, merge, rebase onto another branch, or touch any branch but %s.
- Do not close the bead. A separate review decides whether this lands.
- Do not work outside this worktree. Other tenders are working elsewhere.
- Do not edit the brief.

## The result file

Write %s as markdown, with these sections:

    ## Outcome
    done | partial | blocked   -- one word, then a sentence saying why

    ## What changed
    the commits made and what each was for

    ## Tests
    the command run, and its result

    ## For the reviewer
    anything a reviewer should look at first: a judgement call made, a
    shortcut taken, a thing that did not work and why

Writing that file is how the garden learns you have finished, so write it last
and write it once.
`, t.Worktree, t.Branch, watching(o.Attach), t.ResultPath(), t.Branch, t.ResultPath())
}

// spikeBrief asks for knowledge instead of code.
//
// The findings go out through bd remember one at a time, as they are found. A
// spike that saves its recording for the end loses everything it learned if it
// runs out of room first, which is the failure a spike exists to prevent rather
// than reproduce.
func spikeBrief(b *strings.Builder, o Options, t Tender) {
	fmt.Fprintf(b, `## Where you are

You are a spike: an agent sent to find something out before anyone builds on
it. You have a git worktree of your own at %s. %s

Nothing you write here will be kept. There is no review and no merge, and the
worktree is thrown away. What survives this session is what you record.

## What to do

1. Read whatever answers the question: the code, the history, the tests, the
   tools the project already uses. Follow what you actually find rather than a
   plan made before you started.
2. Record each finding the moment you have it, one per call, on one line:

       bd remember "the extractor only reads commit messages, so a session that ends without a commit composts to nothing"

   As you go, not at the end. A spike that saves its recording for last loses
   everything it learned if it runs out of room first, which is the failure
   this exists to prevent.
3. State findings as what is true, not as what you did. "The gate's second test
   runs on the merged tree" is a finding. "Looked at the gate" is not, and will
   be composted into the pile as though it were.
4. Write your result to %s when you are done.

## What not to do

- Do not write code. No edits, no new files, no commits. A spike that leaves a
  diff has done the next bead's work badly instead of its own work well.
- Do not close the bead, and do not start the work this is exploring for.
- Do not work outside this worktree. Other agents are working elsewhere.
- Do not edit the brief.

## The result file

Write %s as markdown, with these sections:

    ## Outcome
    done | partial | blocked   -- one word, then a sentence saying why

    ## What you found
    each finding, in the order you found them

    ## What is still unknown
    what you could not answer, and what it would take to answer it

Writing that file is how the garden learns you have finished, so write it last
and write it once. The findings themselves are already recorded; this says what
the exploration came to.
`, t.Worktree, watching(o.Attach), t.ResultPath(), t.ResultPath())
}

// Stop ends a tender: the tmux session, and optionally the worktree with it.
// The worktree is kept by default. A run that went wrong is the most useful
// thing in the garden until someone has read it.
func Stop(t Tender, removeWorktree bool) error {
	events.Emit(events.Event{
		Name: "tender.stop", Bead: t.Bead, Bed: t.Bed, Outcome: t.State(),
		Duration: time.Since(t.Started),
		Fields:   events.F{"worktree_removed": removeWorktree, "branch": t.Branch},
	})
	if t.Running() {
		if err := tmux("kill-session", "-t", t.Session); err != nil {
			return err
		}
	}
	if !removeWorktree {
		return nil
	}
	if err := git(t.Repo, "worktree", "remove", "--force", t.Worktree); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Dir(t.Worktree))
}
