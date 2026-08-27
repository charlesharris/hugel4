package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charris/hugel/internal/compost"
	"github.com/charris/hugel/internal/config"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/redact"
	"github.com/charris/hugel/internal/tender"
	"github.com/charris/hugel/internal/transcript"
)

func runCompost(args []string) error {
	fs := flag.NewFlagSet("compost", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel compost — turn spent sessions into knowledge

Distils a session, scrubs it, extracts entries from what was deliberately
recorded during it -- commit messages, beads closed with a reason -- and puts
them in the pile.

Entries arrive unreviewed. The extractor proposes a scope; you decide whether
it was right.

usage:
  hugel compost --session ID     compost one session
  hugel compost --all            compost every session
  hugel compost --all --dry-run  show what would be composted
  hugel compost --all --new      only sessions whose transcript has changed

flags:
`)
		fs.PrintDefaults()
	}
	var (
		session = fs.String("session", "", "session id prefix")
		all     = fs.Bool("all", false, "compost every session")
		dry     = fs.Bool("dry-run", false, "show what would be written, write nothing")
		bed     = fs.String("bed", "", "restrict to one bed")
		root    = fs.String("root", "", "transcript root (default ~/.claude/projects)")
		pileDir = fs.String("pile", "", "pile location (default ~/.hugel/pile)")
		onlyNew = fs.Bool("new", false, "only sessions whose transcript changed since the last run")
		quiet   = fs.Bool("quiet", false, "write nothing to stdout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *session == "" && !*all {
		fs.Usage()
		return fmt.Errorf("need --session or --all")
	}
	return compostSessions(compostOpts{
		session: *session, bed: *bed, root: *root, pileDir: *pileDir,
		all: *all, dry: *dry, onlyNew: *onlyNew, quiet: *quiet,
	})
}

// compostOpts is what a compost run needs. It exists so the SessionStart hook
// can ask for the same run the command does rather than growing a second one.
type compostOpts struct {
	session, bed, root, pileDir string
	all, dry, onlyNew, quiet    bool
}

func compostSessions(o compostOpts) error {
	out := io.Discard
	if !o.quiet {
		out = os.Stdout
	}

	dir := o.root
	if dir == "" {
		var err error
		if dir, err = transcript.DefaultRoot(); err != nil {
			return err
		}
	}
	sessions, err := loadForCompost(dir, o.onlyNew)
	if err != nil {
		return err
	}

	var chosen []*transcript.Session
	for _, s := range sessions {
		if o.bed != "" && s.Bed != o.bed {
			continue
		}
		if o.session != "" && !strings.HasPrefix(s.ID, o.session) {
			continue
		}
		chosen = append(chosen, s)
	}
	if len(chosen) == 0 {
		if o.onlyNew {
			return markComposted() // nothing has changed; that is the normal case
		}
		return fmt.Errorf("no sessions matched")
	}

	var store *pile.Store
	if !o.dry {
		pd := o.pileDir
		if pd == "" {
			if pd, err = pile.DefaultRoot(); err != nil {
				return err
			}
		}
		if store, err = pile.Open(pd); err != nil {
			return err
		}
	}

	scrub := redact.FromEnv()
	ex := compost.Heuristic{}
	budget := compost.DefaultBudget()

	var (
		counts    = map[pile.Result]int{}
		proposed  int
		linked    int
		redacted  int
		cost      float64
		anyChange bool
	)
	fmt.Fprintf(out, "%-10s %-16s %7s %8s  %s\n", "SESSION", "BED", "RECORDS", "ENTRIES", "")
	fmt.Fprintln(out, strings.Repeat("─", 62))

	for _, s := range chosen {
		d := compost.Distil(s, budget)
		// What the session was for lives with the tenders, not in the
		// transcript: the harness files a session under the directory it ran
		// in, and for a spike that directory is its worktree.
		d.Spike = tender.SpikeAt(d.Directory)
		// Where each commit landed, asked of the repository the session ran
		// in. A commit record is parsed from the message the agent typed and
		// carries no paths of its own.
		compost.ResolveFiles(d, filesBySubject(d.Directory))
		redacted += redact.Total(d.Redact(scrub))
		h, err := ex.Extract(d)
		if err != nil {
			return fmt.Errorf("extract %s: %w", s.ID, err)
		}
		cost += h.CostUSD
		proposed += len(h.Entries)
		// A revert is the pile's only evidence from outside the session that
		// made the claim: the garden went back and took the change out.
		linked += compost.LinkReverts(h.Entries, func(id string) bool {
			for _, e := range h.Entries {
				if e.Identity() == id {
					return true
				}
			}
			return store != nil && store.Has(id)
		})

		var wrote []string
		for _, e := range h.Entries {
			if o.dry {
				counts["would write"]++
				continue
			}
			res, err := store.Put(e)
			if err != nil {
				fmt.Fprintf(os.Stderr, "hugel: %v\n", err)
				continue
			}
			counts[res]++
			if res != pile.Unchanged {
				anyChange = true
				wrote = append(wrote, string(res))
			}
		}
		_ = wrote
		fmt.Fprintf(out, "%-10s %-16s %7d %8d\n",
			truncate(s.ID, 10), truncate(s.Bed, 16), len(d.Records), len(h.Entries))
	}

	fmt.Fprintln(out, strings.Repeat("─", 62))
	for _, k := range []pile.Result{"would write", pile.Created, pile.Updated, pile.Unchanged} {
		if counts[k] > 0 {
			fmt.Fprintf(out, "%9d %s\n", counts[k], k)
		}
	}
	fmt.Fprintf(out, "\nextractor %s/%s proposed %d entries for %s\n",
		ex.Name(), ex.Version(), proposed, money(cost))
	if redacted > 0 {
		fmt.Fprintf(out, "redacted %d credentials before extraction\n", redacted)
	}
	if linked > 0 {
		fmt.Fprintf(out, "linked %d revert(s) to the decision they took back\n", linked)
	}
	if o.dry || store == nil {
		return nil
	}
	if anyChange {
		if err := store.Commit(fmt.Sprintf("Compost %d sessions", len(chosen))); err != nil {
			return err
		}
	}
	n, _ := store.Count()
	fmt.Fprintf(out, "pile now holds %d entries\n", n)
	return markComposted()
}

// loadForCompost parses transcripts, optionally only those written since the
// last run. Parsing is the whole cost of composting, so filtering by file time
// before parsing is what keeps an automatic run cheap as sessions accumulate --
// otherwise every new session makes every future run slower.
func loadForCompost(dir string, onlyNew bool) ([]*transcript.Session, error) {
	paths, err := transcript.Discover(dir)
	if err != nil {
		return nil, err
	}
	var since time.Time
	if onlyNew {
		if mark, err := compostMark(); err == nil {
			if st, err := os.Stat(mark); err == nil {
				// A transcript is appended to while a session runs, so a file
				// touched in the same minute as the mark is treated as new.
				since = st.ModTime().Add(-time.Minute)
			}
		}
	}

	var sessions []*transcript.Session
	for _, p := range paths {
		if !since.IsZero() {
			st, err := os.Stat(p)
			if err != nil || st.ModTime().Before(since) {
				continue
			}
		}
		s, err := transcript.ParseFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hugel: skipped %v\n", err)
			continue
		}
		if len(s.Requests) == 0 {
			continue // nothing was spent here
		}
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Start.Before(sessions[j].Start) })
	return sessions, nil
}

// compostMark is the file whose modification time says when composting last
// ran. A timestamp in a file beats a timestamp in a config: it is one syscall
// to read, and touching it is the whole write.
func compostMark() (string, error) {
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "composted"), nil
}

func markComposted() error {
	p, err := compostMark()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	now := time.Now()
	return os.Chtimes(p, now, now)
}

// filesBySubject resolves a commit message subject to the files that commit
// changed, in the repository a session ran in.
//
// A unique match or nothing. Two commits can share a subject -- a revert and
// the thing it reverted very nearly do -- and picking one of them would attach
// another change's files to this entry, which reads as evidence and is wrong.
// Returning nothing is the honest answer and the extractor already handles it.
func filesBySubject(repo string) func(string) []string {
	if repo == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		// Not a repository, or a worktree whose .git file has gone with it.
		// Composting still works; entries simply keep the paths their prose
		// named, which is where they were before this.
		return nil
	}
	return func(subject string) []string {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			return nil
		}
		out, err := exec.Command("git", "-C", repo, "log", "--all",
			"--fixed-strings", "--grep="+subject, "--format=%H", "-n", "2").Output()
		if err != nil {
			return nil
		}
		shas := strings.Fields(string(out))
		if len(shas) != 1 {
			return nil
		}
		files, err := exec.Command("git", "-C", repo, "show",
			"--name-only", "--format=", "--no-renames", shas[0]).Output()
		if err != nil {
			return nil
		}
		return strings.Fields(string(files))
	}
}
