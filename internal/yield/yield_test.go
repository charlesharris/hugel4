package yield

import (
	"testing"
	"time"

	"github.com/charris/hugel/internal/transcript"
)

func req(model string, sidechain bool, u transcript.Usage) transcript.Request {
	return transcript.Request{Model: model, Speed: "standard", Sidechain: sidechain, Usage: u}
}

func session(bed string, end time.Time, prompts int, rs ...transcript.Request) *transcript.Session {
	return &transcript.Session{
		ID: bed + "-sess", Bed: bed, Prompts: prompts,
		Start: end.Add(-time.Hour), End: end, Requests: rs,
	}
}

// The headline number: what share of spend went to hauling context around
// rather than producing output.
func TestContextTax(t *testing.T) {
	// opus-5: 1M cache read = $0.50, 1M output = $25.
	e := Price(session("beer-run", time.Now(), 1,
		req("claude-opus-5", false, transcript.Usage{CacheRead: 1_000_000, Output: 1_000_000}),
	))
	if got, want := e.Cost.Total(), 25.5; got != want {
		t.Fatalf("total = %v, want %v", got, want)
	}
	if got, want := e.ContextTax(), 0.5/25.5; got != want {
		t.Errorf("ContextTax = %v, want %v", got, want)
	}
	if got, want := e.CostPerPrompt(), 25.5; got != want {
		t.Errorf("CostPerPrompt = %v, want %v", got, want)
	}
}

// A session that reads 100x its output is the shape hugel exists to catch.
func TestContextTaxCatchesASpiral(t *testing.T) {
	e := Price(session("healthos", time.Now(), 1,
		req("claude-opus-5", false, transcript.Usage{CacheRead: 400_000_000, Output: 1_000_000}),
	))
	if e.ContextTax() < 0.85 {
		t.Errorf("ContextTax = %v, want a runaway session to read as heavily taxed", e.ContextTax())
	}
}

// Subagent spend is counted in the total but tracked separately, because it
// happened in a context that was thrown away instead of being re-sent.
func TestSidechainTrackedSeparately(t *testing.T) {
	e := Price(session("beer-run", time.Now(), 1,
		req("claude-opus-5", false, transcript.Usage{Output: 1_000_000}),
		req("claude-opus-5", true, transcript.Usage{Output: 1_000_000}),
	))
	if got, want := e.Cost.Total(), 50.0; got != want {
		t.Errorf("total = %v, want %v", got, want)
	}
	if got, want := e.Sidechain.Total(), 25.0; got != want {
		t.Errorf("sidechain = %v, want %v", got, want)
	}
}

func TestUnpricedModelsAreReportedNotSilentlyZeroed(t *testing.T) {
	e := Price(session("beer-run", time.Now(), 1,
		req("some-other-llm", false, transcript.Usage{Output: 1_000_000}),
		req("<synthetic>", false, transcript.Usage{}),
	))
	if e.Unpriced != 1 {
		t.Errorf("Unpriced = %d, want 1 (synthetic is free, not unpriced)", e.Unpriced)
	}
	if e.Cost.Total() != 0 {
		t.Errorf("unknown model must not be valued")
	}
}

func TestBuildRollsUpByBedAndFilters(t *testing.T) {
	now := time.Now()
	sessions := []*transcript.Session{
		session("beer-run", now, 1, req("claude-opus-5", false, transcript.Usage{Output: 1_000_000})),
		session("beer-run", now, 1, req("claude-opus-5", false, transcript.Usage{Output: 1_000_000})),
		session("healthos", now.Add(-90*24*time.Hour), 1, req("claude-opus-5", false, transcript.Usage{Output: 1_000_000})),
	}

	all := Build(sessions, Filter{})
	if got, want := len(all.Beds), 2; got != want {
		t.Fatalf("beds = %d, want %d", got, want)
	}
	if got, want := all.Beds[0].Name, "beer-run"; got != want {
		t.Errorf("beds sorted by spend: first = %q, want %q", got, want)
	}
	if got, want := all.Beds[0].Sessions, 2; got != want {
		t.Errorf("beer-run sessions = %d, want %d", got, want)
	}

	recent := Build(sessions, Filter{Since: now.Add(-30 * 24 * time.Hour)})
	if got, want := len(recent.Entries), 2; got != want {
		t.Errorf("since-filtered entries = %d, want %d", got, want)
	}

	oneBed := Build(sessions, Filter{Bed: "healthos"})
	if got, want := len(oneBed.Entries), 1; got != want {
		t.Errorf("bed-filtered entries = %d, want %d", got, want)
	}
}
