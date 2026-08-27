package compost

import (
	"strings"
	"testing"
)

func TestOnlyCommitsGetFiles(t *testing.T) {
	d := &Digest{Records: []Record{
		{Kind: KindCommit, Subject: "the gate refuses a spike"},
		{Kind: KindMemory, Subject: "the pooler needs session mode"},
		{Kind: KindBeadClosed, Subject: "closed hugel4-x"},
	}}
	n := ResolveFiles(d, func(string) []string { return []string{"internal/gate/run.go"} })
	if n != 1 {
		t.Errorf("resolved %d records, want only the commit", n)
	}
	if len(d.Records[0].Files) != 1 {
		t.Error("the commit got no files")
	}
	// A memory is a claim about how things are, not a change that moved code.
	for _, r := range d.Records[1:] {
		if len(r.Files) != 0 {
			t.Errorf("%s record claims files %v", r.Kind, r.Files)
		}
	}
}

// A subject that resolves to no commit, or to more than one, must leave the
// record alone. Attaching another change's files would read as evidence.
func TestAnUnresolvedCommitKeepsNoFiles(t *testing.T) {
	d := &Digest{Records: []Record{{Kind: KindCommit, Subject: "ambiguous"}}}
	if n := ResolveFiles(d, func(string) []string { return nil }); n != 0 {
		t.Errorf("resolved %d, want 0", n)
	}
	if d.Records[0].Files != nil {
		t.Errorf("files = %v, want none", d.Records[0].Files)
	}
	if n := ResolveFiles(d, nil); n != 0 {
		t.Error("a nil resolver resolved something")
	}
}

func TestFilesAreCappedAndStable(t *testing.T) {
	var many []string
	for _, c := range "zyxwvutsrqponml" {
		many = append(many, "pkg/"+string(c)+".go")
	}
	d := &Digest{Records: []Record{{Kind: KindCommit, Subject: "s"}}}
	ResolveFiles(d, func(string) []string { return many })
	got := d.Records[0].Files
	if len(got) != maxFiles {
		t.Errorf("kept %d files, want the cap of %d", len(got), maxFiles)
	}
	// Sorted, so the same commit composts to the same entry every time rather
	// than to whatever git happened to list first.
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("files are not sorted: %v", got)
			break
		}
	}
}

// Where the change landed beats where the message said it landed. A message
// names a path when the author thought to; a commit names every path it moved.
func TestGitFilesBeatProseAndBecomeAreas(t *testing.T) {
	d := digestWith(Record{
		Kind:    KindCommit,
		Subject: "the gate declines to judge a spike rather than failing it",
		Body:    "This mentions internal/prose/only.go and nothing else.",
		Files:   []string{"internal/gate/run.go", "internal/gate/gate.go", "internal/cli/gate.go"},
	})
	h, err := Heuristic{}.Extract(d)
	if err != nil || len(h.Entries) == 0 {
		t.Fatalf("no entries: %v", err)
	}
	e := h.Entries[0]
	if len(e.Paths) != 2 {
		t.Errorf("paths = %v, want the two areas the commit touched", e.Paths)
	}
	for _, p := range e.Paths {
		if strings.Contains(p, ".go") {
			t.Errorf("paths carry a file rather than an area: %v", e.Paths)
		}
		if p == "internal/prose" {
			t.Errorf("prose outranked the commit: %v", e.Paths)
		}
	}
	// The files themselves survive as evidence, which is what evidence is for.
	if e.Evidence == nil || len(e.Evidence.Files) != 3 {
		t.Errorf("evidence files = %v, want the three the commit changed", e.Evidence)
	}
}

// A commit nobody could resolve still composts, and still says what its message
// said. This is the path every entry took before, and it must not regress.
func TestProseStillAnswersWhenGitCannot(t *testing.T) {
	d := digestWith(Record{
		Kind:    KindCommit,
		Subject: "drive: separate the parsers from the ioctls so the tree builds anywhere",
		Body:    "internal/drive kept its pure parsers beside the syscalls.",
	})
	h, _ := Heuristic{}.Extract(d)
	if len(h.Entries) == 0 {
		t.Fatal("no entries")
	}
	if got := h.Entries[0].Paths; len(got) == 0 || got[0] != "internal/drive" {
		t.Errorf("paths = %v, want the path the message named", got)
	}
}
