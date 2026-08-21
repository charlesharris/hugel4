package soil

import (
	"strings"
	"testing"
	"time"

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
