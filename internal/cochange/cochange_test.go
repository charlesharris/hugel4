package cochange

import "testing"

// A synthetic log rather than a repository: the arithmetic is the part worth
// pinning, and a fixture repository would test git.
func logOf(commits ...[]string) string {
	shas := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccc", "dddddddddddddddddddddddddddddddddddddddd",
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "ffffffffffffffffffffffffffffffffffffffff",
	}
	out := ""
	for i, files := range commits {
		out += shas[i%len(shas)] + "\n"
		for _, f := range files {
			out += f + "\n"
		}
		out += "\n"
	}
	return out
}

func TestDirectoriesThatChangeTogetherAreCoupled(t *testing.T) {
	// api and store move together four times; docs rides along once.
	log := logOf(
		[]string{"api/a.go", "store/s.go"},
		[]string{"api/b.go", "store/s.go"},
		[]string{"api/c.go", "store/t.go"},
		[]string{"api/d.go", "store/t.go"},
		[]string{"api/e.go", "docs/readme.md"},
	)
	c := fromLog(log)
	if got := c.Score("api", "store"); got < MinScore {
		t.Errorf("api<->store scored %.2f, want a coupling above %.2f", got, MinScore)
	}
	// Symmetric: a caller should not have to know which way round to ask.
	if c.Score("api", "store") != c.Score("store", "api") {
		t.Error("coupling is not symmetric")
	}
	// One shared commit is a coincidence, not a dependency.
	if got := c.Score("api", "docs"); got != 0 {
		t.Errorf("api<->docs scored %.2f on a single shared commit, want 0", got)
	}
}

// The denominator is the rarer directory. A directory touched in every commit
// would otherwise look coupled to everything it happened to be near.
func TestTheRarerDirectorySetsTheScore(t *testing.T) {
	log := logOf(
		[]string{"cli/a.go", "rare/r.go"},
		[]string{"cli/b.go", "rare/r.go"},
		[]string{"cli/c.go", "rare/r.go"},
		[]string{"cli/d.go"}, []string{"cli/e.go"}, []string{"cli/f.go"},
	)
	c := fromLog(log)
	// rare appears 3 times and shares all 3 with cli: total coupling.
	if got := c.Score("rare", "cli"); got != 1 {
		t.Errorf("score = %.2f, want 1 -- every commit rare had was shared", got)
	}
}

// A sweeping commit says everything was renamed, not that everything depends on
// everything. Left in, one such commit couples the whole repository.
func TestASweepingCommitCouplesNothing(t *testing.T) {
	var wide []string
	for _, d := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n"} {
		wide = append(wide, d+"/f.go")
	}
	c := fromLog(logOf(wide, wide, wide, wide))
	if len(c) != 0 {
		t.Errorf("a repo whose only commits touch everything produced %d coupled areas, want none", len(c))
	}
}

func TestDirNamesTheArea(t *testing.T) {
	cases := map[string]string{
		"internal/pile/entry.go": "internal/pile",
		"internal/pile":          "internal/pile",
		"Makefile":               ".",
		"README.md":              ".",
		"":                       "",
	}
	for in, want := range cases {
		if got := Dir(in); got != want {
			t.Errorf("Dir(%q) = %q, want %q", in, got, want)
		}
	}
}

// A nil coupling is the ordinary case -- a bed that is not a git repository,
// or one too young to have a shape -- and consulting it must be safe.
func TestNoCouplingIsSafeToConsult(t *testing.T) {
	var c Coupling
	if c.Score("a", "b") != 0 {
		t.Error("a nil coupling claimed a score")
	}
	if Of("") != nil || Of("/nonexistent/nowhere") != nil {
		t.Error("a directory that is not a repository produced coupling")
	}
}
