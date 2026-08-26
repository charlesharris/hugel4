// Package beads reads work from bd.
//
// hugel does not track work. bd is the work model — beads, dependencies, a
// ready queue — and hugel is the knowledge and accounting model, so this reads
// and never writes. Anything hugel needs to change about a bead goes through bd
// itself, which keeps one tracker rather than two views that can disagree.
package beads

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
)

// ErrNoBd is returned when bd is not installed. It is not a failure: a bed
// without an issue tracker is a normal bed, and hugel still knows what it cost.
var ErrNoBd = errors.New("bd is not installed")

// Bead is one work item, as much of it as a garden view needs.
type Bead struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Body     string    `json:"description,omitempty"`
	Accept   string    `json:"acceptance_criteria,omitempty"`
	Labels   []string  `json:"labels,omitempty"`
	Type     string    `json:"issue_type"`
	Status   string    `json:"status"`
	Priority int       `json:"priority"`
	Assignee string    `json:"assignee,omitempty"`
	Updated  time.Time `json:"updated_at"`

	// Ready is bd's answer, not ours. Whether a bead can be started depends on
	// dependencies, defer dates and gates, and recomputing that here would let
	// hugel's queue drift from the one tenders actually pull from.
	Ready bool `json:"-"`
}

// NeedsAttention is the label for work waiting on a person rather than on an
// agent. A label rather than a status, because the bead genuinely is ready --
// for a human. bd ready should still show it; only the tender queue should not.
const NeedsAttention = "needs-attention"

// Labeled reports whether a bead carries a label.
func (b Bead) Labeled(name string) bool {
	for _, l := range b.Labels {
		if strings.EqualFold(l, name) {
			return true
		}
	}
	return false
}

// Blocked reports work that is open but cannot be started. bd stores it as
// open and derives the blockage from dependencies, so the same derivation
// belongs here rather than a fourth status nobody else uses.
func (b Bead) Blocked() bool { return b.Status == "open" && !b.Ready }

// Work is one bed's open work.
type Work struct {
	Bed   string
	Dir   string
	Beads []Bead
}

// Counts summarises a bed at a glance.
func (w Work) Counts() (ready, active, blocked int) {
	for _, b := range w.Beads {
		switch {
		case b.Status == "in_progress":
			active++
		case b.Ready:
			ready++
		case b.Blocked():
			blocked++
		}
	}
	return ready, active, blocked
}

// Read returns the open work in a bd repository.
//
// Two invocations rather than one: bd is asked which beads exist and separately
// which are ready. Deriving readiness from the dependency graph here would be
// one call cheaper and would eventually disagree with bd about defer dates and
// gates, which is the kind of drift that makes two systems worse than one.
func Read(dir string) (*Work, error) {
	w := &Work{Bed: filepath.Base(dir), Dir: dir}

	out, err := run(dir, "list", "--json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(out, &w.Beads); err != nil {
		return nil, fmt.Errorf("read beads in %s: %w", dir, err)
	}

	out, err = run(dir, "ready", "--json")
	if err != nil {
		return nil, err
	}
	var ready []Bead
	if err := json.Unmarshal(out, &ready); err != nil {
		return nil, fmt.Errorf("read ready beads in %s: %w", dir, err)
	}
	isReady := map[string]bool{}
	for _, b := range ready {
		isReady[b.ID] = true
	}
	for i := range w.Beads {
		w.Beads[i].Ready = isReady[w.Beads[i].ID]
	}

	sort.Slice(w.Beads, func(i, j int) bool {
		if w.Beads[i].Priority != w.Beads[j].Priority {
			return w.Beads[i].Priority < w.Beads[j].Priority
		}
		return w.Beads[i].ID < w.Beads[j].ID
	})
	return w, nil
}

func run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("bd", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("bd %s in %s: %s",
				strings.Join(args, " "), dir, strings.TrimSpace(string(ee.Stderr)))
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNoBd
		}
		return nil, fmt.Errorf("bd %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// RepoOf finds the bd repository a directory belongs to, walking up the way bd
// itself discovers one. An empty result means this directory tracks no work.
//
// A .beads directory is not enough to call it a project. bd keeps its global
// state in ~/.beads too, holding shared-server settings and formulas rather
// than issues, so an unqualified walk upward matches the home directory and
// every bed on the machine reports the same non-existent repository. The
// config bd init writes is the marker that distinguishes the two.
func RepoOf(dir string) string {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if isRepo(d) {
			return d
		}
		if parent := filepath.Dir(d); parent == d {
			return ""
		}
	}
}

func isRepo(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ".beads", "config.yaml"))
	return err == nil && !st.IsDir()
}

// Survey reads the work in every bed that tracks any, newest activity first.
//
// Beds without a bd repository are skipped rather than reported as empty: a
// project that has never tracked work is not a project with no work left.
// A bed whose repository fails to read is reported, because a tracker that is
// there and broken is worth knowing about.
func Survey(dirs map[string]string) ([]*Work, []error) {
	type result struct {
		w   *Work
		err error
	}
	results := make(chan result)
	n := 0
	for bed, dir := range dirs {
		repo := RepoOf(dir)
		if repo == "" {
			continue
		}
		n++
		go func(bed, repo string) {
			w, err := Read(repo)
			if w != nil {
				w.Bed = bed // the bed's name, not the repository directory's
			}
			results <- result{w, err}
		}(bed, repo)
	}

	var (
		out      []*Work
		problems []error
	)
	for i := 0; i < n; i++ {
		r := <-results
		if r.err != nil {
			problems = append(problems, r.err)
			continue
		}
		out = append(out, r.w)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, ai, _ := out[i].Counts()
		rj, aj, _ := out[j].Counts()
		if ai+ri != aj+rj {
			return ai+ri > aj+rj
		}
		return out[i].Bed < out[j].Bed
	})
	return out, problems
}

// Get returns one bead by id, read fresh from bd.
//
// Fresh rather than from anything hugel stored when the work started: a bead's
// acceptance criteria are the thing a review is answered against, and if they
// were amended while the work was in flight it is the amended ones that decide.
func Get(dir, id string) (*Bead, error) {
	out, err := run(dir, "show", id, "--json")
	if err != nil {
		return nil, err
	}
	var found []Bead
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("read bead %s: %w", id, err)
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no bead %s", id)
	}
	return &found[0], nil
}

// Claim marks a bead as being worked, through bd rather than in a status hugel
// keeps for itself. The queue a tender pulls from has to stay the queue every
// other reader sees, which is the whole reason hugel does not track work.
func Claim(dir, id string) error {
	_, err := run(dir, "update", id, "--claim")
	return err
}

// Close finishes a bead with a stated reason. The reason is not bookkeeping:
// beads closed with one are what the pile composts, so this is the sentence
// that becomes knowledge.
func Close(dir, id, reason string) error {
	_, err := run(dir, "close", id, "--reason", reason)
	return err
}

// HandBack returns a bead to a person: released from its claim, marked as
// needing attention, and carrying what the tender learned.
//
// One bd invocation so the three cannot half-happen. A bead released without
// its mark goes straight back to a tender, which hands the same wrong spec to
// another agent; a bead marked without its reason tells a person it needs them
// and not why.
func HandBack(dir, id, reason string) error {
	_, err := run(dir, "update", id,
		"--status", "open",
		"--add-label", NeedsAttention,
		"--append-notes", reason)
	return err
}

// Release puts a claimed bead back in the queue, for when the tender holding it
// died. Going through bd rather than editing a status hugel keeps means the
// bead is available to everything else that reads the queue, not just to hugel.
func Release(dir, id string) error {
	_, err := run(dir, "update", id, "--status", "open")
	return err
}

// Ready is one startable bead and the bed it belongs to.
type Ready struct {
	Bead Bead
	Work *Work
}

// Queue is what could be started now, across every bed, best first.
//
// Highest priority wins wherever it lives, so a P0 in a quiet bed is picked
// before a P3 in a busy one; ties break on bed then id so the order is stable
// between runs and two dispatches see the same queue.
//
// Epics are excluded. An epic is a container for work rather than work, and a
// tender handed one would try to do all of it at once.
func Queue(work []*Work, bed string, skip func(id string) bool) []Ready {
	var out []Ready
	for _, w := range work {
		if bed != "" && w.Bed != bed {
			continue
		}
		for _, b := range w.Beads {
			if !b.Ready || b.Status == "in_progress" || b.Type == "epic" {
				continue
			}
			// Work waiting on a person is not available to a tender, however
			// ready bd says it is.
			if b.Labeled(NeedsAttention) {
				continue
			}
			if skip != nil && skip(b.ID) {
				continue
			}
			out = append(out, Ready{Bead: b, Work: w})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Bead.Priority != b.Bead.Priority {
			return a.Bead.Priority < b.Bead.Priority
		}
		if a.Work.Bed != b.Work.Bed {
			return a.Work.Bed < b.Work.Bed
		}
		return a.Bead.ID < b.Bead.ID
	})
	return out
}
