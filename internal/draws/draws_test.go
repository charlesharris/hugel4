package draws

import (
	"os"
	"testing"
	"time"
)

func TestAppendAndLoad(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())

	if got, err := Load(); err != nil || got != nil {
		t.Fatalf("Load on a fresh garden = %v, %v, want no draws and no error", got, err)
	}

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	for i, q := range []string{"first question", "second question"} {
		if err := Append(Draw{
			At: now.Add(time.Duration(i) * time.Hour), Bed: "hugel4", Query: q,
			Budget: 1500, Tokens: 780, Considered: 247, Entries: []string{"abc123"},
		}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d draws, want 2", len(got))
	}
	if got[0].Query != "first question" {
		t.Errorf("draws came back out of order: %q first", got[0].Query)
	}
	if !got[0].At.Equal(now) || got[0].Tokens != 780 || len(got[0].Entries) != 1 {
		t.Errorf("draw did not round-trip: %+v", got[0])
	}
}

// A garden that has been running for months will eventually have a truncated
// or half-written line in the log. One bad line must not cost the history.
func TestCorruptLineIsSkipped(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	if err := Append(Draw{Query: "good", Tokens: 10}); err != nil {
		t.Fatal(err)
	}
	p, _ := Path()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{not json\n")
	f.Close()
	if err := Append(Draw{Query: "later", Tokens: 20}); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Query != "later" {
		t.Errorf("loaded %+v, want the two good draws", got)
	}
}
