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
