package compost

import "sort"

// ResolveFiles fills in what each commit in the digest actually changed, and
// reports how many were resolved.
//
// Kept out of the extractor and out of this package's own reach on purpose:
// composting reads a transcript and nothing else, which is what makes it
// testable against a fixture and safe to re-run. Where a commit landed is a
// question only the repository can answer, so the caller who has the repository
// answers it, the same way it answers which spike a session belonged to.
//
// filesOf returns nothing for a subject it cannot resolve to exactly one
// commit, and a record with nothing resolved keeps no files at all. An entry
// claiming the wrong files is worse than one claiming none: the wrong ones read
// as evidence, and every reader downstream believes them.
func ResolveFiles(d *Digest, filesOf func(subject string) []string) int {
	if d == nil || filesOf == nil {
		return 0
	}
	resolved := 0
	for i := range d.Records {
		if d.Records[i].Kind != KindCommit {
			// A memory or a bead close is a claim about how things are, not a
			// change that touched anything. Giving it files would say it was
			// about code it never moved.
			continue
		}
		files := filesOf(d.Records[i].Subject)
		if len(files) == 0 {
			continue
		}
		d.Records[i].Files = capPaths(files)
		resolved++
	}
	return resolved
}

// maxFiles bounds what one entry may claim. A sweeping commit touches dozens
// and an entry that names all of them has stopped saying where the work was.
// The same cap the prose scraper already uses, so the two sources agree about
// how much an entry is allowed to be about.
const maxFiles = 12

// capPaths sorts and bounds a path list. Sorted because an entry's identity is
// stable knowledge and git's output order is not; bounded because a cap that
// takes whatever came first is a cap on nothing in particular.
func capPaths(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	if len(out) > maxFiles {
		out = out[:maxFiles]
	}
	return out
}
