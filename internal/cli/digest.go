package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charris/hugel/internal/compost"
	"github.com/charris/hugel/internal/transcript"
)

func runDigest(args []string) error {
	fs := flag.NewFlagSet("digest", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel digest — distil a session into compostable material

Stage one of composting: mechanical, free, and bounded. Turns a session of any
size into a few thousand characters describing what was asked, what changed,
what ran, and what broke.

usage:
  hugel digest --session ID     distil one session
  hugel digest --all            distil every session, size report only

flags:
`)
		fs.PrintDefaults()
	}
	var (
		session = fs.String("session", "", "session id prefix")
		all     = fs.Bool("all", false, "report digest sizes across every session")
		asJSON  = fs.Bool("json", false, "emit the digest as JSON")
		root    = fs.String("root", "", "transcript root (default ~/.claude/projects)")
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

	budget := compost.DefaultBudget()

	if *all {
		fmt.Printf("%-10s %-16s %7s %9s %8s %7s  %s\n",
			"SESSION", "BED", "TOOLS", "CTX READ", "DIGEST", "RATIO", "")
		fmt.Println(strings.Repeat("─", 76))
		for _, s := range sessions {
			d := compost.Distil(s, budget)
			ctx := s.Usage().ContextRead()
			// Rough but honest: characters per token averages near four.
			ratio := 0.0
			if ctx > 0 {
				ratio = float64(d.Size()) / 4 / float64(ctx)
			}
			fmt.Printf("%-10s %-16s %7d %9s %8s %6.4f%%\n",
				truncate(s.ID, 10), truncate(s.Bed, 16), len(s.Tools),
				tokens(ctx), tokens(d.Size()/4)+"t", ratio*100)
		}
		return nil
	}

	var match *transcript.Session
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, *session) {
			if match != nil {
				return fmt.Errorf("session prefix %q is ambiguous", *session)
			}
			match = s
		}
	}
	if match == nil {
		return fmt.Errorf("no session matching %q", *session)
	}

	d := compost.Distil(match, budget)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	}
	fmt.Print(d.Render())
	fmt.Fprintf(os.Stderr, "\n[%s]\n", d)
	return nil
}
