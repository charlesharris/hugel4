package tender

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charris/hugel/internal/transcript"
)

// Progress is what a tender has been doing, as opposed to whether it is alive.
//
// A tender can be priced and not seen: yield says what one spent, and until
// this there was nothing that said what it spent it on. With a pool running
// unattended that made the bill the first sign that one had gone wrong.
//
// Every field is read from the transcript the harness is already writing. This
// is a reading of an instrument that exists, not a new one -- a tender that
// had to report its own progress would be a tender that could lie about it.
type Progress struct {
	Turns   int       // model turns taken
	Files   int       // distinct files it has changed
	Trouble int       // tool calls that errored
	Last    time.Time // when it last did anything
	Doing   string    // the last thing it did, in a few words
}

// Idle is how long the tender has been silent. A tender that has stopped
// saying anything has usually stopped, whatever tmux still reports.
func (p Progress) Idle() time.Duration {
	if p.Last.IsZero() {
		return 0
	}
	return time.Since(p.Last)
}

// ProgressOf reads what a tender has done from the sessions it ran.
//
// The join is the worktree: the harness files a session under the directory it
// ran in, and a tender's directory is its own worktree by construction. More
// than one session can share it -- an interrupted tender resumed, or a
// gardener who attached and asked something -- so they are summed rather than
// picked between. All of it is work done on this bead.
func ProgressOf(t Tender, sessions []*transcript.Session) (Progress, bool) {
	want := filepath.Clean(t.Worktree)
	var (
		p     Progress
		files = map[string]bool{}
		found bool
	)
	for _, s := range sessions {
		if s == nil || filepath.Clean(s.CWD) != want {
			continue
		}
		found = true
		p.Turns += len(s.Requests)
		for _, u := range s.Tools {
			if u.Writes() && u.Target != "" {
				files[u.Target] = true
			}
			if u.Errored {
				p.Trouble++
			}
			if !u.At.Before(p.Last) {
				p.Last, p.Doing = u.At, describe(u)
			}
		}
		// The agent's own account beats an inventory of its tool calls when it
		// is the more recent of the two: "the pooler needs session mode" says
		// more than "edit config.go", and it is the tender talking rather than
		// hugel guessing from a filename.
		for _, n := range s.Notes {
			if n.At.After(p.Last) && strings.TrimSpace(n.Text) != "" {
				p.Last, p.Doing = n.At, firstSentence(n.Text)
			}
		}
	}
	p.Files = len(files)
	return p, found
}

// describe renders one tool call the way a person reading over a shoulder
// would say it.
func describe(u transcript.ToolUse) string {
	// The first line, not the last: a heredoc or a multi-line script says what
	// it is doing at the top, and the tail of one is usually punctuation.
	target := u.Target
	if i := strings.IndexByte(target, '\n'); i >= 0 {
		target = target[:i]
	}
	target = strings.TrimSpace(target)
	// A command that starts by changing directory has spent its most readable
	// characters saying where it already is. The rest is the news.
	if rest, ok := strings.CutPrefix(target, "cd "); ok {
		if i := strings.IndexAny(rest, ";&"); i >= 0 {
			target = strings.TrimSpace(strings.TrimLeft(rest[i:], ";& "))
		}
	}
	verb := strings.ToLower(u.Name)
	switch u.Name {
	case "Bash":
		verb = "run"
	case "Edit", "Write", "NotebookEdit":
		verb = "edit"
	case "Read", "Grep", "Glob", "NotebookRead":
		verb = "read"
	}
	if target == "" {
		return verb
	}
	return fmt.Sprintf("%s %s", verb, target)
}

// firstSentence keeps a note to the part that says what is going on.
func firstSentence(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if i := strings.IndexAny(s, ".!?"); i > 0 {
		return s[:i]
	}
	return s
}
