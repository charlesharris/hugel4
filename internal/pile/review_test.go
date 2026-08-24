package pile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func stored(t *testing.T, s *Store, e *Entry) *Entry {
	t.Helper()
	if _, err := s.Put(e); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return e
}

// Judging an entry is not a claim about its subject, so it must not disturb the
// entry's identity or its content hash. If it did, the next compost run would
// read the entry as changed knowledge and overwrite the judgement.
func TestReviewLeavesIdentityAlone(t *testing.T) {
	s := newStore(t)
	e := stored(t, s, entry("a decision", "because"))
	id, hash := e.ID, e.ContentHash

	got, res, err := s.SetReview(id, Accepted)
	if err != nil || res != Updated {
		t.Fatalf("SetReview = %v, %v", res, err)
	}
	if got.Review != Accepted {
		t.Errorf("review = %q, want accepted", got.Review)
	}
	if got.ID != id || got.ContentHash != hash {
		t.Errorf("identity moved: %s/%s -> %s/%s", id, hash, got.ID, got.ContentHash)
	}

	reread, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Review != Accepted {
		t.Errorf("reread review = %q, want it on disk", reread.Review)
	}
}

// Re-reviewing what you reviewed last week must leave the repository clean, for
// the same reason an unchanged compost run does: a diff should mean something
// happened.
func TestReReviewIsNotAWrite(t *testing.T) {
	s := newStore(t)
	e := stored(t, s, entry("a decision", "because"))
	if _, _, err := s.SetReview(e.ID, Rejected); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(s.Root, e.Filename())
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	got, res, err := s.SetReview(e.ID, Rejected)
	if err != nil {
		t.Fatal(err)
	}
	if res != Unchanged {
		t.Errorf("second review = %q, want unchanged", res)
	}
	if got.Review != Rejected {
		t.Errorf("review = %q, want rejected", got.Review)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("re-reviewing rewrote the file")
	}
}

// A rejected entry must survive the next compost run still rejected. Extraction
// is automated and will re-propose what a human already threw out; if that
// resurrected it, review would be worthless.
func TestRejectionSurvivesRecompost(t *testing.T) {
	s := newStore(t)
	e := stored(t, s, entry("a decision", "because"))
	if _, _, err := s.SetReview(e.ID, Rejected); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Put(entry("a decision", "because, restated at length")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Review != Rejected {
		t.Errorf("review = %q after recompost, want rejected", got.Review)
	}
	if got.Body == "because" {
		t.Error("body was not updated; only the judgement should be preserved")
	}
}

// Superseding sinks the old entry rather than deleting it: it was true once,
// and the link on the newer entry is how a reader gets from one to the other.
func TestSupersedeSinksAndLinks(t *testing.T) {
	s := newStore(t)
	old := stored(t, s, entry("the old way", "we did it like this"))
	newer := stored(t, s, entry("the new way", "now we do it like that"))

	gotOld, gotNew, err := s.Supersede(old.ID, newer.ID)
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	if gotOld.Status != Superseded {
		t.Errorf("old status = %q, want superseded", gotOld.Status)
	}
	if !gotNew.linked("supersedes", old.ID) {
		t.Errorf("newer entry links = %v, want a supersedes link to %s", gotNew.Links, old.ID)
	}

	// The old entry stays readable, and the link is on disk.
	reread, err := s.Get(newer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reread.linked("supersedes", old.ID) {
		t.Error("supersedes link was not persisted")
	}
	if n, _ := s.Count(); n != 2 {
		t.Errorf("count = %d, want both entries kept", n)
	}

	// Doing it twice must not accumulate duplicate links.
	if _, gotNew, err = s.Supersede(old.ID, newer.ID); err != nil {
		t.Fatal(err)
	}
	if len(gotNew.Links) != 1 {
		t.Errorf("links = %v, want one", gotNew.Links)
	}
}

func TestSupersedeRejectsItself(t *testing.T) {
	s := newStore(t)
	e := stored(t, s, entry("a decision", "because"))
	if _, _, err := s.Supersede(e.ID, e.ID); err == nil {
		t.Error("an entry superseded itself")
	}
}

// Ids are shown truncated wherever a gardener meets one, so a prefix has to
// work — and an ambiguous prefix has to fail loudly rather than pick.
func TestGetByPrefix(t *testing.T) {
	s := newStore(t)
	e := stored(t, s, entry("a decision", "because"))

	got, err := s.Get(e.ID[:6])
	if err != nil || got.ID != e.ID {
		t.Fatalf("Get(prefix) = %v, %v", got, err)
	}
	if _, err := s.Get(""); err == nil {
		t.Error("empty prefix matched something")
	}
	if _, err := s.Get("nosuchentry"); err == nil {
		t.Error("missing id did not error")
	}
}

func TestGetRejectsAmbiguousPrefix(t *testing.T) {
	s := newStore(t)
	// Identity is a hash, so a shared prefix has to be found rather than
	// written: store entries until two of them begin with the same character.
	seen := map[byte]bool{}
	var shared string
	for i := 0; i < 200 && shared == ""; i++ {
		e := stored(t, s, entry(fmt.Sprintf("decision %d", i), "because"))
		if seen[e.ID[0]] {
			shared = e.ID[:1]
		}
		seen[e.ID[0]] = true
	}
	if shared == "" {
		t.Fatal("no two entries shared a leading character")
	}
	if _, err := s.Get(shared); err == nil {
		t.Errorf("prefix %q matched several entries and still picked a winner", shared)
	}
}

// Status and review answer different questions. An approach can be abandoned
// while the entry recording it stays perfectly true.
func TestAbandonIsNotRejection(t *testing.T) {
	s := newStore(t)
	e := stored(t, s, entry("a decision", "because"))
	got, _, err := s.SetStatus(e.ID, Abandoned)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Abandoned {
		t.Errorf("status = %q, want abandoned", got.Status)
	}
	if got.Review != Unreviewed {
		t.Errorf("review = %q, want abandoning to leave it alone", got.Review)
	}
}
