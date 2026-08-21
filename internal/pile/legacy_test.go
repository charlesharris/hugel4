package pile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const legacyEntry = `---
title: Neo4j routing rejects writes on read replicas
type: failure
date: 2026-03-18T00:00:00Z
bed: hugel
status: active
tags:
    - neo4j
    - graph
---
Writes sent to a replica fail with a routing error.

## Why

The driver resolves the routing table to a reader.
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadLegacy(t *testing.T) {
	e, err := ReadLegacy(writeTemp(t, "2026-03-18-neo4j.md", legacyEntry))
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Neo4j routing rejects writes on read replicas" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Type != Failure || e.Bed != "hugel" || e.Status != Active {
		t.Errorf("fields = %+v", e)
	}
	if len(e.Tags) != 2 || e.Tags[0] != "neo4j" {
		t.Errorf("tags = %v", e.Tags)
	}
	if !e.OccurredAt.Equal(time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("occurred = %v", e.OccurredAt)
	}
	// The body keeps its markdown, headings and all.
	if want := "Writes sent to a replica"; e.Body[:len(want)] != want {
		t.Errorf("body starts %q", e.Body[:40])
	}
	if !contains(e.Body, "## Why") {
		t.Error("body lost its structure")
	}
	// Imported knowledge is not more trustworthy for being old.
	if e.Review != Unreviewed {
		t.Errorf("review = %q, want unreviewed", e.Review)
	}
	if e.Source.ImportedFrom == "" {
		t.Error("provenance not recorded")
	}
}

// The corpus spans three format generations; both list styles appear.
func TestFrontMatterListStyles(t *testing.T) {
	inline := `---
title: t
type: pattern
bed: b
status: active
tags: [alpha, beta]
---
body`
	e, err := ReadLegacy(writeTemp(t, "a.md", inline))
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Tags) != 2 || e.Tags[0] != "alpha" || e.Tags[1] != "beta" {
		t.Errorf("inline tags = %v", e.Tags)
	}
}

func TestLegacyDateFormats(t *testing.T) {
	for _, in := range []string{"2026-03-18T00:00:00Z", "2026-03-18", "2026/03/18"} {
		got, ok := parseLegacyDate(in)
		if !ok {
			t.Errorf("parseLegacyDate(%q) failed", in)
			continue
		}
		if got.Year() != 2026 || got.Month() != 3 || got.Day() != 18 {
			t.Errorf("parseLegacyDate(%q) = %v", in, got)
		}
	}
	if _, ok := parseLegacyDate("not a date"); ok {
		t.Error("accepted nonsense as a date")
	}
}

func TestReadLegacyRejectsFilesWithoutFrontMatter(t *testing.T) {
	if _, err := ReadLegacy(writeTemp(t, "a.md", "just a note")); err == nil {
		t.Error("accepted a file with no front matter")
	}
	if _, err := ReadLegacy(writeTemp(t, "b.md", "---\ntitle: x\nno terminator")); err == nil {
		t.Error("accepted unterminated front matter")
	}
}

// A malformed file must not sink an import of a hundred good ones.
func TestImportLegacyDirSurvivesBadFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.md"), []byte(legacyEntry), 0o644)
	os.WriteFile(filepath.Join(dir, "bad.md"), []byte("no front matter"), 0o644)
	os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("not markdown"), 0o644)

	entries, problems, err := ImportLegacyDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("imported %d entries, want 1", len(entries))
	}
	if len(problems) != 1 {
		t.Errorf("reported %d problems, want 1", len(problems))
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
