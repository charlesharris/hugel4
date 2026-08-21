package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charris/hugel/internal/compost"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/redact"
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
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *session == "" && !*all {
		fs.Usage()
		return fmt.Errorf("need --session or --all")
	}

	dir := *root
	if dir == "" {
		var err error
		if dir, err = transcript.DefaultRoot(); err != nil {
			return err
		}
	}
	sessions, _, err := transcript.LoadAll(dir)
	if err != nil {
		return err
	}

	var chosen []*transcript.Session
	for _, s := range sessions {
		if *bed != "" && s.Bed != *bed {
			continue
		}
		if *session != "" && !strings.HasPrefix(s.ID, *session) {
			continue
		}
		chosen = append(chosen, s)
	}
	if len(chosen) == 0 {
		return fmt.Errorf("no sessions matched")
	}

	var store *pile.Store
	if !*dry {
		pd := *pileDir
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
		redacted  int
		cost      float64
		anyChange bool
	)
	fmt.Printf("%-10s %-16s %7s %8s  %s\n", "SESSION", "BED", "RECORDS", "ENTRIES", "")
	fmt.Println(strings.Repeat("─", 62))

	for _, s := range chosen {
		d := compost.Distil(s, budget)
		redacted += redact.Total(d.Redact(scrub))
		h, err := ex.Extract(d)
		if err != nil {
			return fmt.Errorf("extract %s: %w", s.ID, err)
		}
		cost += h.CostUSD
		proposed += len(h.Entries)

		var wrote []string
		for _, e := range h.Entries {
			if *dry {
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
		fmt.Printf("%-10s %-16s %7d %8d\n",
			truncate(s.ID, 10), truncate(s.Bed, 16), len(d.Records), len(h.Entries))
	}

	fmt.Println(strings.Repeat("─", 62))
	for _, k := range []pile.Result{"would write", pile.Created, pile.Updated, pile.Unchanged} {
		if counts[k] > 0 {
			fmt.Printf("%9d %s\n", counts[k], k)
		}
	}
	fmt.Printf("\nextractor %s/%s proposed %d entries for %s\n",
		ex.Name(), ex.Version(), proposed, money(cost))
	if redacted > 0 {
		fmt.Printf("redacted %d credentials before extraction\n", redacted)
	}
	if *dry || store == nil {
		return nil
	}
	if anyChange {
		if err := store.Commit(fmt.Sprintf("Compost %d sessions", len(chosen))); err != nil {
			return err
		}
	}
	n, _ := store.Count()
	fmt.Printf("pile now holds %d entries\n", n)
	return nil
}
