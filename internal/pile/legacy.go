package pile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Legacy entries are markdown with a small YAML front matter block, the format
// two earlier Hugels used:
//
//	---
//	title: Neo4j routing rejects writes
//	type: decision
//	date: 2026-03-18T00:00:00Z
//	bed: hugel
//	status: active
//	tags:
//	    - neo4j
//	---
//	body text
//
// The subset in use is narrow enough to read directly. Taking a YAML dependency
// to parse six scalar keys and one list would cost more than it is worth.

// ReadLegacy parses one legacy markdown entry.
func ReadLegacy(path string) (*Entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read legacy entry: %w", err)
	}
	front, body, err := splitFrontMatter(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	e := &Entry{
		Title:  front["title"],
		Body:   strings.TrimSpace(body),
		Bed:    front["bed"],
		Type:   Type(strings.ToLower(front["type"])),
		Scope:  ScopeBed,
		Status: Status(strings.ToLower(front["status"])),
		// Imported knowledge is not more trustworthy than composted knowledge,
		// and much of it is a year old. It arrives unreviewed like everything
		// else, and confidence is left at the midpoint rather than invented.
		Review:     Unreviewed,
		Confidence: 0.5,
		Tags:       front.list("tags"),
		Source: Source{
			Extractor:        "legacy-import",
			ExtractorVersion: "1",
			ImportedFrom:     filepath.Base(path),
		},
	}
	if e.Status == "" {
		e.Status = Active
	}
	if when, ok := parseLegacyDate(front["date"]); ok {
		e.OccurredAt = when
	}
	return e, nil
}

// ImportLegacyDir reads every legacy markdown entry in a directory.
func ImportLegacyDir(dir string) ([]*Entry, []error, error) {
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read legacy dir: %w", err)
	}
	var (
		out      []*Entry
		problems []error
	)
	for _, n := range names {
		if n.IsDir() || filepath.Ext(n.Name()) != ".md" {
			continue
		}
		e, err := ReadLegacy(filepath.Join(dir, n.Name()))
		if err != nil {
			problems = append(problems, err)
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurredAt.Before(out[j].OccurredAt) })
	return out, problems, nil
}

type frontMatter map[string]string

// list returns a key stored as a YAML block sequence.
func (f frontMatter) list(key string) []string {
	v := f[key]
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, "\n") {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "-"))
		part = strings.Trim(part, `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitFrontMatter(s string) (frontMatter, string, error) {
	s = strings.TrimPrefix(s, "\ufeff") // byte order mark
	if !strings.HasPrefix(s, "---\n") {
		return nil, "", fmt.Errorf("no front matter")
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, "", fmt.Errorf("unterminated front matter")
	}
	head, body := rest[:end], rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")

	front := frontMatter{}
	key := ""
	for _, line := range strings.Split(head, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// A continuation line belongs to the key above it.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "-") {
			if key != "" {
				front[key] += "\n" + strings.TrimSpace(line)
			}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// An inline list, the other style these files use.
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			v = strings.ReplaceAll(strings.Trim(v, "[]"), ",", "\n")
		}
		front[key] = strings.Trim(v, `"'`)
	}
	return front, body, nil
}

func parseLegacyDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
