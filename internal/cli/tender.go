package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/config"
	"github.com/charris/hugel/internal/soil"
	"github.com/charris/hugel/internal/tender"
	"github.com/charris/hugel/internal/transcript"
)

func runTender(args []string) error {
	fs := flag.NewFlagSet("tender", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel tender — work one bead, unattended

usage:
  hugel tender <bead>        start a tender on a ready bead
  hugel tender --list        what is being tended, and how it went
  hugel tender --show <bead> the brief, and the result if there is one
  hugel tender --stop <bead> end a tender; keeps the worktree to look at

A tender is an ordinary agent session running detached in tmux, in a git
worktree of its own. It is briefed by a file it is told to read and answers by
writing a file back — nothing types into its pane, so what was asked and what
happened are both on disk afterwards.

It commits to its own branch and stops there. Pushing, merging and closing the
bead are a reviewer's decisions, not a tender's.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		list   = fs.Bool("list", false, "what is being tended")
		show   = fs.String("show", "", "print a tender's brief and result")
		stop   = fs.String("stop", "", "end a tender")
		clean  = fs.Bool("clean", false, "with --stop, remove the worktree too")
		ask    = fs.Bool("ask-permission", false, "let the agent stop and ask; a tender nobody is watching will simply park")
		attach = fs.Bool("attach", false, "a person will sit in the pane; the brief tells it to ask rather than guess")
		extra  = fs.String("note", "", "anything else this tender should know")
		root   = fs.String("root", "", "transcript root (default ~/.claude/projects)")
		soil   = fs.Int("soil", 1200, "tokens of soil to put in the brief; 0 for none")
	)
	rest, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}

	switch {
	case *list:
		return listTenders()
	case *show != "":
		return showTender(*show)
	case *stop != "":
		return stopTender(*stop, *clean)
	}
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("need a bead to tend")
	}
	return startTender(startOptions{
		Bead: rest[0], Root: *root, Extra: *extra,
		SkipPermissions: !*ask, Attach: *attach, Budget: *soil,
	})
}

// startOptions is what the gardener chose, before hugel has found the bead.
//
// A struct rather than a parameter list because spike and tender differ by two
// booleans out of six: positionally they would be one transposition away from
// briefing an agent as the wrong kind of thing, and the compiler would not say.
type startOptions struct {
	Bead            string
	Root            string
	Extra           string
	SkipPermissions bool
	Attach          bool
	Spike           bool
	Budget          int
}

func startTender(o startOptions) error {
	bead := o.Bead
	dir := o.Root
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

	// Find the bead by asking every bed that tracks work. A bead id carries its
	// prefix, so the bed it belongs to is discoverable rather than something the
	// gardener has to name.
	work, problems := beads.Survey(transcript.BedDirs(sessions))
	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "hugel: %v\n", p)
	}
	var (
		found *beads.Bead
		bed   *beads.Work
	)
	for _, w := range work {
		for i, b := range w.Beads {
			if b.ID == bead || strings.HasPrefix(b.ID, bead) {
				if found != nil {
					return fmt.Errorf("%q matches more than one bead", bead)
				}
				found, bed = &w.Beads[i], w
			}
		}
	}
	if found == nil {
		return fmt.Errorf("no open bead matching %q", bead)
	}
	if !found.Ready && found.Status != "in_progress" {
		return fmt.Errorf("%s is blocked; a tender cannot start work that cannot start", found.ID)
	}

	t, err := tender.Start(tender.Options{
		Bead: *found, Bed: bed.Bed, Repo: bed.Dir,
		SkipPermissions: o.SkipPermissions, Extra: o.Extra,
		Spike: o.Spike, Attach: o.Attach,
		Soil: soilFor(*found, bed.Bed, o.Budget),
	})
	if err != nil {
		return err
	}
	// Claiming goes through bd rather than into a status hugel keeps, so the
	// queue a tender pulls from stays the queue everything else reads.
	if err := beads.Claim(bed.Dir, found.ID); err != nil {
		fmt.Fprintf(os.Stderr, "hugel: %s started but not claimed in bd: %v\n", found.ID, err)
	}

	verb := "tending"
	if o.Spike {
		verb = "spiking"
	}
	fmt.Printf("%s %s  %s\n", verb, t.Bead, truncate(t.Title, 58))
	fmt.Printf("  worktree  %s\n", t.Worktree)
	// A spike's branch is where it is standing, not where its work will arrive:
	// it commits nothing, so printing the branch would promise a diff to review.
	if !o.Spike {
		fmt.Printf("  branch    %s\n", t.Branch)
	}
	fmt.Printf("  brief     %s\n", t.BriefPath())
	fmt.Printf("  watch     %s\n", t.Attach())
	if !o.SkipPermissions && !o.Attach {
		fmt.Println("\nrunning with permission prompts on: if it asks, nobody is there to answer.")
	}
	return nil
}

func listTenders() error {
	all, err := tender.List()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Println("nothing has been tended yet")
		return nil
	}
	fmt.Printf("%-18s %-10s %8s  %-16s %s\n", "BEAD", "STATE", "ELAPSED", "BED", "TITLE")
	fmt.Println(strings.Repeat("─", 92))
	for _, t := range all {
		fmt.Printf("%-18s %-10s %8s  %-16s %s\n",
			truncate(t.Bead, 18), t.State(), short(time.Since(t.Started)),
			truncate(t.Bed, 16), truncate(t.Title, 34))
	}
	fmt.Println(strings.Repeat("─", 92))
	fmt.Println("hugel tender --show <bead> for the brief and result")
	return nil
}

func showTender(bead string) error {
	t, err := tender.Load(bead)
	if err != nil {
		return err
	}
	fmt.Printf("%s  %s\n%s\n\n", t.Bead, t.State(), t.Attach())
	if b, err := os.ReadFile(t.BriefPath()); err == nil {
		fmt.Printf("─── brief ───\n%s\n", strings.TrimSpace(string(b)))
	}
	b, err := os.ReadFile(t.ResultPath())
	if err != nil {
		fmt.Println("\n─── result ───\nnot written yet")
		return nil
	}
	fmt.Printf("\n─── result ───\n%s\n", strings.TrimSpace(string(b)))
	return nil
}

func stopTender(bead string, clean bool) error {
	t, err := tender.Load(bead)
	if err != nil {
		return err
	}
	if err := tender.Stop(*t, clean); err != nil {
		return err
	}
	if clean {
		fmt.Printf("stopped %s and removed its worktree\n", t.Bead)
		return nil
	}
	fmt.Printf("stopped %s; its worktree is still at %s\n", t.Bead, t.Worktree)
	return nil
}

// short renders a duration the way a person says one.
func short(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// soilFor draws what the pile knows about a bead.
//
// The bead is the query: its title and description are already a statement of
// what the work is about, written by a person, which is exactly what soil wants
// and what nobody has to compose. A failed draw costs the tender nothing but
// the soil -- briefing an agent without prior knowledge is the old behaviour,
// not a broken one.
func soilFor(b beads.Bead, bed string, budget int) string {
	if budget <= 0 {
		return ""
	}
	ix, err := openIndex("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "hugel: no soil for this tender: %v\n", err)
		return ""
	}
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	drawn := ix.Draw(soil.Query{
		Text: b.Title + " " + b.Body, Bed: bed, Kin: cfg.KinOf(bed), Budget: budget,
	})
	if len(drawn.Items) == 0 {
		return ""
	}
	// The whole bead scores the draw; the title labels it. A description is the
	// better query -- more words, written by a person -- and the worst possible
	// heading, since the rendered header quotes it back in full at the top of
	// the brief the tender is about to read.
	drawn.Query = b.Title
	recordDraw(drawn, budget)
	return drawn.Render()
}
