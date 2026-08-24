// Package tender runs one bead of work unattended.
//
// Unattended, not headless. The agent runs as an ordinary interactive Claude
// Code session inside a detached tmux session, which is what keeps the work on
// a subscription rather than metered API billing. Nothing here drives the pane
// with send-keys: the tender is briefed by a file it is told to read, and it
// answers by writing a file back. Keystroke injection into a live agent is
// unreadable afterwards and races with whatever the agent is doing; a brief and
// a result are both durable and both reviewable.
package tender

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charris/hugel/internal/config"
)

// Tender is one bead being worked, and everything needed to find it again
// after hugel exits. hugel holds no state of its own while a tender runs: the
// tmux session is the process, the worktree is the work, and this file is how
// a later invocation reattaches to both.
type Tender struct {
	Bead     string    `json:"bead"`
	Bed      string    `json:"bed"`
	Title    string    `json:"title"`
	Repo     string    `json:"repo"`
	Worktree string    `json:"worktree"`
	Branch   string    `json:"branch"`
	Session  string    `json:"tmux_session"`
	Started  time.Time `json:"started"`
}

// now is the clock, replaced in tests.
var now = time.Now

// Dir is where a tender's brief, result and worktree live.
func Dir(bead string) (string, error) {
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "tenders", bead), nil
}

// BriefPath and ResultPath sit beside the worktree rather than inside it, so
// that a tender's paperwork never shows up as a change to the project.
func (t Tender) BriefPath() string  { return filepath.Join(filepath.Dir(t.Worktree), "brief.md") }
func (t Tender) ResultPath() string { return filepath.Join(filepath.Dir(t.Worktree), "result.md") }

// Done reports whether the tender has written its result.
func (t Tender) Done() bool {
	_, err := os.Stat(t.ResultPath())
	return err == nil
}

// Running reports whether the tmux session is still alive.
func (t Tender) Running() bool {
	return exec.Command("tmux", "has-session", "-t", t.Session).Run() == nil
}

// Attach is what a gardener types to look in on a tender.
func (t Tender) Attach() string { return "tmux attach -t " + t.Session }

// State describes a tender at a glance.
func (t Tender) State() string {
	switch {
	case t.Done():
		return "finished"
	case t.Running():
		return "working"
	default:
		return "stopped"
	}
}

// sessionName is the tmux handle for a bead. Dots are how tmux addresses panes
// within a window, and bd's hierarchical ids are full of them.
func sessionName(bead string) string {
	return "hugel-" + strings.NewReplacer(".", "-", ":", "-", " ", "-").Replace(bead)
}

// worktreeIn puts the worktree in a directory named for the bed rather than for
// the bead.
//
// A harness names a session's project by the basename of its working directory,
// and that name is what hugel calls a bed. A worktree at .../hugel4-sd7.4 would
// file the tender's whole transcript under a bed called "sd7.4" -- one new bed
// per bead, none of them the project. Nesting the worktree under the bead's
// directory and naming it for the bed keeps the accounting where it belongs.
func worktreeIn(dir, bed string) string { return filepath.Join(dir, bed) }

func (t Tender) save() error {
	dir := filepath.Dir(t.Worktree)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create tender dir: %w", err)
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "tender.json"), append(b, '\n'), 0o644)
}

// List returns every tender the garden has started, newest first.
func List() ([]Tender, error) {
	home, err := config.Home()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, "tenders")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tenders: %w", err)
	}
	var out []Tender
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "tender.json"))
		if err != nil {
			continue
		}
		var t Tender
		if err := json.Unmarshal(b, &t); err != nil {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.After(out[j].Started) })
	return out, nil
}

// Load returns one tender by bead id.
func Load(bead string) (*Tender, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		if t.Bead == bead {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("no tender for %s", bead)
}

var errNoTmux = errors.New("tmux is not installed")

func tmux(args ...string) error {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errNoTmux
		}
		return fmt.Errorf("tmux %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
