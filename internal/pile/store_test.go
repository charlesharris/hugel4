package pile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Init(t.TempDir())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func entry(title, body string) *Entry {
	return &Entry{
		Type: Decision, Scope: ScopeBed, Title: title, Body: body,
		Bed: "hugel", Confidence: 0.5,
		Source: Source{Extractor: "test", ExtractorVersion: "1"},
	}
}

// Composting the same session twice must converge. If it accumulated
// duplicates the pile would grow without learning anything.
func TestPutConverges(t *testing.T) {
	s := newStore(t)

	if got, err := s.Put(entry("a decision", "because")); err != nil || got != Created {
		t.Fatalf("first put = %v, %v", got, err)
	}
	if got, err := s.Put(entry("a decision", "because")); err != nil || got != Unchanged {
		t.Fatalf("identical put = %v, %v, want unchanged", got, err)
	}
	if got, err := s.Put(entry("a decision", "because of something new")); err != nil || got != Updated {
		t.Fatalf("edited put = %v, %v, want updated", got, err)
	}
	if n, _ := s.Count(); n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

// An unchanged entry must not be rewritten, or every compost run would show up
// as a diff in a repository that learned nothing.
func TestUnchangedPutLeavesTheFileAlone(t *testing.T) {
	s := newStore(t)
	e := entry("a decision", "because")
	if _, err := s.Put(e); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, e.Filename())
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := s.Put(entry("a decision", "because")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("unchanged entry was rewritten")
	}
}

// Extraction is automated and must not overrule a person. An entry someone
// reviewed or abandoned keeps that standing when re-composted.
func TestPutPreservesHumanJudgement(t *testing.T) {
	s := newStore(t)
	e := entry("a decision", "because")
	if _, err := s.Put(e); err != nil {
		t.Fatal(err)
	}

	e.Review = Accepted
	e.Status = Abandoned
	if err := s.write(e.Filename(), e); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Put(entry("a decision", "a revised account")); err != nil {
		t.Fatal(err)
	}
	all, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if all[0].Review != Accepted {
		t.Errorf("review = %q, want the human's %q", all[0].Review, Accepted)
	}
	if all[0].Status != Abandoned {
		t.Errorf("status = %q, want the human's %q", all[0].Status, Abandoned)
	}
	if !strings.Contains(all[0].Body, "revised") {
		t.Error("the new body was discarded along with the standing")
	}
}

// Two entries can want the same filename on the same day. Silently
// overwriting one would lose knowledge without saying so.
func TestFilenameCollisionKeepsBoth(t *testing.T) {
	s := newStore(t)
	a := entry("same title", "one")
	b := entry("same title", "two")
	b.Type = Failure // different identity, same slug and date
	if _, err := s.Put(a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(b); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Count(); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	all, _ := s.All()
	if all[0].Body == all[1].Body {
		t.Error("one entry overwrote the other")
	}
}

func TestGeneralEntriesAreNotFiledUnderABed(t *testing.T) {
	s := newStore(t)
	e := entry("something universally true", "about software")
	e.Scope = ScopeGeneral
	if _, err := s.Put(e); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "entries", "general")); err != nil {
		t.Errorf("general entry not filed under entries/general: %v", err)
	}
}

func TestOpenRejectsANonPile(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("Open accepted a directory that is not a pile")
	}
}

func TestInitIsSafeToRepeat(t *testing.T) {
	dir := t.TempDir()
	s, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(entry("a decision", "because")); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(dir); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	again, _ := Open(dir)
	if n, _ := again.Count(); n != 1 {
		t.Errorf("re-init lost entries: count = %d", n)
	}
}

// The pile hangs off the garden, so moving the garden moves the pile. It used
// to be built from the home directory directly, which meant a test with
// HUGEL_HOME set still wrote to the real pile.
func TestDefaultRootFollowsTheGarden(t *testing.T) {
	garden := t.TempDir()
	t.Setenv("HUGEL_HOME", garden)
	t.Setenv("HUGEL_PILE", "")

	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(garden, "pile"); got != want {
		t.Errorf("DefaultRoot = %q, want %q", got, want)
	}

	// An explicit pile still wins: it is the more specific instruction.
	elsewhere := t.TempDir()
	t.Setenv("HUGEL_PILE", elsewhere)
	if got, _ := DefaultRoot(); got != elsewhere {
		t.Errorf("DefaultRoot = %q, want HUGEL_PILE to win", got)
	}
}

// HUGEL_PILE is the one garden path that does not come through config.Home, so
// it is the one that has to ask for the sandbox by name. The pile was moved
// under the garden because a test nearly wrote to the real one; the explicit
// override is the hole that move left open.
func TestAnExplicitPileOutsideATempDirIsRefused(t *testing.T) {
	t.Setenv("HUGEL_PILE", filepath.Join(string(filepath.Separator), "var", "hugel-pile-is-not-here"))

	defer func() {
		if recover() == nil {
			t.Fatal("a non-temporary HUGEL_PILE was accepted")
		}
	}()
	DefaultRoot()
}
