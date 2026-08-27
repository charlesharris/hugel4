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

	"github.com/charris/hugel/internal/cochange"
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

	// contradicted is the set of entries something later took back. The edge is
	// written on the revert and read here, so the entry it falsifies does not
	// have to be rewritten to lose its standing.
	contradicted map[string]bool
}

type doc struct {
	terms  map[string]int
	length int
}

// Build indexes entries. Rejected and abandoned entries are dropped here rather
// than down-weighted: knowledge someone threw away should not surface at all.
func Build(entries []*pile.Entry, now time.Time) *Index {
	ix := &Index{df: map[string]int{}, now: now, contradicted: map[string]bool{}}
	// Links are read before entries are filtered: a revert that was itself
	// rejected or abandoned still said something about what it took back.
	for _, e := range entries {
		for _, id := range e.Contradicted() {
			ix.contradicted[id] = true
		}
	}
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

	// Coupling says which parts of the project change together, derived from
	// git at draw time. It answers the question word overlap cannot: what else
	// bears on this. Absent, the draw is exactly what it was before -- the
	// bonus is a nudge for near-ties and never a substitute for relevance.
	Coupling cochange.Coupling
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
	byScore := func(m []Match) {
		sort.Slice(m, func(i, j int) bool {
			if m[i].Score != m[j].Score {
				return m[i].Score > m[j].Score
			}
			return m[i].Entry.OccurredAt.After(m[j].Entry.OccurredAt)
		})
	}
	byScore(matches)
	// Coupling is applied to everything that matched, before the list is cut
	// down. Applying it after would only ever reorder entries that had already
	// won, which is the one place it has nothing to add.
	if applyCoupling(matches, q.Coupling) {
		byScore(matches)
	}
	if len(matches) > q.Limit {
		matches = matches[:q.Limit]
	}
	return matches
}

// How far a dependency may move a ranking.
const (
	// maxSeeds bounds how many matches stand for "what the query is about".
	// The query itself names no paths -- a bead is a sentence, not a diff -- so
	// the best available statement of its subject is the entries that already
	// matched it on wording.
	maxSeeds = 3
	// couplingBonus is deliberately small. Word overlap remains the signal and
	// this is a nudge: at full coupling it lifts an entry by a sixth, enough to
	// settle a near-tie in favour of the neighbour that actually bears on the
	// work, and never enough to float something irrelevant.
	couplingBonus = 0.16
)

// applyCoupling lifts matches whose code areas change together with the areas
// the strongest matches are about, and reports whether it moved anything.
//
// This is the reader the pile has never had. Edge types have existed in the
// schema since it was built and nothing ever consumed one, which is why nothing
// ever wrote one. A weight is the cheapest reader that changes an outcome, and
// it composes with the ranking instead of competing with it.
func applyCoupling(matches []Match, c cochange.Coupling) bool {
	if c == nil || len(matches) < 2 {
		return false
	}
	// The subject is stated by the strongest matches; the lift is for what
	// ranked below them. A share rather than a fixed count, so a draw that
	// returned three entries does not make all three the subject and leave
	// nothing to lift -- with few matches only the top one speaks for the
	// query.
	seeds := len(matches) / 3
	if seeds < 1 {
		seeds = 1
	}
	if seeds > maxSeeds {
		seeds = maxSeeds
	}
	areas := map[string]bool{}
	for _, m := range matches[:seeds] {
		for _, p := range m.Entry.Paths {
			if d := cochange.Dir(p); d != "" {
				areas[d] = true
			}
		}
	}
	if len(areas) == 0 {
		return false
	}
	moved := false
	for i := seeds; i < len(matches); i++ {
		best := 0.0
		for _, p := range matches[i].Entry.Paths {
			d := cochange.Dir(p)
			if d == "" {
				continue
			}
			for seed := range areas {
				// Score answers 0 for an area against itself, so an entry in
				// the subject's own area is not rewarded for being there --
				// that would amplify word overlap under another name.
				if s := c.Score(d, seed); s > best {
					best = s
				}
			}
		}
		if best > 0 {
			matches[i].Score *= 1 + couplingBonus*best
			moved = true
		}
	}
	return moved
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
	// Something the garden went back and took out. This is the heaviest penalty
	// in here, and deliberately heavier than any amount of age: age is a proxy
	// for wrongness and a poor one -- a two-year-old constraint can be exactly
	// right and a three-week-old decision can be dead -- while a revert is the
	// evidence that proxy was standing in for.
	//
	// Down-weighted rather than dropped. That a thing was tried and taken back
	// is what stops the next tender trying it again, which makes it some of the
	// most useful knowledge in the pile as long as it arrives labelled.
	if ix.contradicted[e.ID] {
		w *= 0.2
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
