// Package pile is hugel's knowledge store: typed entries composted out of
// sessions, shared across every bed.
//
// The files are the pile. Entries are JSON on disk in a git repository, and any
// index built over them is derived and disposable — rebuildable at any time by
// re-reading the files. That inversion is deliberate. An earlier Hugel made a
// graph database the source of truth, which meant the knowledge died with a
// container volume.
//
// Nothing that changes when an entry is merely *read* belongs in these files.
// Usage counts and last-touched timestamps live in the derived index, or every
// soil lookup would dirty the repository.
package pile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charris/hugel/internal/redact"
)

// Type is what kind of knowledge an entry carries. The taxonomy is small on
// purpose: five kinds a gardener can tell apart without thinking.
type Type string

const (
	Decision   Type = "decision"   // a choice that was made, and why
	Pattern    Type = "pattern"    // an approach that has proven to work
	Discovery  Type = "discovery"  // something learned that changes understanding
	Failure    Type = "failure"    // a wrong turn, and what it taught
	Constraint Type = "constraint" // a hard boundary that shapes design
)

// Scope decides whether an entry travels. A bed-scoped entry is about one
// project; a general one is about software. Without this distinction soil for
// one bed drowns in another bed's specifics, and the whole point of a shared
// pile is lost.
type Scope string

const (
	ScopeBed     Scope = "bed"
	ScopeGeneral Scope = "general"
)

// Status is the entry's standing as knowledge.
type Status string

const (
	Active     Status = "active"
	Superseded Status = "superseded"
	Abandoned  Status = "abandoned"
)

// Review is the entry's standing as *trusted* knowledge. Extraction is
// automated, so entries arrive unreviewed and soil can weight accordingly.
// This is the guardrail against a pile that poisons itself.
type Review string

const (
	Unreviewed Review = "unreviewed"
	Accepted   Review = "accepted"
	Rejected   Review = "rejected"
)

// Link is a typed relation to another entry.
type Link struct {
	Rel string `json:"rel"` // evolved_from, supersedes, contradicts, relates_to
	ID  string `json:"id"`
}

// Git locates an entry in repository history, so a reader can tell whether it
// predates a rewrite that invalidated it.
type Git struct {
	Branch string `json:"branch,omitempty"`
	SHA    string `json:"sha,omitempty"`
}

// Evidence anchors a claim to what was actually observed. Provenance is what
// separates knowledge from an assertion.
type Evidence struct {
	Commands []string `json:"commands,omitempty"`
	Files    []string `json:"files,omitempty"`
	Quote    string   `json:"quote,omitempty"`
}

// Source records where an entry came from and what it cost to make it.
//
// CostUSD is not bookkeeping. Composting spends tokens to save tokens, and an
// entry that cost more to extract than it will ever save is waste. Recording
// the price per entry is what makes that auditable rather than assumed.
type Source struct {
	Session          string       `json:"session,omitempty"`
	TranscriptSHA256 string       `json:"transcript_sha256,omitempty"`
	DigestSHA256     string       `json:"digest_sha256,omitempty"`
	Extractor        string       `json:"extractor"`
	ExtractorVersion string       `json:"extractor_version"`
	CostUSD          float64      `json:"cost_usd"`
	Redactions       []redact.Hit `json:"redactions,omitempty"`
	ImportedFrom     string       `json:"imported_from,omitempty"`
}

// Entry is one piece of composted knowledge.
type Entry struct {
	ID          string `json:"id"`
	ContentHash string `json:"content_hash"`

	Type  Type   `json:"type"`
	Scope Scope  `json:"scope"`
	Title string `json:"title"`
	Body  string `json:"body"`

	Bed        string  `json:"bed"`
	Status     Status  `json:"status"`
	Review     Review  `json:"review"`
	Confidence float64 `json:"confidence"`

	Tags  []string `json:"tags,omitempty"`
	Paths []string `json:"paths,omitempty"`
	Beads []string `json:"beads,omitempty"`
	Links []Link   `json:"links,omitempty"`

	Git *Git `json:"git,omitempty"`

	// OccurredAt is when the knowledge was earned; CreatedAt is when the entry
	// was written. They differ for backfilled and imported entries, and
	// conflating them would date the whole legacy corpus to import day.
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`

	Evidence *Evidence `json:"evidence,omitempty"`
	Source   Source    `json:"source"`
}

// Identity is what makes two extractions of the same insight the same entry:
// the bed, the kind, and the normalised title. Re-composting a session must
// converge rather than accumulate duplicates.
func (e *Entry) Identity() string {
	scope := string(e.Bed)
	if e.Scope == ScopeGeneral {
		scope = "general"
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + string(e.Type) + "\x00" + normaliseTitle(e.Title)))
	return hex.EncodeToString(sum[:])[:16]
}

// Content hashes the parts of an entry that carry meaning, so an edit is
// detectable without treating a status change as new knowledge.
func (e *Entry) Content() string {
	h := sha256.New()
	for _, s := range []string{
		string(e.Type), string(e.Scope), e.Title, e.Body, e.Bed,
		strings.Join(e.Tags, ","), strings.Join(e.Paths, ","), strings.Join(e.Beads, ","),
	} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Seal fills in the derived fields and normalises the entry for storage. It is
// the only place ID and ContentHash are set.
func (e *Entry) Seal(now time.Time) {
	if e.Status == "" {
		e.Status = Active
	}
	if e.Review == "" {
		e.Review = Unreviewed
	}
	if e.Scope == "" {
		e.Scope = ScopeBed
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now.UTC()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = e.CreatedAt
	}
	e.CreatedAt = e.CreatedAt.UTC().Truncate(time.Second)
	e.OccurredAt = e.OccurredAt.UTC().Truncate(time.Second)
	e.Title = strings.TrimSpace(e.Title)
	e.Body = strings.TrimSpace(e.Body)
	e.Tags = tidy(e.Tags)
	e.Paths = tidy(e.Paths)
	e.Beads = tidy(e.Beads)
	e.ID = e.Identity()
	e.ContentHash = e.Content()
}

// Validate reports why an entry may not be stored.
func (e *Entry) Validate() error {
	switch e.Type {
	case Decision, Pattern, Discovery, Failure, Constraint:
	default:
		return fmt.Errorf("unknown type %q", e.Type)
	}
	switch e.Scope {
	case ScopeBed, ScopeGeneral:
	default:
		return fmt.Errorf("unknown scope %q", e.Scope)
	}
	switch e.Status {
	case Active, Superseded, Abandoned:
	default:
		return fmt.Errorf("unknown status %q", e.Status)
	}
	switch e.Review {
	case Unreviewed, Accepted, Rejected:
	default:
		return fmt.Errorf("unknown review state %q", e.Review)
	}
	if strings.TrimSpace(e.Title) == "" {
		return fmt.Errorf("entry has no title")
	}
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Errorf("entry has no body")
	}
	if e.Scope == ScopeBed && e.Bed == "" {
		return fmt.Errorf("bed-scoped entry has no bed")
	}
	if e.Confidence < 0 || e.Confidence > 1 {
		return fmt.Errorf("confidence %v outside 0..1", e.Confidence)
	}
	if e.Source.Extractor == "" {
		return fmt.Errorf("entry has no extractor recorded")
	}
	return nil
}

// Marshal renders an entry for storage: stable field order, two-space indent,
// trailing newline. Git diffs on a pile should show what changed and nothing
// else.
func (e *Entry) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal entry %s: %w", e.ID, err)
	}
	return append(b, '\n'), nil
}

// Unmarshal reads an entry.
func Unmarshal(b []byte) (*Entry, error) {
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("unmarshal entry: %w", err)
	}
	return &e, nil
}

// Filename is where an entry lives, relative to the pile root: grouped by bed
// and named so the directory can be read by a human without a tool.
func (e *Entry) Filename() string {
	bed := e.Bed
	if e.Scope == ScopeGeneral {
		bed = "general"
	}
	if bed == "" {
		bed = "unsorted"
	}
	return fmt.Sprintf("entries/%s/%s-%s.json",
		slug(bed), e.OccurredAt.Format("2006-01-02"), slug(e.Title))
}

func normaliseTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func tidy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// slug renders text as a filename-safe fragment.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= 60 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
