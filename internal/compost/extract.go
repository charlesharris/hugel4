package compost

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/charris/hugel/internal/pile"
)

// Extractor turns a digest into pile entries. The interface exists so that a
// model-backed extractor can be compared against the free one rather than
// assumed better: whatever it costs, it has to beat this baseline on entries a
// gardener actually keeps.
type Extractor interface {
	Name() string
	Version() string
	Extract(d *Digest) (Harvest, error)
}

// Harvest is what one composting run produced and what it cost.
type Harvest struct {
	Entries []*pile.Entry
	CostUSD float64
}

// Heuristic extracts only from deliberate records — commit messages and beads
// closed with a reason. It reads what someone already chose to write down
// rather than inferring knowledge from prose.
//
// That is a narrow catch on purpose. Agent notes are richer and would yield
// more entries, but prose is where an extractor invents things, and an invented
// entry in a permanent cross-project pile is worse than a missing one. Widen
// this only with evidence that the wider catch is kept rather than deleted.
type Heuristic struct{}

func (Heuristic) Name() string    { return "heuristic" }
func (Heuristic) Version() string { return "1" }

func (h Heuristic) Extract(d *Digest) (Harvest, error) {
	var out []*pile.Entry
	digest := d.SHA256()

	for _, r := range d.Records {
		if r.Kind == KindCommit && r.trivial() {
			continue
		}
		body := r.Body
		if strings.TrimSpace(body) == "" {
			body = r.Subject
		}
		text := r.Subject + "\n" + body

		e := &pile.Entry{
			Type:       recordType(r),
			Scope:      proposeScope(text, d.Bed),
			Title:      cleanTitle(r.Subject),
			Body:       body,
			Bed:        d.Bed,
			Confidence: confidence(r),
			Paths:      pathsIn(text),
			Beads:      mergeBeads(r.Bead, beadsIn(text)),
			OccurredAt: d.End,
			Evidence: &pile.Evidence{
				Quote: r.Subject,
				Files: pathsIn(text),
			},
			Source: pile.Source{
				Session:          d.SessionID,
				DigestSHA256:     digest,
				Extractor:        h.Name(),
				ExtractorVersion: h.Version(),
				Redactions:       d.Redactions,
				Spike:            d.Spike,
			},
		}
		if d.Branch != "" {
			e.Git = &pile.Git{Branch: d.Branch}
		}
		if e.Title == "" || e.Body == "" {
			continue
		}
		out = append(out, e)
	}
	// Extraction from records is free. Recording that explicitly matters: an
	// extractor that costs more than the soil it enables is waste, and the
	// comparison needs a zero to start from.
	return Harvest{Entries: out, CostUSD: 0}, nil
}

func recordType(r Record) pile.Type {
	if r.Kind == KindMemory {
		// A memory says how something is, not what was chosen. Typing it as a
		// decision would credit it with a deliberation that never happened.
		// Discovery is the honest default; review is where a constraint or a
		// pattern gets recognised as one.
		return pile.Discovery
	}
	if r.Revert {
		// A revert is the one commit shape that records a failure rather than a
		// choice: something was tried, and taken back.
		return pile.Failure
	}
	return pile.Decision
}

func confidence(r Record) float64 {
	if r.Kind == KindMemory {
		// Deliberately written to outlive its session, but asserted rather than
		// demonstrated: no diff, no closed work, and possibly no human.
		return 0.5
	}
	if strings.TrimSpace(r.Body) == "" {
		return 0.4 // a subject alone states what, never why
	}
	return 0.6
}

// conventionalPrefix matches the "scope:" that opens many commit subjects.
var conventionalPrefix = regexp.MustCompile(`^[a-z][a-z0-9_\-/]{1,20}(\([a-z0-9_\-/]+\))?!?:\s+`)

// cleanTitle makes a record's subject read as a claim rather than an
// instruction. The conventional-commit prefix is kept where it names a
// subsystem, because "drive: parsers split from ioctls" locates the knowledge.
func cleanTitle(s string) string {
	s = collapse(s)
	if len(s) > 110 {
		if i := strings.LastIndex(s[:110], " "); i > 50 {
			s = s[:i] + "…"
		} else {
			s = s[:110] + "…"
		}
	}
	return strings.TrimSpace(s)
}

// pathish matches tokens that look like a file in a repository.
var pathish = regexp.MustCompile(`\b[\w.\-/]+\.(?:go|rb|js|ts|tsx|py|rs|ex|exs|sql|md|toml|yaml|yml|json|sh|mod)\b|\b(?:internal|cmd|lib|app|src|pkg|test|spec)/[\w.\-/]+`)

func pathsIn(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range pathish.FindAllString(s, -1) {
		m = strings.Trim(m, ".,;:")
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

// proposeScope decides whether knowledge travels beyond the bed it was learned
// in. This extractor always says no.
//
// The first version guessed: general when a record mentioned no repository file
// and never named its bed. Run against real sessions it marked 89 of 138
// entries general, including dense project-specific design notes whose only
// crime was discussing a decision without naming a filename. The reasoning was
// backwards -- absence of evidence of bed-specificity is not evidence of
// generality, and it is the common case, because prose about a design usually
// argues rather than cites.
//
// A regex cannot judge whether an insight generalises; that is a semantic
// question. Guessing wrong in the general direction is the expensive one: a
// mislabelled entry travels to every other bed's soil, which is the exact
// failure scope exists to prevent. Guessing wrong toward bed only means the
// entry stays home.
//
// The Extractor interface still carries scope, so a model-backed extractor can
// propose it properly. Until one does, promotion is a review action.
func proposeScope(text, bed string) pile.Scope {
	return pile.ScopeBed
}

func mergeBeads(primary string, found []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range append([]string{primary}, found...) {
		if b == "" || seen[b] {
			continue
		}
		seen[b] = true
		out = append(out, b)
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// SHA256 identifies the digest an entry was extracted from, so re-composting
// the same material is detectable and the extractor's output is traceable to
// its exact input.
func (d *Digest) SHA256() string {
	sum := sha256.Sum256([]byte(d.Render()))
	return hex.EncodeToString(sum[:])
}
