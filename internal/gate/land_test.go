package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repo builds a git repository with one commit and returns its path.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "gate@test"},
		{"config", "user.name", "gate"},
	} {
		if out, err := git(dir, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	commit(t, dir, "a", "first")
	return dir
}

func commit(t *testing.T, dir, file, msg string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(msg), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := git(dir, "add", "."); err != nil {
		t.Fatalf("add: %v %s", err, out)
	}
	if out, err := git(dir, "commit", "-q", "-m", msg); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	return at(t, dir, "HEAD")
}

func at(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := git(dir, "rev-parse", ref)
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(out)
}

// TestALandingThatWouldDiscardWorkIsRefused reproduces the run that lost three
// commits.
//
// main had work the branch never saw, because the gate merged the remote's copy
// of main rather than main. The merge was a no-op and reported "clean", and the
// landing then moved main back onto the branch tip. Every stage said ok. This
// is that shape: a head that does not descend from where the branch is.
func TestALandingThatWouldDiscardWorkIsRefused(t *testing.T) {
	dir := repo(t)
	forked := at(t, dir, "HEAD")

	// The tender's branch, off the fork point.
	if out, err := git(dir, "checkout", "-q", "-b", "hugel/x-1"); err != nil {
		t.Fatalf("branch: %v %s", err, out)
	}
	branchHead := commit(t, dir, "tender", "what the tender did")

	// Meanwhile main gains work of its own.
	if out, err := git(dir, "checkout", "-q", "main"); err != nil {
		t.Fatalf("checkout: %v %s", err, out)
	}
	mainHead := commit(t, dir, "other", "work the branch never saw")
	if mainHead == forked {
		t.Fatal("main did not move; the test proves nothing")
	}

	_, err := land(dir, "main", branchHead)
	if err == nil {
		t.Fatal("landing was allowed to discard main's commits")
	}
	if !strings.Contains(err.Error(), "discard work") {
		t.Errorf("refusal does not say what is at stake: %v", err)
	}
	if now := at(t, dir, "main"); now != mainHead {
		t.Errorf("main moved anyway: %s, want %s", now, mainHead)
	}
}

// TestAFastForwardLands is the ordinary case, and has to keep working: a guard
// that refuses everything is not a guard.
func TestAFastForwardLands(t *testing.T) {
	dir := repo(t)
	base := at(t, dir, "main")
	if out, err := git(dir, "checkout", "-q", "-b", "hugel/x-1"); err != nil {
		t.Fatalf("branch: %v %s", err, out)
	}
	head := commit(t, dir, "tender", "what the tender did")
	if out, err := git(dir, "checkout", "-q", "main"); err != nil {
		t.Fatalf("checkout: %v %s", err, out)
	}

	got, err := land(dir, "main", head)
	if err != nil {
		t.Fatalf("a fast-forward was refused: %v", err)
	}
	if got != base {
		t.Errorf("reported base %s, want %s", got, base)
	}
	if now := at(t, dir, "main"); now != head {
		t.Errorf("main is at %s, want %s", now, head)
	}
}

// TestLandingIsCompareAndSwap: a branch that moved while the gate was testing
// is a refusal rather than a silent overwrite. update-ref's three-argument form
// is what does this, so the test pins that it is being used.
func TestLandingIsCompareAndSwap(t *testing.T) {
	dir := repo(t)
	if out, err := git(dir, "checkout", "-q", "-b", "hugel/x-1"); err != nil {
		t.Fatalf("branch: %v %s", err, out)
	}
	head := commit(t, dir, "tender", "what the tender did")
	if out, err := git(dir, "checkout", "-q", "main"); err != nil {
		t.Fatalf("checkout: %v %s", err, out)
	}
	stale := at(t, dir, "main")

	// Someone else's landing, between the read and the write.
	if out, err := git(dir, "update-ref", "refs/heads/main", head); err != nil {
		t.Fatalf("move main: %v %s", err, out)
	}
	if out, err := git(dir, "update-ref", "refs/heads/main", stale, head); err != nil {
		t.Fatalf("put it back: %v %s", err, out)
	}

	// The swap itself: update-ref must refuse when the old value is wrong.
	if out, err := git(dir, "update-ref", "refs/heads/main", head, strings.Repeat("0", 40)); err == nil {
		t.Errorf("update-ref accepted a wrong old value: %s", out)
	}
}

// TestIsAncestorTellsNoFromBroken. merge-base --is-ancestor answers "no" with
// exit 1, which is indistinguishable from a failed call unless the code looks.
// Reading a definite no as an error would turn every refusal into a crash.
func TestIsAncestorTellsNoFromBroken(t *testing.T) {
	dir := repo(t)
	first := at(t, dir, "HEAD")
	second := commit(t, dir, "b", "second")

	if ok, err := isAncestor(dir, first, second); err != nil || !ok {
		t.Errorf("first should be an ancestor of second: %v %v", ok, err)
	}
	if ok, err := isAncestor(dir, second, first); err != nil || ok {
		t.Errorf("second is not an ancestor of first, and that is not an error: %v %v", ok, err)
	}
	if _, err := isAncestor(dir, "no-such-ref", second); err == nil {
		t.Error("a bad ref should be an error, not a quiet false")
	}
}

// TestLandRefusesAnUnknownBranch, rather than creating it. A typo in --into
// would otherwise make a branch and report a landing.
func TestLandRefusesAnUnknownBranch(t *testing.T) {
	dir := repo(t)
	head := at(t, dir, "HEAD")
	if _, err := land(dir, "no-such-branch", head); err == nil {
		t.Fatal("landed on a branch that does not exist")
	}
	if out, _ := git(dir, "rev-parse", "--verify", "-q", "refs/heads/no-such-branch"); strings.TrimSpace(out) != "" {
		t.Error("the branch was created by the attempt")
	}
}

// TestGatingIsRefusedWhenTheBaseIsBehindItsRemote. Landing onto a branch that
// is behind means testing against one base and pushing to another, which is
// the shape that lost three commits.
func TestGatingIsRefusedWhenTheBaseIsBehindItsRemote(t *testing.T) {
	dir := repo(t)

	// A stand-in for the remote's copy of main, one commit ahead.
	if out, err := git(dir, "checkout", "-q", "-b", "origin/main"); err != nil {
		t.Fatalf("branch: %v %s", err, out)
	}
	commit(t, dir, "theirs", "somebody else's work")
	if out, err := git(dir, "checkout", "-q", "main"); err != nil {
		t.Fatalf("checkout: %v %s", err, out)
	}

	err := checkBase(dir, "main", "origin/main")
	if err == nil {
		t.Fatal("gating was allowed onto a base behind its remote")
	}
	if !strings.Contains(err.Error(), "pull it before gating") {
		t.Errorf("refusal does not say what to do: %v", err)
	}
}

// TestBeingAheadOfTheRemoteIsNormal. This is the state the incident happened
// in, and it must not become a refusal: unpushed work is ordinary.
func TestBeingAheadOfTheRemoteIsNormal(t *testing.T) {
	dir := repo(t)
	if out, err := git(dir, "branch", "origin/main"); err != nil {
		t.Fatalf("branch: %v %s", err, out)
	}
	commit(t, dir, "mine", "not pushed yet")

	if err := checkBase(dir, "main", "origin/main"); err != nil {
		t.Errorf("being ahead of the remote was refused: %v", err)
	}
}

// TestABranchNeverPushedHasNothingToBeBehind.
func TestABranchNeverPushedHasNothingToBeBehind(t *testing.T) {
	dir := repo(t)
	if err := checkBase(dir, "main", "origin/main"); err != nil {
		t.Errorf("a missing remote ref was treated as a refusal: %v", err)
	}
}
