package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/config"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/transcript"
)

func runBed(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(`hugel bed — the projects the garden knows

usage:
  hugel bed kin <name> <other>...   record that these names are one project
  hugel bed list                    every bed: where it lives and what work is open

A project renamed across rewrites leaves knowledge filed under every name it
ever had. Without kinship, soil drawn in the new bed penalises the old bed's
entries as another project's business -- which means a project's oldest and
most settled decisions rank lowest exactly because they are old.
`)
		return nil
	}
	switch args[0] {
	case "kin":
		return runBedKin(args[1:])
	case "list":
		return runBedList(args[1:])
	default:
		return fmt.Errorf("unknown bed command %q", args[0])
	}
}

func runBedKin(args []string) error {
	fs := flag.NewFlagSet("bed kin", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("need a bed and at least one other name")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AddKin(fs.Arg(0), fs.Args()[1:]...)
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("%s is also known as %s\n", fs.Arg(0), strings.Join(cfg.KinOf(fs.Arg(0)), ", "))
	return nil
}

// runBedList shows every bed the garden knows: where it lives, and what work is
// open there. The directory comes from transcripts rather than configuration --
// a bed is named for a working directory, so the sessions that ran in it already
// say where it is -- and the work comes from bd, which owns it.
func runBedList(args []string) error {
	fs := flag.NewFlagSet("bed list", flag.ContinueOnError)
	root := fs.String("root", "", "transcript root (default ~/.claude/projects)")
	quick := fs.Bool("no-work", false, "skip reading bd; just say where beds live")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	dir := *root
	if dir == "" {
		if dir, err = transcript.DefaultRoot(); err != nil {
			return err
		}
	}
	sessions, _, err := transcript.LoadAll(dir)
	if err != nil {
		return err
	}
	dirs := transcript.BedDirs(sessions)
	if len(dirs) == 0 {
		fmt.Println("no beds yet: hugel learns them from session transcripts")
		return nil
	}

	work := map[string]*beads.Work{}
	if !*quick {
		found, problems, err := surveyBeds(dirs)
		if err != nil {
			return err
		}
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "hugel: %v\n", p)
		}
		work = found
	}

	names := make([]string, 0, len(dirs))
	for b := range dirs {
		names = append(names, b)
	}
	sort.Strings(names)

	fmt.Printf("%-16s %6s %6s %8s  %-34s %s\n", "BED", "READY", "ACTIVE", "BLOCKED", "WHERE", "ALSO KNOWN AS")
	fmt.Println(strings.Repeat("─", 100))
	for _, n := range names {
		ready, active, blocked := "—", "—", "—"
		if w, ok := work[n]; ok {
			r, a, b := w.Counts()
			ready, active, blocked = strconv.Itoa(r), strconv.Itoa(a), strconv.Itoa(b)
		}
		var also string
		if kin := cfg.KinOf(n); len(kin) > 1 {
			var others []string
			for _, k := range kin {
				if k != n {
					others = append(others, k)
				}
			}
			also = strings.Join(others, ", ")
		}
		fmt.Printf("%-16s %6s %6s %8s  %-34s %s\n",
			truncate(n, 16), ready, active, blocked, truncate(home(dirs[n]), 34), also)
	}
	reportUnlocated(dirs)
	return nil
}

// reportUnlocated names beds the pile has knowledge about but that no transcript
// places anywhere. A bed is located from the sessions that ran in it, so an
// imported corpus or a project not worked in since hugel started watching has
// knowledge with nowhere to point. Saying so beats a table that quietly lists
// fewer beds than the garden actually knows.
func reportUnlocated(dirs map[string]string) {
	root, err := pile.DefaultRoot()
	if err != nil {
		return
	}
	store, err := pile.Open(root)
	if err != nil {
		return
	}
	entries, err := store.All()
	if err != nil {
		return
	}
	counts := map[string]int{}
	for _, e := range entries {
		if e.Bed == "" || dirs[e.Bed] != "" {
			continue
		}
		counts[e.Bed]++
	}
	if len(counts) == 0 {
		return
	}
	names := make([]string, 0, len(counts))
	for b := range counts {
		names = append(names, b)
	}
	sort.Slice(names, func(i, j int) bool { return counts[names[i]] > counts[names[j]] })

	var parts []string
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s (%d)", n, counts[n]))
	}
	fmt.Printf("\nknowledge with nowhere to point: %s\n", strings.Join(parts, ", "))
	fmt.Println("these beds have pile entries but no session recorded a directory for them")
}

// surveyBeds reads the work in every bed that tracks any. A missing bd is not
// an error: hugel does not require an issue tracker to account for a garden.
func surveyBeds(dirs map[string]string) (map[string]*beads.Work, []error, error) {
	found, problems := beads.Survey(dirs)
	out := make(map[string]*beads.Work, len(found))
	for _, w := range found {
		out[w.Bed] = w
	}
	var kept []error
	for _, p := range problems {
		if errors.Is(p, beads.ErrNoBd) {
			return out, nil, nil
		}
		kept = append(kept, p)
	}
	return out, kept, nil
}

// home shortens a path the way a person writing one would.
func home(path string) string {
	h, err := os.UserHomeDir()
	if err != nil || !strings.HasPrefix(path, h+"/") {
		return path
	}
	return "~" + strings.TrimPrefix(path, h)
}
