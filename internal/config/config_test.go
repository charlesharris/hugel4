package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingConfigIsEmptyNotAnError(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Kin) != 0 {
		t.Error("expected an empty config")
	}
}

// Kinship is symmetric: asking from any name gives the whole group, or soil
// drawn in the new bed would miss the old bed and not the other way round.
func TestKinIsSymmetric(t *testing.T) {
	c := &Config{}
	c.AddKin("hugel4", "hugel", "hugel-core")

	for _, from := range []string{"hugel4", "hugel", "hugel-core"} {
		got := c.KinOf(from)
		if len(got) != 3 {
			t.Errorf("KinOf(%q) = %v, want all three names", from, got)
		}
	}
	if got := c.KinOf("unrelated"); len(got) != 1 || got[0] != "unrelated" {
		t.Errorf("KinOf(unrelated) = %v, want just itself", got)
	}
	if got := c.KinOf(""); got != nil {
		t.Errorf("KinOf(\"\") = %v", got)
	}
}

func TestAddKinDeduplicates(t *testing.T) {
	c := &Config{}
	c.AddKin("a", "b", "b", "a", "c")
	if got := c.KinOf("a"); len(got) != 3 {
		t.Errorf("KinOf = %v, want three distinct names", got)
	}
}

func TestRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUGEL_HOME", home)

	c := &Config{}
	c.AddKin("hugel4", "hugel")
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(b), "\n") {
		t.Error("config file has no trailing newline")
	}

	back, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(back.KinOf("hugel")) != 2 {
		t.Errorf("round trip lost kinship: %v", back.KinOf("hugel"))
	}
}
