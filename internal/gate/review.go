package gate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charris/hugel/internal/tender"
)

// Verdict is what the reviewing agent concluded.
type Verdict string

const (
	Pass    Verdict = "pass"
	Changes Verdict = "changes-needed"
	Reject  Verdict = "reject"
)

var verdictLine = regexp.MustCompile(`(?im)^\s*(pass|changes-needed|reject)\b`)

// ReadVerdict finds the reviewer's answer in what it wrote.
//
// An unreadable review is not a pass. A reviewer that rambled, crashed, or
// answered a different question has not approved anything, and the gate has to
// treat silence as refusal or the review is decorative.
func ReadVerdict(review string) (Verdict, string) {
	section := review
	if i := strings.Index(strings.ToLower(review), "## verdict"); i >= 0 {
		section = review[i+len("## verdict"):]
	}
	m := verdictLine.FindStringSubmatch(section)
	if m == nil {
		return Reject, "the review states no verdict"
	}
	v := Verdict(strings.ToLower(m[1]))
	return v, firstLine(strings.TrimSpace(section[len(m[0]):]))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// reviewPaths sit beside the tender's own paperwork.
func reviewBriefPath(t tender.Tender) string {
	return filepath.Join(filepath.Dir(t.Worktree), "review-brief.md")
}

func reviewPath(t tender.Tender) string {
	return filepath.Join(filepath.Dir(t.Worktree), "review.md")
}

// ReviewBrief asks a second agent to read the diff.
//
// A different prompt and a separate run, deliberately. The same agent asked
// whether its own work is good has every reason to say yes and no way to see
// what it did not think of, so the reviewer is told what the bead asked for and
// left to find the difference itself.
func ReviewBrief(t tender.Tender, accept, testOutput string) string {
	return fmt.Sprintf(`# Review %s

%s
%s
## What you are doing

Another agent worked this bead in this worktree, on branch %s. You are the
review. You did not write this code and you are not here to be agreeable: work
out whether it should land.

You have the bead above, the diff (git diff %s...HEAD), the tender's own account
in %s, and the test output below.

## What to check

Start with the acceptance criteria above, if there are any. Take them one at a
time and say whether each is met, and how you checked. That question is
decidable; "is this good work" is not, and an agent asked the second one about
another agent's code will tend to say yes.

Criteria are a floor and not a ceiling. Code can meet every one of them and
still be wrong, so once they are answered, keep looking:

- Does it do what the bead asked, rather than something adjacent?
- Is it correct? Look for the failure the tests do not cover.
- Does it match the surrounding code, or has it invented a second way to do
  something the project already does?
- Did the tender leave anything behind: debugging output, a stubbed path, a
  commented-out block, a file it did not mean to add?
- Does what it claims in its result match what the diff actually does?

Read the code. A review that only reads the tender's account of the code is
worth nothing.

## Tests

    %s

## What to write

Write %s, exactly this shape:

    ## Criteria
    one line per acceptance criterion: met | not met | unclear, and how you
    checked. Omit this section only if the bead stated none.

    ## Verdict
    pass | changes-needed | reject
    one sentence saying why

    ## Findings
    what you found, most serious first; nothing if nothing

Say pass only if every stated criterion is met and you would merge it yourself.
An unmet criterion is changes-needed at best, whatever else is good about the
work. Say changes-needed if it is close
and you can name what is missing. Say reject if it is the wrong solution.
Writing that file is how the gate learns you are done, so write it last.

Change nothing. Do not commit, do not fix what you find, do not touch the
branch. Naming a problem and repairing it are two different jobs, and doing both
means nobody reviewed the repair.
`, t.Bead, t.Title, acceptSection(accept), t.Branch, defaultBase, t.ResultPath(), tail(testOutput, 30), reviewPath(t))
}

// acceptSection renders the bead's acceptance criteria, or says plainly that
// there are none. Silence would leave the reviewer to guess whether criteria
// existed and were withheld, and a reviewer that invents a standard is back to
// answering "is this good work".
func acceptSection(accept string) string {
	if strings.TrimSpace(accept) == "" {
		return "\n## Acceptance criteria\n\nThe bead states none. Judge it against what the bead asks for, and say in\nyour findings that it shipped without criteria.\n"
	}
	return "\n## Acceptance criteria\n\nThis is what the work was promised to do. Answer against it.\n\n" +
		strings.TrimSpace(accept) + "\n"
}

const defaultBase = "main"

// review runs the reviewing agent and waits for its answer.
func review(o Options, accept, testOutput string) (Verdict, string, error) {
	t := o.Tender
	brief := ReviewBrief(t, accept, testOutput)
	if err := os.WriteFile(reviewBriefPath(t), []byte(brief), 0o644); err != nil {
		return Reject, "", err
	}
	_ = os.Remove(reviewPath(t))

	session := t.Session + "-review"
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()

	args := []string{"new-session", "-d", "-s", session, "-c", t.Worktree, agentBin()}
	if o.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, fmt.Sprintf(
		"Read %s and carry out the review it describes. Work autonomously: do not wait for confirmation.",
		reviewBriefPath(t)))
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		return Reject, "", fmt.Errorf("start reviewer: %w: %s", err, strings.TrimSpace(string(out)))
	}
	o.say("reviewing in tmux session %s", session)

	deadline := time.Now().Add(o.Wait)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(reviewPath(t)); err == nil {
			_ = exec.Command("tmux", "kill-session", "-t", session).Run()
			v, why := ReadVerdict(string(b))
			return v, why, nil
		}
		if exec.Command("tmux", "has-session", "-t", session).Run() != nil {
			// The reviewer's session ended without writing anything. That is a
			// refusal, not an approval: nobody looked.
			return Reject, "the reviewer stopped without writing a review", nil
		}
		time.Sleep(3 * time.Second)
	}
	_ = exec.Command("tmux", "kill-session", "-t", session).Run()
	return Reject, fmt.Sprintf("the reviewer did not answer within %s", o.Wait), nil
}

func agentBin() string {
	if b := os.Getenv("HUGEL_AGENT"); b != "" {
		return b
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	return "claude"
}
