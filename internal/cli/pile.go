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
		shown = append(shown, e)
	}

	if *stats {
		return pileStats(shown)
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
	all, err := s.All()
	if err != nil {
		return err
	}
	var match *pile.Entry
	for _, e := range all {
		if strings.HasPrefix(e.ID, fs.Arg(0)) {
			if match != nil {
				return fmt.Errorf("id prefix %q is ambiguous", fs.Arg(0))
			}
			match = e
		}
	}
	if match == nil {
		return fmt.Errorf("no entry matching %q", fs.Arg(0))
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
