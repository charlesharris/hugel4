package complete

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/tender"
	"github.com/charris/hugel/internal/transcript"
)

// Candidate is one completion: what gets inserted, and what the shell shows
// beside it. The description is what makes a list of sixteen-hex-digit entry
// ids usable at all.
type Candidate struct {
	Value string
	Desc  string
}

// Line renders a candidate as a shell completer reads it: one line of
// "value:description".
//
// The colon is the separator, so one inside a value would split it in the
// wrong place. Nothing hugel completes contains a colon today, but a bead id
// comes from bd and a bed name from a directory, and neither is hugel's to
// promise. A description is a sentence somebody wrote and may hold a newline,
// which the shell would read as a second candidate.
func (c Candidate) Line() string {
	value := strings.ReplaceAll(c.Value, ":", `\:`)
	return value + ":" + strings.Join(strings.Fields(c.Desc), " ")
}

// For enumerates a source, relative to the directory the shell is standing in.
//
// Nothing here may load transcripts. hugel tender --list takes 780ms and
// hugel bed list 1.5s, almost all of it scanning ~/.claude/projects, and a
// completion that takes a second is a completion people stop pressing tab for.
// Every source below is a directory read, a bd call, or a git call.
//
// Errors are swallowed on purpose. A shell asking what to offer is not a place
// to report that bd is missing or the pile has not been created: the honest
// answer to "what are the candidates" is then "none", and the user finds out
// what is wrong by running the command they were completing.
func For(src Source, cwd string) []Candidate {
	switch src {
	case Ready:
		return readyIn(cwd)
	case Tended:
		return tendersWhere(func(t tender.Tender) bool { return t.Done() })
	case Tenders:
		return tendersWhere(func(tender.Tender) bool { return true })
	case Live:
		return tendersWhere(func(t tender.Tender) bool { return t.Running() })
	case Spikes:
		return tendersWhere(func(t tender.Tender) bool { return t.Spike })
	case Entries:
		return entries()
	case Beds:
		return bedNames()
	case Sessions:
		return sessions()
	case Types:
		return types()
	case Branches:
		return branches(cwd)
	case Hooks:
		return []Candidate{{Value: "session-start", Desc: "compost what has changed, before work begins"}}
	}
	return nil
}

// Sources is every source hugel answers itself, for the error message when
// someone asks for one that does not exist.
func Sources() []string {
	var out []string
	for _, s := range []Source{Ready, Tended, Tenders, Live, Spikes,
		Entries, Beds, Sessions, Types, Branches, Hooks} {
		out = append(out, string(s))
	}
	return out
}

// readyIn offers exactly the queue a tender pulls from, by asking the same
// function dispatch asks. A completion that offered epics and work labelled
// needs-attention would be offering beads that dispatch refuses to start.
func readyIn(cwd string) []Candidate {
	repo := beads.RepoOf(cwd)
	if repo == "" {
		return nil
	}
	w, err := beads.ReadReady(repo)
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, r := range beads.Queue([]*beads.Work{w}, "", nil) {
		out = append(out, Candidate{Value: r.Bead.ID, Desc: r.Bead.Title})
	}
	return out
}

// tendersWhere reads ~/.hugel/tenders, which is a directory of small JSON
// files and costs nothing to walk.
func tendersWhere(keep func(tender.Tender) bool) []Candidate {
	all, err := tender.List()
	if err != nil {
		return nil
	}
	var out []Candidate
	for _, t := range all {
		if !keep(t) {
			continue
		}
		desc := t.State()
		if t.Title != "" {
			desc += " — " + t.Title
		}
		out = append(out, Candidate{Value: t.Bead, Desc: desc})
	}
	return out
}

func entries() []Candidate {
	all, err := allEntries()
	if err != nil {
		return nil
	}
	out := make([]Candidate, 0, len(all))
	for _, e := range all {
		out = append(out, Candidate{Value: e.ID, Desc: string(e.Type) + " · " + e.Title})
	}
	return out
}

func allEntries() ([]*pile.Entry, error) {
	root, err := pile.DefaultRoot()
	if err != nil {
		return nil, err
	}
	s, err := pile.Open(root)
	if err != nil {
		return nil, err
	}
	return s.All()
}

// bedNames is derived from what the pile and the tenders already record,
// rather than from transcripts.
//
// hugel bed list builds a better answer by reading every session's working
// directory, and takes a second and a half to do it. A bed worth completing is
// a bed hugel holds knowledge or work about, and both of those are on disk in
// ~/.hugel.
func bedNames() []Candidate {
	seen := map[string]bool{}
	if all, err := allEntries(); err == nil {
		for _, e := range all {
			if e.Bed != "" {
				seen[e.Bed] = true
			}
		}
	}
	if all, err := tender.List(); err == nil {
		for _, t := range all {
			if t.Bed != "" {
				seen[t.Bed] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Candidate, 0, len(names))
	for _, n := range names {
		out = append(out, Candidate{Value: n})
	}
	return out
}

// sessions lists transcript files without parsing one. Discover is a directory
// walk; LoadAll is the thing that takes a second.
func sessions() []Candidate {
	root, err := transcript.DefaultRoot()
	if err != nil {
		return nil
	}
	paths, err := transcript.Discover(root)
	if err != nil {
		return nil
	}
	out := make([]Candidate, 0, len(paths))
	for i := len(paths) - 1; i >= 0; i-- { // newest last from Discover, so reverse
		id := strings.TrimSuffix(filepath.Base(paths[i]), ".jsonl")
		out = append(out, Candidate{Value: id, Desc: bedOfDir(filepath.Dir(paths[i]))})
	}
	return out
}

// bedOfDir names the bed a transcript directory belongs to. The harness encodes
// a project path by replacing separators with dashes, so the last segment is
// the working directory's name, which is what hugel calls a bed.
func bedOfDir(dir string) string {
	base := filepath.Base(dir)
	if i := strings.LastIndex(base, "-"); i >= 0 && i+1 < len(base) {
		return base[i+1:]
	}
	return base
}

func types() []Candidate {
	return []Candidate{
		{Value: string(pile.Decision), Desc: "a choice that was made, and why"},
		{Value: string(pile.Pattern), Desc: "an approach that has proven to work"},
		{Value: string(pile.Discovery), Desc: "something learned that changes understanding"},
		{Value: string(pile.Failure), Desc: "a wrong turn, and what it taught"},
		{Value: string(pile.Constraint), Desc: "a hard boundary that shapes design"},
	}
}

// branches completes --into, which is the one flag whose wrong value is
// expensive: the gate merges onto it.
func branches(cwd string) []Candidate {
	repo := cwd
	if repo == "" {
		repo, _ = os.Getwd()
	}
	cmd := exec.Command("git", "-C", repo, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil
		}
		return nil
	}
	var cands []Candidate
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			cands = append(cands, Candidate{Value: line})
		}
	}
	return cands
}
