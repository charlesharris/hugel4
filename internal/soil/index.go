// Package soil draws context from the pile.
//
// The name is the point. Soil is not the pile — it is the small, selected part
// of it that gets delivered to a particular piece of work, and delivering less
// of it is the whole discipline. Every token that enters a session is re-sent
// on every later turn, so what soil costs is set by how much of it enters and
// how early, not by how much the pile holds.
//
// Retrieval is lexical and local: BM25 over the entries, weighted by whether
// knowledge belongs to this bed, whether a human has vouched for it, and how
// long ago it was earned. No embeddings, no service, no network. At this size a
// good ranking function beats a vector database, and the pile has to prove it
// deserves one before it gets one.
package soil

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charris/hugel/internal/pile"
)

// BM25 parameters. k1 damps the effect of repeating a term; b controls how much
// a long entry is penalised for its length.
const (
	k1 = 1.2
	b  = 0.6
)

// Index is a searchable view of the pile, built in memory. Rebuilding it costs
// milliseconds at pile scale, so it is never persisted: the files stay the
// source of truth and the index stays disposable.
type Index struct {
	entries []*pile.Entry
	docs    []doc
	df      map[string]int
	avgLen  float64
	now     time.Time
}

type doc struct {
	terms  map[string]int
	length int
}

// Build indexes entries. Rejected and abandoned entries are dropped here rather
// than down-weighted: knowledge someone threw away should not surface at all.
func Build(entries []*pile.Entry, now time.Time) *Index {
	ix := &Index{df: map[string]int{}, now: now}
	total := 0
	for _, e := range entries {
		if e.Review == pile.Rejected || e.Status == pile.Abandoned {
			continue
		}
		d := doc{terms: map[string]int{}}
		for _, t := range terms(searchText(e)) {
			d.terms[t]++
			d.length++
		}
		for t := range d.terms {
			ix.df[t]++
		}
		total += d.length
		ix.entries = append(ix.entries, e)
		ix.docs = append(ix.docs, d)
	}
	if len(ix.docs) > 0 {
		ix.avgLen = float64(total) / float64(len(ix.docs))
	}
	return ix
}

// Len is how many entries are searchable.
func (ix *Index) Len() int { return len(ix.entries) }

func searchText(e *pile.Entry) string {
	var b strings.Builder
	// The title is worth more than the body, so it is indexed twice. A cheap
	// way to say "what this entry is about" outranks "what it mentions".
	b.WriteString(e.Title)
	b.WriteString(" ")
	b.WriteString(e.Title)
	b.WriteString(" ")
	b.WriteString(e.Body)
	for _, s := range [][]string{e.Tags, e.Paths, e.Beads} {
		b.WriteString(" ")
		b.WriteString(strings.Join(s, " "))
	}
	b.WriteString(" ")
	b.WriteString(string(e.Type))
	return b.String()
}

// terms splits text the way both a query and an entry should be split. Path
// separators and punctuation break, so "internal/drive/sense.go" contributes
// its parts and a search for "sense.go" finds it.
func terms(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_'
	})
	out := make([]string, 0, len(fields)*2)
	for _, f := range fields {
		f = strings.Trim(f, "._")
		if f == "" || stop[f] {
			continue
		}
		out = append(out, f)
		// A dotted token is also worth its stem: "sense.go" matches "sense".
		if i := strings.IndexByte(f, '.'); i > 1 {
			if stem := f[:i]; !stop[stem] {
				out = append(out, stem)
			}
		}
	}
	return out
}

// stop holds words too common in a codebase knowledge base to discriminate.
var stop = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"to": true, "of": true, "in": true, "on": true, "at": true, "by": true,
	"for": true, "with": true, "from": true, "as": true, "that": true, "this": true,
	"it": true, "its": true, "we": true, "you": true, "not": true, "no": true,
	"so": true, "if": true, "then": true, "than": true, "when": true, "which": true,
	"what": true, "how": true, "why": true, "do": true, "does": true, "did": true,
	"can": true, "will": true, "would": true, "should": true, "there": true,
}

// score is BM25 for one document against the query terms.
func (ix *Index) score(i int, q []string) float64 {
	d := ix.docs[i]
	if d.length == 0 {
		return 0
	}
	n := float64(len(ix.docs))
	var total float64
	for _, t := range q {
		f := float64(d.terms[t])
		if f == 0 {
			continue
		}
		df := float64(ix.df[t])
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		norm := 1 - b + b*float64(d.length)/ix.avgLen
		total += idf * (f * (k1 + 1)) / (f + k1*norm)
	}
	return total
}

// Query asks the pile a question.
type Query struct {
	Text string // what the work is about
	Bed  string // where the work is happening
	// Kin names the other beds that are the same project under an older name.
	// Without it a rename penalises a project's oldest and most settled
	// decisions exactly because they are old.
	Kin    []string
	Type   pile.Type // optional: only this kind of knowledge
	Limit  int       // most entries to return
	Budget int       // most tokens of soil to deliver
	// Snippet caps any single entry, so one long entry cannot spend the whole
	// budget. Soil is a survey of what the pile knows; reading an entry in full
	// is a second, deliberate step.
	Snippet int
}

// Match is one entry the pile offered, and why.
type Match struct {
	Entry *pile.Entry
	Score float64
}

// Defaults for a query that did not say.
const (
	defaultLimit   = 8
	defaultBudget  = 1500
	defaultSnippet = 110
)

// Search ranks the pile against a query.
//
// Relevance alone is the wrong ranking for a shared pile. An entry from another
// project may match the words perfectly and still be the wrong knowledge, so
// lexical score is weighted by three things a search engine has no reason to
// care about: whether this bed earned the knowledge, whether a human vouched
// for it, and how long ago it was true.
func (ix *Index) Search(q Query) []Match {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	qt := terms(q.Text)
	if len(qt) == 0 {
		return nil
	}

	var matches []Match
	for i, e := range ix.entries {
		if q.Type != "" && e.Type != q.Type {
			continue
		}
		s := ix.score(i, qt)
		if s <= 0 {
			continue
		}
		s *= ix.weight(e, q.Bed, q.Kin)
		if s <= 0 {
			continue
		}
		matches = append(matches, Match{Entry: e, Score: s})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Entry.OccurredAt.After(matches[j].Entry.OccurredAt)
	})
	if len(matches) > q.Limit {
		matches = matches[:q.Limit]
	}
	return matches
}

// weight adjusts relevance by provenance, trust, and age.
func (ix *Index) weight(e *pile.Entry, bed string, kin []string) float64 {
	w := 1.0

	switch {
	case bed == "" || e.Bed == bed || isKin(e.Bed, kin):
		// The bed that earned it, an earlier name for the same project, or a
		// query that did not say where it is.
	case e.Scope == pile.ScopeGeneral:
		// General knowledge is meant to travel, but knowledge earned here is
		// still likelier to be the knowledge wanted here.
		w *= 0.8
	default:
		// Another project's specifics. Not excluded -- a neighbouring bed often
		// hit the same wall first -- but it has to be clearly more relevant to
		// outrank something local.
		w *= 0.3
	}

	switch e.Review {
	case pile.Accepted:
		w *= 1.4 // someone read this and vouched for it
	case pile.Unreviewed:
		w *= 1.0
	}
	if e.Status == pile.Superseded {
		w *= 0.4
	}

	// Confidence is the extractor's own estimate; let it move the ranking a
	// little without letting it dominate.
	w *= 0.7 + 0.6*e.Confidence

	// Age decays but never disappears: a two-year-old constraint can still be
	// the reason something is the way it is.
	years := ix.now.Sub(e.OccurredAt).Hours() / (24 * 365)
	if years > 0 {
		w *= 0.55 + 0.45*math.Exp(-years)
	}
	return w
}

func isKin(bed string, kin []string) bool {
	for _, k := range kin {
		if strings.EqualFold(bed, k) {
			return true
		}
	}
	return false
}
