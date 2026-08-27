package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charris/hugel/internal/cochange"
	"github.com/charris/hugel/internal/config"
	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/redact"
	"github.com/charris/hugel/internal/soil"
)

func runSoil(args []string) error {
	fs := flag.NewFlagSet("soil", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel soil — draw context from the pile

usage:
  hugel soil "what the work is about" [--bed NAME]

Soil is the small, selected part of the pile delivered to one piece of work.
The budget is the feature: what soil costs is set by how much of it enters a
session and how early, not by how much the pile holds.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		bed     = fs.String("bed", "", "the bed the work is in (default: this directory's name)")
		kind    = fs.String("type", "", "only this kind of knowledge")
		limit   = fs.Int("limit", 8, "most entries to return")
		budget  = fs.Int("budget", 1500, "most tokens of soil to deliver")
		asJSON  = fs.Bool("json", false, "emit JSON")
		pileDir = fs.String("pile", "", "pile location (default ~/.hugel/pile)")
	)
	words, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(words) == 0 {
		fs.Usage()
		return fmt.Errorf("need something to ask the pile about")
	}

	if *bed == "" {
		*bed = currentBed()
	}

	ix, err := openIndex(*pileDir)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Drawn where the gardener is standing: coupling is a fact about the
	// project in front of them, and the working directory is which project
	// that is.
	repo, _ := os.Getwd()
	s := ix.Draw(soil.Query{
		Text: strings.Join(words, " "), Bed: *bed, Kin: cfg.KinOf(*bed),
		Type: pile.Type(*kind), Limit: *limit, Budget: *budget,
		Coupling: cochange.Of(repo),
	})
	recordDraw(s, *budget)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	fmt.Print(s.Render())
	return nil
}

// recordDraw notes what the pile handed over, so that whether an agent reaches
// for it — and whether what it got was worth keeping — become countable rather
// than asserted.
//
// A failed write must never cost the caller its soil: this runs inside live
// sessions, and an instrument that can break the thing it measures is worse
// than no instrument.
func recordDraw(s *soil.Soil, budget int) {
	ids := make([]string, 0, len(s.Items))
	for _, it := range s.Items {
		ids = append(ids, it.ID)
	}
	query, _ := redact.FromEnv().Redact(s.Query)
	err := draws.Append(draws.Draw{
		At: time.Now().UTC(), Bed: s.Bed, Query: query, Budget: budget,
		Tokens: s.Tokens, Considered: s.Considered, Entries: ids,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "hugel: draw not recorded: %v\n", err)
	}
}

// openIndex builds a searchable view of the pile. It is rebuilt per invocation
// on purpose: the files are the source of truth and the index is disposable, so
// there is never a stale one to invalidate.
func openIndex(pileDir string) (*soil.Index, error) {
	dir := pileDir
	if dir == "" {
		var err error
		if dir, err = pile.DefaultRoot(); err != nil {
			return nil, err
		}
	}
	store, err := pile.Open(dir)
	if err != nil {
		return nil, err
	}
	entries, err := store.All()
	if err != nil {
		return nil, err
	}
	return soil.Build(entries, time.Now()), nil
}
