package cli

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/tend"
	"github.com/charris/hugel/internal/transcript"
	"github.com/charris/hugel/internal/yield"
)

func runTend(args []string) error {
	fs := flag.NewFlagSet("tend", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel tend — the working surface

usage:
  hugel tend [--since 7d] [--bed NAME]

What the garden did lately, and the judgement you pass on it. Bounded by time
rather than by backlog: sit down after three days away and you are shown three
days, not the whole pile.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		since   = fs.String("since", "7d", "how far back to look (7d, 2w, 48h)")
		bed     = fs.String("bed", "", "restrict to one bed")
		root    = fs.String("root", "", "transcript root (default ~/.claude/projects)")
		pileDir = fs.String("pile", "", "pile location (default ~/.hugel/pile)")
		limit   = fs.Int("limit", 25, "most entries to show per group")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	d, err := parseSince(*since)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-d)

	dir, err := pileRoot(*pileDir)
	if err != nil {
		return err
	}
	store, err := pile.Open(dir)
	if err != nil {
		return err
	}
	entries, err := store.All()
	if err != nil {
		return err
	}

	tdir := *root
	if tdir == "" {
		if tdir, err = transcript.DefaultRoot(); err != nil {
			return err
		}
	}
	sessions, _, err := transcript.LoadAll(tdir)
	if err != nil {
		return err
	}
	log, err := draws.Load()
	if err != nil {
		return err
	}

	f := yield.Filter{Since: cutoff, Bed: *bed}
	act := tend.Gather(entries, log, yield.Soil(sessions, log, entries, f), cutoff)

	p := tea.NewProgram(tend.New(act, store, *limit), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
