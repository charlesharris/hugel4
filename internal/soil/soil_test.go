package soil

import (
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/cochange"
	"github.com/charris/hugel/internal/pile"
)

var now = time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

func e(bed, title, body string, opts ...func(*pile.Entry)) *pile.Entry {
	x := &pile.Entry{
		ID: strings.Repeat("a", 16), Type: pile.Decision, Scope: pile.ScopeBed,
		Title: title, Body: body, Bed: bed, Status: pile.Active,
		Review: pile.Unreviewed, Confidence: 0.6, OccurredAt: now.AddDate(0, 0, -1),
	}
	for _, o := range opts {
		o(x)
	}
	return x
}

func titles(ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Entry.Title
	}
	return out
}

func TestRanksByRelevance(t *testing.T) {
	ix := Build([]*pile.Entry{
		e("b", "connection pooling in postgres", "pgbouncer must run in session mode"),
		e("b", "a note about css grid", "columns and rows"),
	}, now)
	got := titles(ix.Search(Query{Text: "postgres pooling", Bed: "b"}))
	if len(got) != 1 || !strings.Contains(got[0], "pooling") {
		t.Errorf("ranked %v", got)
	}
}

// A shared pile means another project's entry can match the words perfectly
// and still be the wrong knowledge.
func TestLocalKnowledgeOutranksAForeignBed(t *testing.T) {
	ix := Build([]*pile.Entry{
		e("other", "retry policy on timeouts", "back off exponentially"),
		e("mine", "retry policy on timeouts", "back off exponentially"),
	}, now)
	got := ix.Search(Query{Text: "retry policy timeouts", Bed: "mine"})
	if len(got) != 2 {
		t.Fatalf("got %d matches", len(got))
	}
	if got[0].Entry.Bed != "mine" {
		t.Errorf("foreign bed outranked the local one: %+v", titles(got))
	}
}

// A project renamed across rewrites must not have its oldest decisions
// demoted for being filed under the old name.
func TestKinCountsAsTheSameProject(t *testing.T) {
	entries := []*pile.Entry{
		e("hugel", "the pile is the source of truth", "the graph is derived and disposable"),
		e("elsewhere", "the pile is the source of truth", "the graph is derived and disposable"),
	}
	ix := Build(entries, now)

	without := ix.Search(Query{Text: "pile source of truth", Bed: "hugel4"})
	with := ix.Search(Query{Text: "pile source of truth", Bed: "hugel4", Kin: []string{"hugel", "hugel4"}})

	if len(with) != 2 || len(without) != 2 {
		t.Fatalf("matches: %d without kin, %d with", len(without), len(with))
	}
	if with[0].Entry.Bed != "hugel" {
		t.Errorf("kin bed did not rank first: %v", titles(with))
	}
	if with[0].Score <= without[0].Score {
		t.Error("declaring kinship did not lift the kin bed's score")
	}
}

// General knowledge is meant to travel, but knowledge earned here is still
// likelier to be the knowledge wanted here.
func TestGeneralTravelsButDoesNotOutrankLocal(t *testing.T) {
	ix := Build([]*pile.Entry{
		e("far", "listen notify needs session mode", "transaction pooling breaks it",
			func(x *pile.Entry) { x.Scope = pile.ScopeGeneral }),
		e("far", "listen notify needs session mode", "transaction pooling breaks it"),
	}, now)
	got := ix.Search(Query{Text: "listen notify session mode", Bed: "mine"})
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].Entry.Scope != pile.ScopeGeneral {
		t.Error("general entry did not outrank another bed's specifics")
	}
}

// Knowledge someone threw away should not surface at all; knowledge someone
// vouched for should surface sooner.
func TestReviewAndStatusShapeWhatSurfaces(t *testing.T) {
	ix := Build([]*pile.Entry{
		e("b", "rejected wisdom", "about caching",
			func(x *pile.Entry) { x.Review = pile.Rejected }),
		e("b", "abandoned wisdom", "about caching",
			func(x *pile.Entry) { x.Status = pile.Abandoned }),
		e("b", "plain wisdom", "about caching"),
		e("b", "vouched wisdom", "about caching",
			func(x *pile.Entry) { x.Review = pile.Accepted }),
	}, now)
	if ix.Len() != 2 {
		t.Fatalf("indexed %d entries, want the rejected and abandoned ones dropped", ix.Len())
	}
	got := titles(ix.Search(Query{Text: "caching wisdom", Bed: "b"}))
	if len(got) != 2 || got[0] != "vouched wisdom" {
		t.Errorf("ranked %v, want the vouched entry first", got)
	}
}

func TestSupersededSinksButRemains(t *testing.T) {
	ix := Build([]*pile.Entry{
		e("b", "old approach to indexing", "we used a graph",
			func(x *pile.Entry) { x.Status = pile.Superseded }),
		e("b", "current approach to indexing", "we use a graph"),
	}, now)
	got := titles(ix.Search(Query{Text: "approach indexing graph", Bed: "b"}))
	if len(got) != 2 {
		t.Fatalf("superseded entry vanished: %v", got)
	}
	if got[0] != "current approach to indexing" {
		t.Errorf("superseded entry ranked first: %v", got)
	}
}

// The budget is the feature. Soil that ignores it reproduces the problem it
// exists to solve.
func TestDrawRespectsBudget(t *testing.T) {
	long := strings.Repeat("pooling connections and sessions and modes ", 400)
	var entries []*pile.Entry
	for i := 0; i < 20; i++ {
		entries = append(entries, e("b", "pooling note", long))
	}
	s := Build(entries, now).Draw(Query{Text: "pooling connections", Bed: "b", Budget: 300, Limit: 20})
	if s.Tokens > 300 {
		t.Errorf("delivered %d tokens against a 300 budget", s.Tokens)
	}
	if len(s.Items) == 0 {
		t.Fatal("budget starved the result entirely")
	}
	if s.Omitted == 0 {
		t.Error("truncation was silent; soil must say what it left out")
	}
}

// One long entry must not spend the whole budget: soil is a survey of what the
// pile knows, and reading an entry in full is a second, deliberate step.
func TestNoSingleEntryEatsTheBudget(t *testing.T) {
	long := strings.Repeat("pooling connections in session mode ", 500)
	s := Build([]*pile.Entry{
		e("b", "first pooling note", long),
		e("b", "second pooling note", long),
		e("b", "third pooling note", long),
	}, now).Draw(Query{Text: "pooling connections", Bed: "b", Budget: 1200, Limit: 5})
	if len(s.Items) < 3 {
		t.Errorf("one entry crowded out the rest: %d items, %d tokens", len(s.Items), s.Tokens)
	}
}

// An entry's answer to a specific question is rarely its first sentence.
func TestExcerptCentresOnTheQuery(t *testing.T) {
	body := strings.Repeat("preamble that says nothing useful at all. ", 40) +
		"THE ANSWER IS SESSION MODE. " + strings.Repeat("trailing matter. ", 40)
	got, partial := excerpt(body, terms("session mode"), 40)
	if !partial {
		t.Fatal("expected a partial excerpt")
	}
	if !strings.Contains(got, "SESSION MODE") {
		t.Errorf("excerpt missed the query terms: %q", got)
	}
}

func TestEmptyQueryReturnsNothing(t *testing.T) {
	ix := Build([]*pile.Entry{e("b", "a title", "a body")}, now)
	if got := ix.Search(Query{Text: "   ", Bed: "b"}); got != nil {
		t.Errorf("empty query matched %v", titles(got))
	}
	if got := ix.Search(Query{Text: "the and of to", Bed: "b"}); got != nil {
		t.Errorf("stopwords-only query matched %v", titles(got))
	}
}

func TestTermsSplitPaths(t *testing.T) {
	got := terms("internal/drive/sense.go")
	want := map[string]bool{"internal": true, "drive": true, "sense.go": true, "sense": true}
	for _, g := range got {
		delete(want, g)
	}
	if len(want) > 0 {
		t.Errorf("terms(%q) = %v, missing %v", "internal/drive/sense.go", got, want)
	}
}

func TestRenderSaysWhenThePileKnowsNothing(t *testing.T) {
	s := Build([]*pile.Entry{e("b", "unrelated", "matter")}, now).
		Draw(Query{Text: "quantum chromodynamics", Bed: "b"})
	if !strings.Contains(s.Render(), "nothing") {
		t.Errorf("render = %q", s.Render())
	}
}

func ids(ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Entry.ID
	}
	return out
}

// The acceptance the bead was filed against: a contradicted entry has to sink
// further than an old one. Age is a proxy for wrongness and a poor one -- a
// three-year-old decision can be exactly right -- and a revert is the evidence
// that proxy was standing in for.
func TestAContradictedEntrySinksBelowAnOldOne(t *testing.T) {
	old := e("b", "indexing with a graph", "the index is a graph",
		func(x *pile.Entry) { x.ID = "old-1"; x.OccurredAt = now.AddDate(-3, 0, 0) })
	taken := e("b", "indexing with a graph", "the index is a graph",
		func(x *pile.Entry) { x.ID = "taken-1" })
	revert := e("b", "Revert \"indexing with a graph\"", "it did not survive contact",
		func(x *pile.Entry) {
			x.ID, x.Type = "revert-1", pile.Failure
			x.Links = []pile.Link{{Rel: pile.RelContradicts, ID: "taken-1"}}
		})

	ix := Build([]*pile.Entry{old, taken, revert}, now)
	got := ids(ix.Search(Query{Text: "indexing graph index", Bed: "b"}))

	var oldAt, takenAt = -1, -1
	for i, id := range got {
		switch id {
		case "old-1":
			oldAt = i
		case "taken-1":
			takenAt = i
		}
	}
	if takenAt < 0 {
		t.Fatalf("the contradicted entry vanished: %v", got)
	}
	if oldAt < 0 || oldAt > takenAt {
		t.Errorf("ranked %v; three years of age outranked a revert", got)
	}
}

// A revert that was itself thrown away still said something about what it took
// back, so the edge is read before entries are filtered.
func TestAnEdgeFromARejectedRevertStillCounts(t *testing.T) {
	taken := e("b", "indexing with a graph", "the index is a graph",
		func(x *pile.Entry) { x.ID = "taken-1" })
	revert := e("b", "Revert \"indexing with a graph\"", "gone",
		func(x *pile.Entry) {
			x.ID, x.Type, x.Review = "revert-1", pile.Failure, pile.Rejected
			x.Links = []pile.Link{{Rel: pile.RelContradicts, ID: "taken-1"}}
		})
	ix := Build([]*pile.Entry{taken, revert}, now)
	if !ix.contradicted["taken-1"] {
		t.Error("the edge was dropped with the entry that carried it")
	}
}

// Coupling answers the question word overlap cannot: what else bears on this.
// The entry that never mentions the query's words still gets a hearing when the
// project's own history says its part of the tree moves with the part the query
// is about.
func TestCouplingLiftsANeighbourOverANearTie(t *testing.T) {
	seed := e("hugel4", "the pooler stays in session mode", "pooler pooler session")
	seed.ID = "seed000000000000"
	seed.Paths = []string{"internal/api"}

	// Two entries that match the query equally weakly. One is in a part of the
	// tree that moves with the seed's; the other is not.
	near := e("hugel4", "connection limits in the store", "pooler")
	near.ID = "near000000000000"
	near.Paths = []string{"internal/store/conn.go"}

	far := e("hugel4", "icon sizing in the viewer", "pooler")
	far.ID = "far0000000000000"
	far.Paths = []string{"internal/ui/icons.go"}

	ix := Build([]*pile.Entry{seed, near, far}, now)
	q := Query{Text: "pooler session mode", Bed: "hugel4", Limit: 5}

	scoreOf := func(ms []Match, id string) float64 {
		for _, m := range ms {
			if m.Entry.ID == id {
				return m.Score
			}
		}
		return 0
	}
	plain := ix.Search(q)
	q.Coupling = cochange.Coupling{
		"internal/api":   {"internal/store": 1.0},
		"internal/store": {"internal/api": 1.0},
	}
	coupled := ix.Search(q)

	if got, was := scoreOf(coupled, near.ID), scoreOf(plain, near.ID); got <= was {
		t.Errorf("the coupled neighbour scored %.4f, was %.4f -- coupling did not lift it", got, was)
	}
	if got, was := scoreOf(coupled, far.ID), scoreOf(plain, far.ID); got != was {
		t.Errorf("an entry coupled to nothing was re-scored: %.4f -> %.4f", was, got)
	}
	// And the lift is a nudge, not an override: it must not outrank the entry
	// that actually matched the words.
	if scoreOf(coupled, near.ID) > scoreOf(coupled, seed.ID) {
		t.Error("coupling floated a neighbour above the entry the query actually matched")
	}
}

// The acceptance criterion in its own right: a project whose history says
// nothing draws exactly what it drew before. A ranking signal that cannot be
// switched off is one nobody can trust.
func TestADrawWithoutCouplingIsUnchanged(t *testing.T) {
	entries := []*pile.Entry{
		e("hugel4", "the pooler stays in session mode", "pooler session"),
		e("hugel4", "connection limits in the store", "pooler limits"),
		e("hugel4", "icon sizing in the viewer", "pooler icons"),
	}
	for i, x := range entries {
		x.ID = strings.Repeat(string(rune('a'+i)), 16)
		x.Paths = []string{"internal/pkg" + string(rune('a'+i))}
	}
	ix := Build(entries, now)
	q := Query{Text: "pooler session", Bed: "hugel4", Limit: 5}

	before := titles(ix.Search(q))
	q.Coupling = nil
	after := titles(ix.Search(q))
	if len(before) != len(after) {
		t.Fatalf("different number of results: %v vs %v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("order changed with no coupling: %v vs %v", before, after)
		}
	}
	// An empty coupling is the same promise as no coupling at all.
	q.Coupling = cochange.Coupling{}
	if got := titles(ix.Search(q)); len(got) != len(before) || got[0] != before[0] {
		t.Errorf("an empty coupling changed the draw: %v vs %v", got, before)
	}
}

// Rewarding the seeds again would amplify word overlap under a new name. The
// bonus is for what the query did not think to ask about.
func TestCouplingDoesNotRewardTheSeedsAgain(t *testing.T) {
	a := e("hugel4", "the pooler stays in session mode", "pooler session")
	a.ID = "aaaa000000000000"
	a.Paths = []string{"internal/api/pool.go"}
	b := e("hugel4", "pooler timeouts", "pooler session")
	b.ID = "bbbb000000000000"
	b.Paths = []string{"internal/api/timeout.go"}

	ix := Build([]*pile.Entry{a, b}, now)
	q := Query{Text: "pooler session", Bed: "hugel4", Limit: 5}
	plain := ix.Search(q)
	q.Coupling = cochange.Coupling{"internal/api": {"internal/api": 1.0}}
	coupled := ix.Search(q)

	for i := range plain {
		if plain[i].Score != coupled[i].Score {
			t.Errorf("%q was re-scored for coupling to its own area: %v -> %v",
				plain[i].Entry.Title, plain[i].Score, coupled[i].Score)
		}
	}
}
