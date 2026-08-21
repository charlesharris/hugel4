package compost

import (
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/pile"
)

func digestWith(records ...Record) *Digest {
	return &Digest{
		SessionID: "sess", Bed: "hellbox", Branch: "main",
		End: time.Date(2026, 8, 19, 23, 0, 0, 0, time.UTC), Records: records,
	}
}

func TestExtractFromCommits(t *testing.T) {
	d := digestWith(Record{
		Kind:    KindCommit,
		Subject: "drive: separate the parsers from the ioctls so the tree builds anywhere",
		Body:    "internal/drive kept its pure parsers beside the syscalls, so nothing built off Linux.",
	})
	h, err := Heuristic{}.Extract(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(h.Entries))
	}
	e := h.Entries[0]
	if e.Type != pile.Decision {
		t.Errorf("type = %q, want decision", e.Type)
	}
	if !strings.Contains(e.Title, "separate the parsers") {
		t.Errorf("title = %q", e.Title)
	}
	if len(e.Paths) == 0 || e.Paths[0] != "internal/drive" {
		t.Errorf("paths = %v, want the path named in the body", e.Paths)
	}
	if !e.OccurredAt.Equal(d.End) {
		t.Error("OccurredAt should come from the session, not the clock")
	}
	if e.Source.Session != "sess" || e.Source.Extractor != "heuristic" {
		t.Errorf("provenance = %+v", e.Source)
	}
	if e.Source.DigestSHA256 == "" {
		t.Error("entry is not traceable to the digest it came from")
	}
	if h.CostUSD != 0 {
		t.Errorf("cost = %v, want 0 for the free extractor", h.CostUSD)
	}
}

// A revert is the one commit shape that records a failure rather than a choice.
func TestRevertBecomesAFailure(t *testing.T) {
	h, _ := Heuristic{}.Extract(digestWith(Record{
		Kind: KindCommit, Subject: "Revert \"identify: trust the popularity score\"",
		Body: "It picked the wrong film for every disc older than 1990.", Revert: true,
	}))
	if len(h.Entries) != 1 || h.Entries[0].Type != pile.Failure {
		t.Fatalf("entries = %+v, want one failure", h.Entries)
	}
}

func TestTrivialCommitsAreSkipped(t *testing.T) {
	h, _ := Heuristic{}.Extract(digestWith(
		Record{Kind: KindCommit, Subject: "wip"},
		Record{Kind: KindCommit, Subject: "fixup! earlier thing"},
		Record{Kind: KindCommit, Subject: "Merge branch 'main' into feature"},
		Record{Kind: KindCommit, Subject: "typo"},
	))
	if len(h.Entries) != 0 {
		t.Errorf("kept %d trivial commits: %+v", len(h.Entries), h.Entries)
	}
}

// The first scope heuristic marked 89 of 138 real entries general, including
// dense project-specific notes whose only crime was arguing a decision without
// citing a filename. Absence of evidence of bed-specificity is not evidence of
// generality, and guessing wrong in that direction sends an entry into every
// other bed's soil.
func TestHeuristicNeverProposesGeneralScope(t *testing.T) {
	records := []Record{
		{Kind: KindCommit, Subject: "keep the reader's place, and say which keys move them",
			Body: "The removed viewer stepped stops with arrow keys; the theme steps with J/K."},
		{Kind: KindCommit, Subject: "a decision that names no file at all",
			Body: "Nothing here cites a path or the project."},
	}
	h, _ := Heuristic{}.Extract(digestWith(records...))
	for _, e := range h.Entries {
		if e.Scope != pile.ScopeBed {
			t.Errorf("scope = %q for %q; a regex cannot judge whether knowledge travels",
				e.Scope, e.Title)
		}
	}
}

// A subject alone says what changed but never why, and should not be trusted
// like one that explains itself.
func TestConfidenceReflectsEvidence(t *testing.T) {
	h, _ := Heuristic{}.Extract(digestWith(
		Record{Kind: KindCommit, Subject: "a subject with no body at all here"},
		Record{Kind: KindCommit, Subject: "a subject with a body attached", Body: "and the reasoning"},
	))
	if len(h.Entries) != 2 {
		t.Fatalf("got %d entries", len(h.Entries))
	}
	if h.Entries[0].Confidence >= h.Entries[1].Confidence {
		t.Errorf("bodyless commit rated %v, explained one %v",
			h.Entries[0].Confidence, h.Entries[1].Confidence)
	}
}

func TestExtractFromBeadRecords(t *testing.T) {
	h, _ := Heuristic{}.Extract(digestWith(Record{
		Kind: KindBeadClosed, Bead: "hb-qis",
		Subject: "Parsers split out of the _linux files",
		Body:    "Parsers split out of the _linux files into sense.go and region.go.",
	}))
	if len(h.Entries) != 1 {
		t.Fatalf("got %d entries", len(h.Entries))
	}
	if got := h.Entries[0].Beads; len(got) == 0 || got[0] != "hb-qis" {
		t.Errorf("beads = %v, want the bead it closed", got)
	}
}

func TestDigestSHAIsStableAndInputSpecific(t *testing.T) {
	a := digestWith(Record{Kind: KindCommit, Subject: "one"})
	b := digestWith(Record{Kind: KindCommit, Subject: "two"})
	if a.SHA256() != a.SHA256() {
		t.Error("digest hash is not stable")
	}
	if a.SHA256() == b.SHA256() {
		t.Error("different digests hashed the same")
	}
}
