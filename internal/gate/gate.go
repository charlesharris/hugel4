// Package gate decides whether a tender's work lands.
//
// The cycle is work, test, review, test, commit. The second test is not a
// repetition: the first runs on the tender's branch, and the second runs on the
// branch merged with whatever main has become since the tender started. Work
// that passed in isolation and fails once merged is the ordinary case this
// catches, and it is the only test whose result is true of what gets pushed.
//
// Every stage can refuse, and a refusal leaves everything where it is. A failed
// gate is the most useful thing in the garden until somebody has read it, so
// nothing is cleaned up on the way out.
package gate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charris/hugel/internal/tender"
)

// Stage names the step a gate reached.
type Stage string

const (
	StageKind   Stage = "kind"   // this is work the gate can judge
	StageResult Stage = "result" // the tender said what it did
	StageTest   Stage = "test"   // tests pass on the branch
	StageReview Stage = "review" // a second agent read the diff
	StageMerge  Stage = "merge"  // the branch merged with current main
	StageRetest Stage = "retest" // tests pass on the merged tree
	StagePush   Stage = "push"   // the result reached the remote
	StageClose  Stage = "close"  // the bead was closed
)

// Report is what a gate did and where it stopped.
type Report struct {
	Bead    string
	Reached Stage
	Passed  bool

	// Refused means the gate declined to judge rather than judging and saying
	// no. Kept apart from Passed because everything downstream -- the exit
	// status, the words a person reads, what gets counted as failed work --
	// wants the difference, and a bare false cannot carry it.
	Refused bool

	Why    string
	Stages []StageResultRecord
}

// StageResultRecord is one stage's outcome.
type StageResultRecord struct {
	Stage  Stage
	OK     bool
	Detail string
	Took   time.Duration
}

// Options configures a run.
type Options struct {
	Tender tender.Tender

	// Test is the command that decides whether the work is sound. Empty means
	// discover it; a project that cannot be tested cannot be gated.
	Test string

	// Into is the branch the work lands on.
	Into string

	// Remote is pushed to once the merged tree passes. Empty pushes nothing,
	// which is the whole difference between a laptop and a shared repository.
	Remote string

	// DryRun stops after the review, before anything irreversible.
	//
	// It is its own field rather than an empty Into and Remote. Signalling
	// "do less" by leaving required values blank means every later stage has to
	// remember to check, and the first version of this merged the branch
	// against a ref named "".
	DryRun bool

	// Wait is how long to give the reviewing agent before giving up on it.
	Wait time.Duration

	// SkipPermissions runs the reviewer without prompts, for the same reason a
	// tender does: nobody is watching its pane either.
	SkipPermissions bool

	// Log receives progress, because a gate can take many minutes and silence
	// is indistinguishable from a hang.
	Log func(string)
}

func (o Options) say(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
}

// DiscoverTest finds how a repository runs its tests.
//
// Guessing wrong is safe in one direction only: a command that does not exist
// fails the gate, which is the correct answer for a project whose tests hugel
// cannot find. It must never silently pass.
func DiscoverTest(dir string) string {
	if b, err := os.ReadFile(filepath.Join(dir, "Makefile")); err == nil {
		if hasTarget(string(b), "test") {
			return "make test"
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go test ./..."
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return "npm test"
	}
	return ""
}

func hasTarget(makefile, target string) bool {
	for _, line := range strings.Split(makefile, "\n") {
		if strings.HasPrefix(line, target+":") {
			return true
		}
	}
	return false
}

// runTests runs the test command in a directory and keeps its output, which is
// the evidence a refusal rests on.
func runTests(dir, command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// tail keeps the end of a command's output, which is where a test runner says
// what failed.
func tail(s string, lines int) string {
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}
