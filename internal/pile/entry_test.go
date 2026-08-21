package pile

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sample() *Entry {
	return &Entry{
		Type: Failure, Scope: ScopeBed,
		Title: "Neo4j routing rejects writes on read replicas",
		Body:  "Writes sent to a replica fail with a routing error.",
		Bed:   "hugel", Confidence: 0.8,
		Tags:   []string{"neo4j", "graph"},
		Source: Source{Extractor: "heuristic", ExtractorVersion: "1"},
	}
}

// Re-composting the same session must converge on the same entry rather than
// accumulating duplicates. Identity is bed + type + normalised title.
func TestIdentityIsStableAndIdempotent(t *testing.T) {
	a, b := sample(), sample()
	b.Title = "  neo4j Routing   Rejects Writes On Read Replicas  "
	a.Seal(time.Now())
	b.Seal(time.Now())
	if a.ID != b.ID {
		t.Errorf("title whitespace/case changed identity: %s vs %s", a.ID, b.ID)
	}

	c := sample()
	c.Body = "A completely different account of the same thing."
	c.Seal(time.Now())
	if c.ID != a.ID {
		t.Error("editing the body changed identity; it should only change content")
	}
	if c.ContentHash == a.ContentHash {
		t.Error("editing the body left the content hash unchanged")
	}
}

// A general entry is not owned by the bed it was learned in, or it would never
// reach any other bed's soil.
func TestGeneralScopeIsBedIndependent(t *testing.T) {
	a, b := sample(), sample()
	a.Scope, b.Scope = ScopeGeneral, ScopeGeneral
	a.Bed, b.Bed = "hugel", "hellbox"
	a.Seal(time.Now())
	b.Seal(time.Now())
	if a.ID != b.ID {
		t.Error("general entries learned in different beds got different identities")
	}
	if !strings.HasPrefix(a.Filename(), "entries/general/") {
		t.Errorf("general entry filed under a bed: %s", a.Filename())
	}

	c, d := sample(), sample()
	c.Bed, d.Bed = "hugel", "hellbox"
	c.Seal(time.Now())
	d.Seal(time.Now())
	if c.ID == d.ID {
		t.Error("bed-scoped entries in different beds collided")
	}
}

func TestSealDefaultsAndNormalises(t *testing.T) {
	e := &Entry{
		Type: Decision, Title: " a title ", Body: " a body ", Bed: "hugel",
		Tags:   []string{"b", "a", "b", "  "},
		Source: Source{Extractor: "heuristic", ExtractorVersion: "1"},
	}
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	e.Seal(now)

	if e.Status != Active || e.Review != Unreviewed || e.Scope != ScopeBed {
		t.Errorf("defaults not applied: %+v", e)
	}
	if e.Title != "a title" || e.Body != "a body" {
		t.Errorf("not trimmed: %q %q", e.Title, e.Body)
	}
	if len(e.Tags) != 2 || e.Tags[0] != "a" {
		t.Errorf("tags not deduped/sorted: %v", e.Tags)
	}
	if !e.OccurredAt.Equal(e.CreatedAt) {
		t.Error("OccurredAt should default to CreatedAt")
	}
}

// Imported entries carry the date they were earned, not the date they were
// imported, or the whole legacy corpus dates to migration day.
func TestOccurredAtSurvivesSeal(t *testing.T) {
	e := sample()
	earned := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
	e.OccurredAt = earned
	e.Seal(time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC))
	if !e.OccurredAt.Equal(earned) {
		t.Errorf("OccurredAt = %v, want %v", e.OccurredAt, earned)
	}
	if e.CreatedAt.Equal(earned) {
		t.Error("CreatedAt should be now, not the occurrence date")
	}
	if !strings.Contains(e.Filename(), "2026-03-18") {
		t.Errorf("filed under the wrong date: %s", e.Filename())
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*Entry)
		want string
	}{
		{"ok", func(*Entry) {}, ""},
		{"bad type", func(e *Entry) { e.Type = "musing" }, "unknown type"},
		{"bad scope", func(e *Entry) { e.Scope = "planet" }, "unknown scope"},
		{"no title", func(e *Entry) { e.Title = "  " }, "no title"},
		{"no body", func(e *Entry) { e.Body = "" }, "no body"},
		{"bed scope without bed", func(e *Entry) { e.Bed = "" }, "no bed"},
		{"confidence out of range", func(e *Entry) { e.Confidence = 1.5 }, "confidence"},
		{"no extractor", func(e *Entry) { e.Source.Extractor = "" }, "extractor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := sample()
			tt.mut(e)
			e.Seal(time.Now())
			if tt.name == "bad type" || tt.name == "bad scope" {
				tt.mut(e) // Seal would have defaulted an empty scope back
			}
			err := e.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want something about %q", err, tt.want)
			}
		})
	}
}

// The pile lives in git. Diffs should show what changed and nothing else.
func TestMarshalIsStableAndDiffFriendly(t *testing.T) {
	e := sample()
	e.Seal(time.Now())
	a, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := e.Marshal()
	if string(a) != string(b) {
		t.Error("marshalling is not deterministic")
	}
	if !strings.HasSuffix(string(a), "}\n") {
		t.Error("no trailing newline")
	}
	if !strings.Contains(string(a), "\n  \"id\":") {
		t.Error("not indented for readable diffs")
	}

	back, err := Unmarshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != e.ID || back.Title != e.Title || back.Source.Extractor != e.Source.Extractor {
		t.Error("round trip lost data")
	}
}

// Nothing that changes when an entry is merely read may appear in the file, or
// every soil lookup would dirty the repository.
func TestNoReadTimeStateInTheFile(t *testing.T) {
	e := sample()
	e.Seal(time.Now())
	b, _ := e.Marshal()
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"use_count", "last_used_at", "uses", "accessed_at", "score"} {
		if _, ok := raw[banned]; ok {
			t.Errorf("read-time state %q leaked into the pile file", banned)
		}
	}
}

func TestFilenameIsSafeAndReadable(t *testing.T) {
	e := sample()
	e.Title = "Neo4j / routing: rejects writes!! (on read replicas)"
	e.Seal(time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC))
	got := e.Filename()
	if strings.ContainsAny(got[len("entries/"):], " !()/:") != strings.Contains(got[len("entries/"):], "/") {
		t.Errorf("unsafe characters in filename: %s", got)
	}
	if !strings.HasPrefix(got, "entries/hugel/2026-08-21-neo4j-routing-rejects-writes") {
		t.Errorf("filename = %s", got)
	}
}
