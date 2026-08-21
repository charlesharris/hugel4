package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charris/hugel/internal/config"
	"github.com/charris/hugel/internal/pile"
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
		bed     = fs.String("bed", "", "the bed the work is in (weights local knowledge)")
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

	ix, err := openIndex(*pileDir)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s := ix.Draw(soil.Query{
		Text: strings.Join(words, " "), Bed: *bed, Kin: cfg.KinOf(*bed),
		Type: pile.Type(*kind), Limit: *limit, Budget: *budget,
	})
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	fmt.Print(s.Render())
	return nil
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
