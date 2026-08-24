package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/config"
	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/tend"
	"github.com/charris/hugel/internal/transcript"
	"github.com/charris/hugel/internal/yield"
)

func runGarden(args []string) error {
	fs := flag.NewFlagSet("garden", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel garden — where the work is

usage:
  hugel garden [--since 7d] [--limit 12]

One screen across every bed: what is in flight, what is ready to start, what
cannot move. Tab switches to the knowledge that came back from the last flight,
which is the same surface and the same sitting.

Work is read from bd and never written here. Starting, claiming and closing a
bead are bd's business.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		since   = fs.String("since", "7d", "how far back the knowledge side looks")
		limit   = fs.Int("limit", 12, "most rows to show per bed and per group")
		root    = fs.String("root", "", "transcript root (default ~/.claude/projects)")
		pileDir = fs.String("pile", "", "pile location (default ~/.hugel/pile)")
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// The survey is the slow part: one bd invocation per bed, before the screen
	// can be drawn. Say so rather than looking hung.
	fmt.Fprint(os.Stderr, "reading work…\r")
	work, problems := beads.Survey(transcript.BedDirs(sessions))
	fmt.Fprint(os.Stderr, "              \r")
	for _, p := range problems {
		if errors.Is(p, beads.ErrNoBd) {
			continue
		}
		fmt.Fprintf(os.Stderr, "hugel: %v\n", p)
	}

	f := yield.Filter{Since: cutoff}
	act := tend.Gather(entries, log, yield.Soil(sessions, log, entries, f), cutoff,
		cfg.KinOf(currentBed()))

	p := tea.NewProgram(
		tend.NewGarden(tend.Garden{Beds: work}, act, store, *limit),
		tea.WithAltScreen())
	_, err = p.Run()
	return err
}
