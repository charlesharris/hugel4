package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/config"
	"github.com/charris/hugel/internal/events"
	"github.com/charris/hugel/internal/tender"
	"github.com/charris/hugel/internal/transcript"
)

func runDispatch(args []string) error {
	fs := flag.NewFlagSet("dispatch", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel dispatch — fill the tender slots from the ready queue

usage:
  hugel dispatch [--slots 2] [--bed NAME] [--dry-run]

Counts the tenders still working, and starts more on the highest-priority ready
beads until the slots are full. Then it returns.

It is not a daemon on purpose. A command that fills the slots and exits can be
run from anything that already knows how to repeat -- a shell loop, a timer, a
keystroke -- and can be reasoned about by reading it once. Nothing here has to
survive a crash, because there is nothing running to crash.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		slots = fs.Int("slots", 2, "how many tenders may be working at once")
		bed   = fs.String("bed", "", "only dispatch work in this bed")
		dry   = fs.Bool("dry-run", false, "say what would be started")
		soil  = fs.Int("soil", 1200, "tokens of soil in each brief")
		ask   = fs.Bool("ask-permission", false, "let tenders stop and ask")
		root  = fs.String("root", "", "transcript root (default ~/.claude/projects)")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	unlock, err := lockGarden("dispatch")
	if err != nil {
		return err
	}
	defer unlock()

	if err := reap(); err != nil {
		return err
	}

	live, err := tender.Live()
	if err != nil {
		return err
	}
	free := *slots - len(live)
	for _, t := range live {
		fmt.Printf("working   %-16s %s\n", t.Bead, truncate(t.Title, 52))
	}
	if free <= 0 {
		fmt.Printf("\nall %d slots are full\n", *slots)
		return nil
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
	work, problems := beads.Survey(transcript.BedDirs(sessions))
	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "hugel: %v\n", p)
	}

	// A bead waiting on a person is not available to a tender, however ready bd
	// says it is. That is the mark standing, and clearing the label is what
	// puts the bead back in the queue.
	queue := beads.Queue(work, *bed, waiting)

	started := 0
	for _, c := range queue {
		if started >= free {
			break
		}
		if *dry {
			fmt.Printf("would start %-16s %s\n", c.Bead.ID, truncate(c.Bead.Title, 52))
			started++
			continue
		}
		t, err := tender.Start(tender.Options{
			Bead: c.Bead, Bed: c.Work.Bed, Repo: c.Work.Dir,
			SkipPermissions: !*ask, Soil: soilFor(c.Bead, c.Work.Bed, *soil),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "hugel: %s not started: %v\n", c.Bead.ID, err)
			continue
		}
		if err := beads.Claim(c.Work.Dir, c.Bead.ID); err != nil {
			fmt.Fprintf(os.Stderr, "hugel: %s started but not claimed: %v\n", c.Bead.ID, err)
		}
		fmt.Printf("started   %-16s %s\n", t.Bead, truncate(t.Title, 52))
		started++
	}

	fmt.Println()
	verb := "started"
	if *dry {
		verb = "would start"
	}
	switch {
	case started == 0 && len(queue) == 0:
		fmt.Printf("nothing ready to start (%d of %d slots working)\n", len(live), *slots)
	case started == 0:
		fmt.Printf("%s nothing (%d of %d slots working)\n", verb, len(live), *slots)
	case *dry:
		fmt.Printf("would start %d, filling %d of %d slots\n", started, len(live)+started, *slots)
	default:
		fmt.Printf("%d started, %d of %d slots working\n", started, len(live)+started, *slots)
	}
	return nil
}

// reap acts on tenders that will not finish on their own.
//
// Two shapes, one consequence. A tender whose agent died left a bead claimed
// and a slot spent, and nothing else in the system is watching for it: tmux
// does not know what the session was for and bd does not know its claimant
// stopped existing. A tender that finished and said it was blocked left
// something more useful -- a reason the spec is wrong -- and today that reason
// goes nowhere while the same spec waits to be handed to the next agent.
//
// Both end with the bead back in a person's hands, carrying what was learned.
func reap() error {
	all, err := tender.List()
	if err != nil {
		return err
	}
	for _, t := range all {
		if t.Handled() {
			continue
		}
		switch {
		case t.State() == "stopped":
			handBack(t, "The tender working this died without writing a result. "+
				"Its worktree is at "+t.Worktree, "died")
		case t.State() == "finished" && t.Outcome() != "done":
			// A blocked or partial outcome is a finding about the bead, not a
			// failure of the tender. The bead is what has to change.
			// The reason already leads with the outcome word, so naming it
			// again reads as a stutter on the bead a person will be reading.
			handBack(t, "A tender stopped short. "+t.Reason()+
				" Its worktree is at "+t.Worktree, t.Outcome())
		}
	}
	return nil
}

func handBack(t tender.Tender, note, why string) {
	// How a tender's life ended, recorded where the rest of it is. Between this
	// and gate.run, every exit a tender has is on the event log.
	events.Emit(events.Event{
		Name: "tender.handback", Bead: t.Bead, Bed: t.Bed, Outcome: why,
		Duration: time.Since(t.Started),
		Fields: events.F{
			"reason": t.Reason(), "worktree": t.Worktree, "branch": t.Branch,
		},
	})
	if err := beads.HandBack(t.Repo, t.Bead, note); err != nil {
		fmt.Fprintf(os.Stderr, "hugel: %s needs attention but bd could not be told: %v\n", t.Bead, err)
		return
	}
	if err := t.MarkHandled(why); err != nil {
		fmt.Fprintf(os.Stderr, "hugel: %v\n", err)
	}
	fmt.Printf("handed back %-16s %s; worktree kept at %s\n", t.Bead, why, t.Worktree)
}

// lockGarden stops two dispatches racing for the same slot. An exclusive create
// is the whole mechanism: if the file is there, somebody else is dispatching.
func lockGarden(name string) (func(), error) {
	home, err := config.Home()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(home, name+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			b, _ := os.ReadFile(path)
			return nil, fmt.Errorf("another %s is running (%s); remove %s if it is not",
				name, strings.TrimSpace(string(b)), path)
		}
		return nil, err
	}
	fmt.Fprintf(f, "pid %d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(path) }, nil
}

// waiting reports beads a tender should leave alone: work a person owes
// something to, and work whose last tender has not been dealt with yet.
func waiting(id string) bool {
	t, err := tender.Load(id)
	if err != nil {
		return false // never tended
	}
	// A tender that has been acted on is finished business. The bead may be
	// tended again once whatever it was handed back for is resolved.
	return !t.Handled()
}
