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

// The guard that stops a test writing into the gardener's own garden. Every
// test that leaked did so by omission, so the test for the guard is written
// from the omission: no HUGEL_HOME, and a refusal loud enough to fail.
func TestATestCannotReachTheRealGarden(t *testing.T) {
	t.Setenv("HUGEL_HOME", "") // as if it had never been set

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Home returned the gardener's real garden to a test")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "HUGEL_HOME") {
			t.Errorf("the refusal does not say how to fix it: %v", r)
		}
	}()
	got, err := Home()
	t.Fatalf("Home() = %q, %v; want a refusal", got, err)
}

// A garden pointed at a real directory is refused as firmly as one that was
// never pointed anywhere: setting HUGEL_HOME is not the point, staying out of
// the gardener's files is.
func TestASetButRealGardenIsRefusedToo(t *testing.T) {
	t.Setenv("HUGEL_HOME", filepath.Join(string(filepath.Separator), "var", "hugel-is-not-here"))

	defer func() {
		if recover() == nil {
			t.Fatal("a non-temporary HUGEL_HOME was accepted")
		}
	}()
	Home()
}

func TestATemporaryGardenIsFine(t *testing.T) {
	garden := t.TempDir()
	t.Setenv("HUGEL_HOME", garden)
	got, err := Home()
	if err != nil || got != garden {
		t.Fatalf("Home() = %q, %v; want %q", got, err, garden)
	}
}

// /tmp is reached through /private on macOS and t.TempDir is handed out under
// /var/folders, which is the same story. Comparing the spellings rather than
// the directories would refuse a garden that is in fact temporary.
func TestTheSandboxFollowsSymlinks(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a temporary garden was refused: %v", r)
		}
	}()
	Sandbox(filepath.Join(os.TempDir(), "hugel-test-garden"))
	Sandbox(filepath.Join(string(filepath.Separator), "tmp", "hugel-test-garden"))
}
