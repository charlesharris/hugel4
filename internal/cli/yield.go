package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charris/hugel/internal/draws"
	"github.com/charris/hugel/internal/pile"
	"github.com/charris/hugel/internal/transcript"
	"github.com/charris/hugel/internal/yield"
)

func runYield(args []string) error {
	fs := flag.NewFlagSet("yield", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel yield — what sessions cost, and how much of it was context

usage:
  hugel yield [--since 30d] [--bed NAME]     spend rolled up by bed
  hugel yield --sessions                     one line per session, dearest first
  hugel yield --session ID                   request-by-request, to find a spiral
  hugel yield --soil                         whether the pile is asked, and whether it was right

flags:
`)
		fs.PrintDefaults()
	}
	var (
		since    = fs.String("since", "30d", "only sessions ending within this window (30d, 2w, 48h)")
		all      = fs.Bool("all", false, "no time limit")
		bed      = fs.String("bed", "", "restrict to one bed")
		sessions = fs.Bool("sessions", false, "list sessions instead of beds")
		session  = fs.String("session", "", "show one session in detail (id prefix is enough)")
		limit    = fs.Int("limit", 20, "rows to show in list views")
		soilRep  = fs.Bool("soil", false, "report draws from the pile rather than spend")
		asJSON   = fs.Bool("json", false, "emit JSON")
		root     = fs.String("root", "", "transcript root (default ~/.claude/projects)")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := *root
	if dir == "" {
		var err error
		if dir, err = transcript.DefaultRoot(); err != nil {
			return err
		}
	}

	all_, problems, err := transcript.LoadAll(dir)
	if err != nil {
		return err
	}
	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "hugel: skipped %v\n", p)
	}
	if len(all_) == 0 {
		fmt.Printf("no sessions with recorded usage under %s\n", dir)
		return nil
	}

	if *session != "" {
		return showSession(all_, *session, *limit)
	}

	f := yield.Filter{Bed: *bed}
	if !*all {
		d, err := parseSince(*since)
		if err != nil {
			return err
		}
		if d > 0 {
			f.Since = time.Now().Add(-d)
		}
	}
	if *soilRep {
		return showSoil(all_, f, *asJSON)
	}

	rep := yield.Build(all_, f)
	if len(rep.Entries) == 0 {
		fmt.Println("no sessions in that window")
		return nil
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	if *sessions {
		showSessions(rep, *limit)
		return nil
	}
	showBeds(rep, *limit)
	return nil
}

func showBeds(rep yield.Report, limit int) {
	fmt.Printf("%-20s %5s %6s %8s %9s %9s %8s  %s\n",
		"BED", "SESS", "TURNS", "OUTPUT", "CTX READ", "COST", "CTX TAX", "")
	fmt.Println(strings.Repeat("─", 84))
	for i, b := range rep.Beds {
		if limit > 0 && i >= limit {
			fmt.Printf("… %d more beds\n", len(rep.Beds)-limit)
			break
		}
		fmt.Printf("%-20s %5d %6d %8s %9s %9s %8s  %s\n",
			truncate(b.Name, 20), b.Sessions, b.Requests,
			tokens(b.Usage.Output), tokens(b.Usage.ContextRead()),
			money(b.Cost.Total()), pct(b.ContextTax()), bar(b.ContextTax(), 10))
	}
	fmt.Println(strings.Repeat("─", 84))
	totalRequests := 0
	for _, b := range rep.Beds {
		totalRequests += b.Requests
	}
	fmt.Printf("%-20s %5d %6d %8s %9s %9s %8s  %s\n",
		"TOTAL", len(rep.Entries), totalRequests, tokens(rep.Usage.Output),
		tokens(rep.Usage.ContextRead()), money(rep.Total()), pct(rep.ContextTax()), bar(rep.ContextTax(), 10))

	fmt.Println()
	fmt.Printf("  produced   %s output tokens for %s\n", tokens(rep.Usage.Output), money(rep.Cost.Output))
	fmt.Printf("  carried    %s context tokens for %s  (%s of spend)\n",
		tokens(rep.Usage.ContextRead()), money(rep.Cost.Context()), pct(rep.ContextTax()))
	if rep.Usage.CacheWrite() > 0 {
		fmt.Printf("  cached     %s written, %s read back\n",
			tokens(rep.Usage.CacheWrite()), tokens(rep.Usage.CacheRead))
	}
	if rep.Unpriced > 0 {
		fmt.Printf("\n  note: %d requests used a model with no known rate and are excluded from cost\n", rep.Unpriced)
	}
}

func showSessions(rep yield.Report, limit int) {
	fmt.Printf("%-8s %-16s %-12s %5s %6s %9s %8s %9s\n",
		"SESSION", "BED", "WHEN", "PRMT", "TURNS", "CTX READ", "CTX TAX", "COST")
	fmt.Println(strings.Repeat("─", 82))
	for i, e := range rep.Entries {
		if limit > 0 && i >= limit {
			fmt.Printf("… %d more sessions\n", len(rep.Entries)-limit)
			break
		}
		s := e.Session
		fmt.Printf("%-8s %-16s %-12s %5d %6d %9s %8s %9s\n",
			truncate(s.ID, 8), truncate(s.Bed, 16), s.Start.Format("Jan 02 15:04"),
			s.Prompts, len(s.Requests), tokens(e.Usage.ContextRead()),
			pct(e.ContextTax()), money(e.Total()))
	}
	fmt.Println(strings.Repeat("─", 82))
	fmt.Printf("%d sessions, %s\n", len(rep.Entries), money(rep.Total()))
}

func showSession(sessions []*transcript.Session, prefix string, limit int) error {
	var match *transcript.Session
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, prefix) {
			if match != nil {
				return fmt.Errorf("session prefix %q is ambiguous", prefix)
			}
			match = s
		}
	}
	if match == nil {
		return fmt.Errorf("no session matching %q", prefix)
	}
	e := yield.Price(match)

	fmt.Printf("session  %s\n", match.ID)
	fmt.Printf("bed      %s  (%s)\n", match.Bed, match.CWD)
	if match.Branch != "" {
		fmt.Printf("branch   %s\n", match.Branch)
	}
	fmt.Printf("when     %s  for %s\n", match.Start.Format("Mon Jan 02 15:04"), dur(match.Duration()))
	fmt.Printf("models   %s\n", strings.Join(e.Models, ", "))
	fmt.Printf("spend    %s   output %s · context %s (%s)\n",
		money(e.Total()), money(e.Cost.Output), money(e.Cost.Context()), pct(e.ContextTax()))
	if e.Session.Prompts > 0 {
		fmt.Printf("         %s over %d prompts = %s per prompt\n",
			money(e.Total()), e.Session.Prompts, money(e.CostPerPrompt()))
	}
	if sc := e.Sidechain.Total(); sc > 0 {
		fmt.Printf("sidechain %s (%s) ran in subagent contexts and never entered the main thread\n",
			money(sc), pct(sc/e.Total()))
	}

	// Per-request context growth. A healthy session climbs then plateaus; one
	// that keeps climbing to the end is hauling everything it ever read.
	peak := 0
	for _, r := range match.Requests {
		if v := r.Usage.ContextRead(); v > peak {
			peak = v
		}
	}
	// Long sessions are sampled rather than truncated: the point of this view
	// is the shape of the climb, and a head/tail cut hides it.
	stride := 1
	if limit > 0 && len(match.Requests) > limit {
		stride = (len(match.Requests) + limit - 1) / limit
	}
	if stride > 1 {
		fmt.Printf("\nshowing every %d%s request of %d\n", stride, ordinal(stride), len(match.Requests))
	}
	fmt.Printf("\n%4s %-8s %9s %8s %9s  %s\n", "#", "TIME", "CTX READ", "OUTPUT", "COST", "CONTEXT CARRIED")
	fmt.Println(strings.Repeat("─", 76))
	var running float64
	for i, r := range match.Requests {
		pc, _ := yield.PriceRequest(r)
		running += pc.Total()
		if i%stride != 0 && i != len(match.Requests)-1 {
			continue
		}
		frac := 0.0
		if peak > 0 {
			frac = float64(r.Usage.ContextRead()) / float64(peak)
		}
		tag := ""
		if r.Sidechain {
			tag = " ↳sub"
		}
		fmt.Printf("%4d %-8s %9s %8s %9s  %s%s\n",
			i+1, r.At.Format("15:04:05"), tokens(r.Usage.ContextRead()),
			tokens(r.Usage.Output), money(pc.Total()), bar(frac, 24), tag)
	}
	fmt.Println(strings.Repeat("─", 76))
	fmt.Printf("peak context %s · total %s\n", tokens(peak), money(running))
	return nil
}

func ordinal(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// showSoil reports what the pile was asked for and what came of it.
//
// The two numbers are reach and precision, and both are built to be able to
// come out badly. Reach is the miss rate of pull delivery stated forwards: a
// skill fires only when the agent recognises the moment, and if that number
// stays near zero the argument for pull was wrong. Precision counts only
// entries a human has ruled on, so it cannot be flattered by leaving the pile
// unreviewed.
func showSoil(sessions []*transcript.Session, f yield.Filter, asJSON bool) error {
	log, err := draws.Load()
	if err != nil {
		return err
	}
	dir, err := pile.DefaultRoot()
	if err != nil {
		return err
	}
	var entries []*pile.Entry
	if store, err := pile.Open(dir); err == nil {
		if entries, err = store.All(); err != nil {
			return err
		}
	}

	rep := yield.Soil(sessions, log, entries, f)
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	if rep.Draws == 0 {
		fmt.Printf("the pile was not asked anything in %d sessions\n", rep.Sessions)
		fmt.Println("\nnothing has been drawn yet, so there is nothing to judge.")
		return nil
	}

	fmt.Printf("%-20s %5s %6s %8s %9s  %s\n", "BED", "SESS", "DRAWS", "REACHED", "TOKENS", "REACH")
	fmt.Println(strings.Repeat("─", 72))
	for _, b := range rep.Beds {
		reach := 0.0
		if b.Sessions > 0 {
			reach = float64(b.Reached) / float64(b.Sessions)
		}
		fmt.Printf("%-20s %5d %6d %8d %9s  %5.0f%% %s\n",
			truncate(b.Name, 20), b.Sessions, b.Draws, b.Reached, tokens(b.Tokens),
			reach*100, bar(reach, 10))
	}
	fmt.Println(strings.Repeat("─", 72))
	fmt.Printf("%-20s %5d %6d %8d %9s  %5.0f%% %s\n",
		"TOTAL", rep.Sessions, rep.Draws, rep.Reached, tokens(rep.Tokens),
		rep.Reach()*100, bar(rep.Reach(), 10))

	fmt.Printf("\n  reached    %d of %d sessions asked the pile anything\n", rep.Reached, rep.Sessions)
	fmt.Printf("  delivered  %d entries over %d draws, %d of them distinct\n",
		rep.Delivered, rep.Draws, rep.Distinct)
	if rep.Judged() == 0 {
		fmt.Printf("  judged     none of the %d yet — precision is unknown, not good\n", rep.Distinct)
		return nil
	}
	fmt.Printf("  judged     %d of %d: %d kept, %d thrown out  (%.0f%% precision)\n",
		rep.Judged(), rep.Distinct, rep.Accepted, rep.Rejected, rep.Precision()*100)
	if rep.Missing > 0 {
		fmt.Printf("  gone       %d drawn entries are no longer in the pile\n", rep.Missing)
	}
	return nil
}
