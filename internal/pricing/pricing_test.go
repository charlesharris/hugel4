package pricing

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"claude-opus-5", "claude-opus-5"},
		{"claude-opus-5[1m]", "claude-opus-5"},
		{"Claude-Opus-5", "claude-opus-5"},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5"},
		{"  claude-haiku-4-5  ", "claude-haiku-4-5"},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLookupFastMode(t *testing.T) {
	std, ok := Lookup("claude-opus-5", "standard")
	if !ok || std.Input != 5 || std.Output != 25 {
		t.Fatalf("standard opus-5 = %+v ok=%v", std, ok)
	}
	fast, ok := Lookup("claude-opus-5", "fast")
	if !ok || fast.Input != 10 || fast.Output != 50 {
		t.Fatalf("fast opus-5 = %+v ok=%v, want the premium rate", fast, ok)
	}
	// A model with no fast tier falls back to its standard rate rather than
	// reporting itself unpriced.
	h, ok := Lookup("claude-haiku-4-5", "fast")
	if !ok || h.Input != 1 {
		t.Fatalf("fast haiku = %+v ok=%v, want standard fallback", h, ok)
	}
}

func TestPrice(t *testing.T) {
	// 1M of each class on opus-5 ($5 in / $25 out) makes the multipliers legible.
	c, ok := Price("claude-opus-5", "standard", Tokens{
		Input: 1_000_000, Output: 1_000_000,
		CacheRead: 1_000_000, CacheWrite5m: 1_000_000, CacheWrite1h: 1_000_000,
	})
	if !ok {
		t.Fatal("opus-5 should be priced")
	}
	want := map[string]struct{ got, want float64 }{
		"input":       {c.Input, 5},
		"output":      {c.Output, 25},
		"cache read":  {c.CacheRead, 0.5},           // 0.1x input
		"cache write": {c.CacheWrite, 1.25*5 + 2*5}, // 5m at 1.25x, 1h at 2x
	}
	for name, v := range want {
		if v.got != v.want {
			t.Errorf("%s = %v, want %v", name, v.got, v.want)
		}
	}
	if got, want := c.Total(), 5.0+25+0.5+16.25; got != want {
		t.Errorf("Total() = %v, want %v", got, want)
	}
	// Context is everything except output.
	if got, want := c.Context(), 5.0+0.5+16.25; got != want {
		t.Errorf("Context() = %v, want %v", got, want)
	}
}

func TestUnknownAndFreeModels(t *testing.T) {
	if _, ok := Price("some-other-llm", "standard", Tokens{Input: 100}); ok {
		t.Error("unknown model should not be priced")
	}
	if Free("some-other-llm") {
		t.Error("an unknown model is unpriced, not free")
	}
	for _, m := range []string{"", "<synthetic>"} {
		if !Free(m) {
			t.Errorf("Free(%q) = false, want true", m)
		}
	}
}
