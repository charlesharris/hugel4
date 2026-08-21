package transcript

import (
	"strings"
	"testing"
)

func load(t *testing.T) *Session {
	t.Helper()
	s, err := ParseFile("testdata/session.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return s
}

// Claude Code writes one transcript record per assistant content block, each
// carrying the same usage snapshot for the request. Counting records instead of
// requests inflates every number, so this is the parser's load-bearing rule.
func TestParseDedupesRequests(t *testing.T) {
	s := load(t)
	if got, want := len(s.Requests), 3; got != want {
		t.Fatalf("got %d requests, want %d (req_1 appears twice and must count once)", got, want)
	}
	u := s.Usage()
	if got, want := u.Output, 150; got != want {
		t.Errorf("output = %d, want %d", got, want)
	}
	if got, want := u.CacheWrite5m, 1300; got != want {
		t.Errorf("cache write 5m = %d, want %d", got, want)
	}
}

func TestParseSessionMetadata(t *testing.T) {
	s := load(t)
	if s.ID != "sess-abc" {
		t.Errorf("ID = %q", s.ID)
	}
	if s.Bed != "beer-run" {
		t.Errorf("Bed = %q, want beer-run", s.Bed)
	}
	if s.Branch != "main" || s.Version != "2.1.0" {
		t.Errorf("Branch=%q Version=%q", s.Branch, s.Version)
	}
	if got, want := s.Duration().Minutes(), 3.0; got != want {
		t.Errorf("Duration = %v min, want %v", got, want)
	}
}

// isMeta user records are injected, not typed by the gardener, and must not
// inflate the per-prompt cost denominator.
func TestParseCountsOnlyRealPrompts(t *testing.T) {
	if got, want := load(t).Prompts, 2; got != want {
		t.Errorf("Prompts = %d, want %d", got, want)
	}
}

// A transcript is an append-only log that may be truncated or interleaved with
// records hugel does not understand; one bad line must not sink the file.
func TestParseSkipsMalformedLines(t *testing.T) {
	if len(load(t).Requests) == 0 {
		t.Fatal("malformed line aborted the parse")
	}
}

// Older transcripts report a flat cache_creation_input_tokens with no TTL
// split. Attributing it to the cheaper 5m tier keeps hugel from overstating.
func TestParseFallsBackToFlatCacheCreation(t *testing.T) {
	s := load(t)
	var sub *Request
	for i := range s.Requests {
		if s.Requests[i].Sidechain {
			sub = &s.Requests[i]
		}
	}
	if sub == nil {
		t.Fatal("expected a sidechain request")
	}
	if got, want := sub.Usage.CacheWrite5m, 300; got != want {
		t.Errorf("flat cache_creation -> 5m tier = %d, want %d", got, want)
	}
	if sub.Usage.CacheWrite1h != 0 {
		t.Errorf("unknown TTL must not be billed at the 1h rate")
	}
}

func TestSidechainIsolated(t *testing.T) {
	s := load(t)
	if got, want := s.SidechainUsage().Output, 50; got != want {
		t.Errorf("sidechain output = %d, want %d", got, want)
	}
	if got, want := s.Usage().Output, 150; got != want {
		t.Errorf("total output = %d, want %d (sidechain counts toward the total)", got, want)
	}
}

func TestContextReadIsInputPlusCacheHits(t *testing.T) {
	u := Usage{Input: 10, CacheRead: 90, Output: 500}
	if got, want := u.ContextRead(), 100; got != want {
		t.Errorf("ContextRead = %d, want %d", got, want)
	}
}

func TestBedFromSlugFallback(t *testing.T) {
	// Lossy by nature: separators and hyphens both flatten to "-".
	if got := bedFromSlug("-Users-charris-src-hugel4"); got != "hugel4" {
		t.Errorf("bedFromSlug = %q", got)
	}
}

func TestParseEmptyInput(t *testing.T) {
	s, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse(empty): %v", err)
	}
	if len(s.Requests) != 0 {
		t.Error("empty transcript should yield no requests")
	}
}
