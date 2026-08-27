package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charris/hugel/internal/pile"
)

func runPile(args []string) error {
	if len(args) == 0 {
		return pileUsage()
	}
	switch args[0] {
	case "init":
		return runPileInit(args[1:])
	case "import":
		return runPileImport(args[1:])
	case "list":
		return runPileList(args[1:])
	case "show":
		return runPileShow(args[1:])
	case "review":
		return runPileReview(args[1:])
	case "-h", "--help", "help":
		return pileUsage()
	default:
		pileUsage()
		return fmt.Errorf("unknown pile command %q", args[0])
	}
}

func pileUsage() error {
	fmt.Print(`hugel pile — the knowledge store

usage:
  hugel pile init                  create the pile and its git repository
  hugel pile import <dir>          take in legacy markdown entries
  hugel pile list [--bed B]        what the pile knows
  hugel pile show <id>             one entry in full
  hugel pile review <id>...        record what you decided about an entry

The pile lives at ~/.hugel/pile unless HUGEL_PILE says otherwise.
`)
	return nil
}

func pileRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	return pile.DefaultRoot()
}

func runPileInit(args []string) error {
	fs := flag.NewFlagSet("pile init", flag.ContinueOnError)
	root := fs.String("pile", "", "pile location (default ~/.hugel/pile)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := pileRoot(*root)
	if err != nil {
		return err
	}
	s, err := pile.Init(dir)
	if err != nil {
		return err
	}
	if err := s.Commit("Start the pile"); err != nil {
		return err
	}
	n, err := s.Count()
	if err != nil {
		return err
	}
	fmt.Printf("pile at %s (%d entries)\n", s.Root, n)
	return nil
}

func runPileImport(args []string) error {
	fs := flag.NewFlagSet("pile import", flag.ContinueOnError)
	root := fs.String("pile", "", "pile location (default ~/.hugel/pile)")
	dry := fs.Bool("dry-run", false, "report what would be imported, write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("need a directory of legacy entries")
	}

	dir, err := pileRoot(*root)
	if err != nil {
		return err
	}
	s, err := pile.Open(dir)
	if err != nil {
		return err
	}

	var (
		counts   = map[pile.Result]int{}
		problems []error
		sources  []string
	)
	for _, src := range fs.Args() {
		entries, probs, err := pile.ImportLegacyDir(src)
		if err != nil {
			return err
		}
		problems = append(problems, probs...)
		sources = append(sources, sourceLabel(src))
		for _, e := range entries {
			if *dry {
				counts["would import"]++
				continue
			}
			res, err := s.Put(e)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			counts[res]++
		}
	}

	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "hugel: %v\n", p)
	}
	for _, k := range []pile.Result{"would import", pile.Created, pile.Updated, pile.Unchanged} {
		if counts[k] > 0 {
			fmt.Printf("%9d %s\n", counts[k], k)
		}
	}
	if *dry {
		return nil
	}
	if err := s.Commit("Import legacy entries from " + strings.Join(sources, ", ")); err != nil {
		return err
	}
	n, _ := s.Count()
	fmt.Printf("\npile now holds %d entries at %s\n", n, s.Root)
	return nil
}

func runPileList(args []string) error {
	fs := flag.NewFlagSet("pile list", flag.ContinueOnError)
	root := fs.String("pile", "", "pile location (default ~/.hugel/pile)")
	bed := fs.String("bed", "", "restrict to one bed")
	kind := fs.String("type", "", "restrict to one type")
	spike := fs.String("spike", "", "only what one spike found; \"any\" for every spike's findings")
	limit := fs.Int("limit", 30, "rows to show")
	stats := fs.Bool("stats", false, "summarise instead of listing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir, err := pileRoot(*root)
	if err != nil {
		return err
	}
	s, err := pile.Open(dir)
	if err != nil {
		return err
	}
	all, err := s.All()
	if err != nil {
		return err
	}

	var shown []*pile.Entry
	for _, e := range all {
		if *bed != "" && e.Bed != *bed {
			continue
		}
		if *kind != "" && string(e.Type) != *kind {
			continue
		}
		// "any" answers the question a reader usually has first -- what has
		// exploring actually produced -- which no single bead id can.
		if *spike == "any" && e.Source.Spike == "" {
			continue
		}
		if *spike != "" && *spike != "any" && e.Source.Spike != *spike {
			continue
		}
		shown = append(shown, e)
	}

	if *stats {
		return pileStats(shown)
	}
	if *spike != "" {
		fmt.Printf("%-16s %-11s %-16s %-10s  %s\n", "ID", "TYPE", "SPIKE", "WHEN", "TITLE")
		fmt.Println(strings.Repeat("─", 100))
		for i, e := range shown {
			if *limit > 0 && i >= *limit {
				fmt.Printf("… %d more\n", len(shown)-*limit)
				break
			}
			fmt.Printf("%-16s %-11s %-16s %-10s  %s\n",
				e.ID, e.Type, truncate(e.Source.Spike, 16),
				e.OccurredAt.Format("2006-01-02"), truncate(e.Title, 44))
		}
		fmt.Println(strings.Repeat("─", 100))
		fmt.Printf("%d entries\n", len(shown))
		return nil
	}
	fmt.Printf("%-16s %-11s %-8s %-10s  %s\n", "ID", "TYPE", "SCOPE", "WHEN", "TITLE")
	fmt.Println(strings.Repeat("─", 100))
	for i, e := range shown {
		if *limit > 0 && i >= *limit {
			fmt.Printf("… %d more\n", len(shown)-*limit)
			break
		}
		fmt.Printf("%-16s %-11s %-8s %-10s  %s\n",
			e.ID, e.Type, e.Scope, e.OccurredAt.Format("2006-01-02"), truncate(e.Title, 52))
	}
	fmt.Println(strings.Repeat("─", 100))
	fmt.Printf("%d entries\n", len(shown))
	return nil
}

func pileStats(entries []*pile.Entry) error {
	byType, byBed, byReview := map[string]int{}, map[string]int{}, map[string]int{}
	for _, e := range entries {
		byType[string(e.Type)]++
		byBed[e.Bed]++
		byReview[string(e.Review)]++
	}
	show := func(title string, m map[string]int) {
		fmt.Printf("\n%s\n", title)
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if m[keys[i]] != m[keys[j]] {
				return m[keys[i]] > m[keys[j]]
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			fmt.Printf("  %5d  %s\n", m[k], k)
		}
	}
	fmt.Printf("%d entries\n", len(entries))
	show("by type", byType)
	show("by bed", byBed)
	show("by review", byReview)
	return nil
}

func runPileShow(args []string) error {
	fs := flag.NewFlagSet("pile show", flag.ContinueOnError)
	root := fs.String("pile", "", "pile location (default ~/.hugel/pile)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("need an entry id")
	}
	dir, err := pileRoot(*root)
	if err != nil {
		return err
	}
	s, err := pile.Open(dir)
	if err != nil {
		return err
	}
	match, err := s.Get(fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("%s  %s  %s\n", match.ID, match.Type, match.Scope)
	fmt.Printf("%s\n\n", match.Title)
	fmt.Printf("bed        %s\n", match.Bed)
	fmt.Printf("status     %s (%s)\n", match.Status, match.Review)
	fmt.Printf("occurred   %s\n", match.OccurredAt.Format("2006-01-02"))
	if len(match.Tags) > 0 {
		fmt.Printf("tags       %s\n", strings.Join(match.Tags, ", "))
	}
	if len(match.Paths) > 0 {
		fmt.Printf("paths      %s\n", strings.Join(match.Paths, ", "))
	}
	for _, l := range match.Links {
		fmt.Printf("%-10s %s\n", l.Rel, l.ID)
	}
	fmt.Printf("source     %s", match.Source.Extractor)
	if match.Source.ImportedFrom != "" {
		fmt.Printf(" from %s", match.Source.ImportedFrom)
	}
	fmt.Printf("\n\n%s\n", match.Body)
	return nil
}

// sourceLabel names an import source usefully. Every legacy corpus is in a
// directory called "entries", so the basename alone says nothing.
func sourceLabel(path string) string {
	clean := filepath.Clean(path)
	parent, base := filepath.Split(clean)
	if p := filepath.Base(filepath.Clean(parent)); p != "." && p != string(filepath.Separator) {
		return filepath.Join(p, base)
	}
	return base
}

// runPileReview records a human's standing on entries.
//
// There is deliberately no inbox here — no "hugel pile review" that walks 247
// unreviewed entries. Working a queue that size costs more than the pile saves,
// which is the first thing this garden refuses to build. Review is driven by
// what a draw surfaced: the handful of entries that actually reached a session
// are the only ones whose quality has cost anything yet, and you are holding the
// context to judge them exactly then.
func runPileReview(args []string) error {
	fs := flag.NewFlagSet("pile review", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel pile review — record what you decided about an entry

usage:
  hugel pile review <id>... --accept        vouch for it; soil ranks it higher
  hugel pile review <id>... --reject        it is wrong; drop it from soil
  hugel pile review <id>... --abandon       what it describes is dead
  hugel pile review <id>... --unreview      take back a verdict
  hugel pile review <id> --superseded-by <id>

Ids come from a draw. Review what soil actually surfaced rather than working
through the pile: those are the entries whose quality has cost you anything.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		root    = fs.String("pile", "", "pile location (default ~/.hugel/pile)")
		accept  = fs.Bool("accept", false, "a human vouches for this entry")
		reject  = fs.Bool("reject", false, "this entry is wrong; keep it out of soil")
		abandon = fs.Bool("abandon", false, "what this entry describes was abandoned")
		unsay   = fs.Bool("unreview", false, "take back a verdict; return it to unreviewed")
		by      = fs.String("superseded-by", "", "id of the entry that replaced this one")
		why     = fs.String("why", "", "the reason, recorded in the pile's git log")
	)
	ids, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		fs.Usage()
		return fmt.Errorf("need at least one entry id")
	}

	var chosen []string
	for name, on := range map[string]bool{
		"--accept": *accept, "--reject": *reject,
		"--abandon": *abandon, "--unreview": *unsay, "--superseded-by": *by != "",
	} {
		if on {
			chosen = append(chosen, name)
		}
	}
	sort.Strings(chosen)
	if len(chosen) != 1 {
		fs.Usage()
		if len(chosen) == 0 {
			return fmt.Errorf("say what you decided")
		}
		return fmt.Errorf("%s: pick one", strings.Join(chosen, " and "))
	}

	dir, err := pileRoot(*root)
	if err != nil {
		return err
	}
	store, err := pile.Open(dir)
	if err != nil {
		return err
	}

	if *by != "" {
		if len(ids) != 1 {
			return fmt.Errorf("supersede one entry at a time")
		}
		old, newer, err := store.Supersede(ids[0], *by)
		if err != nil {
			return err
		}
		fmt.Printf("superseded  %s  %s\n", old.ID, truncate(old.Title, 60))
		fmt.Printf("         by  %s  %s\n", newer.ID, truncate(newer.Title, 60))
		return store.Commit(commitMessage("Supersede", []string{old.Title}, *why))
	}

	verb, label := "Accept", "accepted"
	switch {
	case *reject:
		verb, label = "Reject", "rejected"
	case *abandon:
		verb, label = "Abandon", "abandoned"
	case *unsay:
		verb, label = "Unreview", "unreviewed"
	}

	var titles []string
	for _, id := range ids {
		var (
			e   *pile.Entry
			res pile.Result
			err error
		)
		switch {
		case *accept:
			e, res, err = store.SetReview(id, pile.Accepted)
		case *reject:
			e, res, err = store.SetReview(id, pile.Rejected)
		case *abandon:
			e, res, err = store.SetStatus(id, pile.Abandoned)
		case *unsay:
			// A verdict is a human's, so taking one back is too. It returns the
			// entry to unreviewed rather than to the opposite verdict: not
			// knowing is a real state, and pretending otherwise is how a wrong
			// keystroke becomes a fact.
			e, res, err = store.SetReview(id, pile.Unreviewed)
		}
		if err != nil {
			return err
		}
		if res == pile.Unchanged {
			fmt.Printf("already %-10s %s  %s\n", label, e.ID, truncate(e.Title, 60))
			continue
		}
		fmt.Printf("%-18s %s  %s\n", label, e.ID, truncate(e.Title, 60))
		titles = append(titles, e.Title)
	}
	if len(titles) == 0 {
		return nil // nothing changed, so nothing to commit
	}
	return store.Commit(commitMessage(verb, titles, *why))
}

// commitMessage puts the reason in the log rather than in the entry. The pile's
// git history is already its temporal layer, so why an entry was thrown out
// costs nothing to keep there and would cost a schema field anywhere else.
func commitMessage(verb string, titles []string, why string) string {
	subject := fmt.Sprintf("%s %d entries", verb, len(titles))
	if len(titles) == 1 {
		subject = fmt.Sprintf("%s %q", verb, truncate(titles[0], 60))
	}
	if why == "" {
		return subject
	}
	return subject + "\n\n" + why
}
