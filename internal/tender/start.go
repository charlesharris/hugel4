package tender

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charris/hugel/internal/beads"
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
		Bead: o.Bead.ID, Bed: o.Bed, Title: o.Bead.Title, Repo: o.Repo,
		Worktree: worktreeIn(dir, o.Bed),
		Branch:   "hugel/" + o.Bead.ID,
		Session:  sessionName(o.Bead.ID),
		Started:  now(),
	}
	if t.Running() {
		return nil, fmt.Errorf("%s is already being tended in tmux session %s", o.Bead.ID, t.Session)
	}
	if _, err := os.Stat(t.Worktree); err == nil {
		return nil, fmt.Errorf("%s already has a worktree at %s; stop it first", o.Bead.ID, t.Worktree)
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
	if err := os.WriteFile(t.BriefPath(), []byte(Brief(o, *t)), 0o644); err != nil {
		return nil, err
	}
	if err := t.save(); err != nil {
		return nil, err
	}

	args := []string{"new-session", "-d", "-s", t.Session, "-c", t.Worktree, claudeBin()}
	if o.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, fmt.Sprintf(
		"Read %s and carry out the work it describes. Work autonomously: do not wait for confirmation.",
		t.BriefPath()))
	if err := tmux(args...); err != nil {
		_ = git(o.Repo, "worktree", "remove", "--force", t.Worktree)
		return nil, err
	}
	return t, nil
}

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
	if strings.TrimSpace(o.Extra) != "" {
		fmt.Fprintf(&b, "## Also\n\n%s\n\n", strings.TrimSpace(o.Extra))
	}

	fmt.Fprintf(&b, `## Where you are

You are a tender: an agent working one bead of a garden, unattended, in a git
worktree of its own at %s, on branch %s. Nobody is watching the pane. Work
through the bead and do not wait for confirmation.

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
`, t.Worktree, t.Branch, t.ResultPath(), t.Branch, t.ResultPath())
	return b.String()
}

// Stop ends a tender: the tmux session, and optionally the worktree with it.
// The worktree is kept by default. A run that went wrong is the most useful
// thing in the garden until someone has read it.
func Stop(t Tender, removeWorktree bool) error {
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
