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
	Spike    bool      `json:"spike,omitempty"`
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
//
// A tender with no session name is not running. The guard matters more than it
// looks: tmux reads an empty target as "no target given" and resolves it to the
// current session, so asking about a half-built Tender would answer for the
// session doing the asking -- and Stop would then kill it.
func (t Tender) Running() bool {
	if t.Session == "" {
		return false
	}
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

// SpikeAt names the spike that worked in a directory, if one did.
//
// A spike's session is recorded by the harness under the directory it ran in,
// which is its worktree -- so the worktree is the only join between a
// transcript and the bead it was exploring. Nothing else survives: the tmux
// session is gone by the time anyone composts, and the transcript records a
// path rather than a bead.
//
// An ordinary tender's worktree answers empty. Only a spike's findings are
// attributed, because only a spike's product is the finding itself.
func SpikeAt(dir string) string {
	if dir == "" {
		return ""
	}
	all, err := List()
	if err != nil {
		return ""
	}
	want := filepath.Clean(dir)
	for _, t := range all {
		if t.Spike && filepath.Clean(t.Worktree) == want {
			return t.Bead
		}
	}
	return ""
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

// Live returns tenders with an agent still working. A tender that has written
// its result is finished even if its pane is still open, because the result is
// what the garden goes by.
func Live() ([]Tender, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	var out []Tender
	for _, t := range all {
		if t.State() == "working" {
			out = append(out, t)
		}
	}
	return out, nil
}

// Stopped returns tenders whose agent is gone and which never wrote a result.
//
// This is the failure the pool has to notice. A tender that died holding a bead
// leaves the bead claimed and the slot spent, and nothing else in the system is
// watching for it: tmux does not know what the session was for, and bd does not
// know the claimant stopped existing.
func Stopped() ([]Tender, error) {
	all, err := List()
	if err != nil {
		return nil, err
	}
	var out []Tender
	for _, t := range all {
		if t.State() == "stopped" {
			out = append(out, t)
		}
	}
	return out, nil
}

// Exists reports whether a bead has been tended before, in any state. Starting
// a second tender on a bead that already has a worktree would have two agents
// writing to one branch.
func Exists(bead string) bool {
	_, err := Load(bead)
	return err == nil
}

// Outcome is the one word the tender was asked to lead its result with.
// A result that states none reads as "unstated", which is not "done": a tender
// that would not say how it went has not said it went well.
func (t Tender) Outcome() string {
	b, err := os.ReadFile(t.ResultPath())
	if err != nil {
		return ""
	}
	return outcomeIn(string(b))
}

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

// Reason is what the tender said about how it went: the outcome section, and
// whatever it left for a reviewer. This is the text that goes back onto the
// bead, so it is the tender's own words rather than a summary of them.
func (t Tender) Reason() string {
	b, err := os.ReadFile(t.ResultPath())
	if err != nil {
		return ""
	}
	doc := string(b)
	parts := []string{}
	for _, h := range []string{"## outcome", "## for the reviewer"} {
		if s := sectionIn(doc, h); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " — ")
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

// Handled reports whether the garden has already acted on this tender's result,
// so that a repeated dispatch does not hand the same bead back forever.
func (t Tender) Handled() bool {
	_, err := os.Stat(t.markPath())
	return err == nil
}

// MarkHandled records that a tender's outcome has been acted on.
func (t Tender) MarkHandled(what string) error {
	return os.WriteFile(t.markPath(), []byte(what+"\n"), 0o644)
}

func (t Tender) markPath() string { return filepath.Join(filepath.Dir(t.Worktree), "handled") }

// Archive moves a finished tender aside so a bead can be worked again.
//
// The evidence is kept rather than deleted: a run that went wrong is the most
// useful thing in the garden until somebody has read it, and the second attempt
// at a bead is exactly when the first one is worth reading. It is renamed
// rather than removed so the next tender gets a clean directory of its own.
func Archive(bead string) error {
	dir, err := Dir(bead)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	for n := 1; ; n++ {
		aside := fmt.Sprintf("%s.%d", dir, n)
		if _, err := os.Stat(aside); os.IsNotExist(err) {
			return os.Rename(dir, aside)
		}
	}
}
