// Package cochange derives which parts of a repository change together.
//
// This is the one edge in the garden that costs nothing to write. Two
// directories that keep appearing in the same commit are neighbours by
// dependency rather than by wording, which is the relationship a ranking
// function cannot return at any budget and the reason the graph instinct kept
// resurfacing.
//
// Computed, never stored. Every graph hugel has kept died with the store it
// lived in, and the entries that survived did so because they were flat files.
// A coupling derived from git on demand cannot be lost in a migration, because
// there is nothing to migrate, and cannot go stale, which a stored edge does
// the moment the next commit lands.
package cochange

import (
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Thresholds carried over from hugel_v2's structural analysis, where they were
// tuned against real repositories. They are the hard-won part of this: the
// arithmetic is obvious and the cutoffs are not.
const (
	// MinScore is the share of the rarer directory's commits that must be
	// shared before a pair counts as coupled.
	MinScore = 0.3
	// MinCount stops a pair of directories that have only ever been touched
	// together twice from reading as a dependency.
	MinCount = 3
	// MaxPairs caps how much coupling one repository can contribute.
	MaxPairs = 30
	// maxDirsPerCommit ignores sweeping commits. A rename across forty
	// directories says they were all renamed, not that they depend on one
	// another, and left in it couples everything to everything.
	maxDirsPerCommit = 12
	// maxCommits bounds the history read. Coupling is a recent-shape question
	// and the whole log of an old repository is slow to no purpose.
	maxCommits = 2000
)

// Coupling maps a directory to the directories it changes with, and how
// strongly. It is symmetric: both directions are written.
type Coupling map[string]map[string]float64

// Score reports how strongly two directories change together, 0 when they do
// not. A nil Coupling answers 0 for everything, which is what makes this safe
// to consult without checking whether it was computed.
func (c Coupling) Score(a, b string) float64 {
	if c == nil || a == b {
		return 0
	}
	return c[a][b]
}

// Of reads a git repository's history and returns its directory coupling.
//
// A repository that is not git, or has no history worth speaking of, yields an
// empty Coupling and no error: absent coupling is the ordinary case, not a
// failure, and a caller should not have to decide what to do about it.
func Of(repo string) Coupling {
	if repo == "" {
		return nil
	}
	cmd := exec.Command("git", "-C", repo, "log",
		"--format=%H", "--name-only", "--no-merges", "-n", strconv.Itoa(maxCommits))
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return fromLog(string(out))
}

// fromLog is the whole computation, kept apart from git so it can be tested
// against a log rather than against a repository.
func fromLog(log string) Coupling {
	var (
		commits [][]string
		cur     map[string]bool
	)
	flush := func() {
		if len(cur) == 0 || len(cur) > maxDirsPerCommit {
			return
		}
		dirs := make([]string, 0, len(cur))
		for d := range cur {
			dirs = append(dirs, d)
		}
		sort.Strings(dirs)
		commits = append(commits, dirs)
	}
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "":
			continue
		case isSHA(line):
			flush()
			cur = map[string]bool{}
		case cur != nil:
			cur[Dir(line)] = true
		}
	}
	flush()

	together := map[[2]string]int{}
	alone := map[string]int{}
	for _, dirs := range commits {
		for _, d := range dirs {
			alone[d]++
		}
		for i := 0; i < len(dirs); i++ {
			for j := i + 1; j < len(dirs); j++ {
				together[[2]string{dirs[i], dirs[j]}]++
			}
		}
	}

	type pair struct {
		a, b  string
		score float64
	}
	var pairs []pair
	for k, n := range together {
		if n < MinCount {
			continue
		}
		// The rarer directory is the denominator. Sharing every one of a small
		// directory's commits is a strong signal; sharing the same number with
		// a directory touched constantly is not.
		fewer := alone[k[0]]
		if alone[k[1]] < fewer {
			fewer = alone[k[1]]
		}
		if fewer == 0 {
			continue
		}
		score := float64(n) / float64(fewer)
		if score < MinScore {
			continue
		}
		if score > 1 {
			score = 1
		}
		pairs = append(pairs, pair{k[0], k[1], score})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		if pairs[i].a != pairs[j].a {
			return pairs[i].a < pairs[j].a
		}
		return pairs[i].b < pairs[j].b
	})
	if len(pairs) > MaxPairs {
		pairs = pairs[:MaxPairs]
	}
	if len(pairs) == 0 {
		return nil
	}
	c := Coupling{}
	for _, p := range pairs {
		c.link(p.a, p.b, p.score)
		c.link(p.b, p.a, p.score)
	}
	return c
}

func (c Coupling) link(from, to string, score float64) {
	if c[from] == nil {
		c[from] = map[string]float64{}
	}
	c[from][to] = score
}

// Dir is how a path is named as an area. Coupling is a question about parts of
// a system rather than about files: two files in one directory changing
// together says nothing, and at file level the same relationship is spread too
// thin to clear any threshold.
//
// A bare name with no separator is a root-level file -- Makefile, go.mod,
// README.md -- and they all answer ".". That loses a path naming a top-level
// directory, which is genuinely ambiguous from the string alone, and the trade
// is worth it: git names files and never directories, so the ambiguity only
// arises for an entry's recorded paths, where "." is still a real area and an
// inconsistent one would be worse. Grouping Makefile with go.mod is right;
// giving Makefile an area of its own while go.mod gets "." is not.
func Dir(p string) string {
	p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), "/"))
	if p == "" {
		return ""
	}
	if !strings.Contains(p, "/") {
		return "."
	}
	// A path ending in a file name is answered with the directory holding it;
	// one that already names a directory keeps its own name.
	if path.Ext(p) != "" {
		return path.Dir(p)
	}
	return p
}

func isSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}
