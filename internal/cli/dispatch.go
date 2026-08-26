package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charris/hugel/internal/beads"
	"github.com/charris/hugel/internal/config"
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

	if err := reapDead(); err != nil {
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

	queue := beads.Queue(work, *bed, tender.Exists)

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

// reapDead finds tenders whose agent stopped without writing a result and puts
// their beads back in the queue.
//
// The worktree is kept. A tender that died is the most informative thing in the
// garden until somebody has read it, and the bead being available again does
// not require throwing away the evidence of why it failed the first time.
func reapDead() error {
	dead, err := tender.Stopped()
	if err != nil {
		return err
	}
	for _, t := range dead {
		if released(t) {
			continue
		}
		if err := beads.Release(t.Repo, t.Bead); err != nil {
			fmt.Fprintf(os.Stderr, "hugel: %s died; could not release it: %v\n", t.Bead, err)
			continue
		}
		if err := markReleased(t); err != nil {
			fmt.Fprintf(os.Stderr, "hugel: %v\n", err)
		}
		fmt.Printf("released  %-16s died without a result; worktree kept at %s\n",
			t.Bead, t.Worktree)
	}
	return nil
}

// A released tender is marked so that repeated dispatches do not keep releasing
// the same bead and printing the same line forever.
func releasedPath(t tender.Tender) string {
	return filepath.Join(filepath.Dir(t.Worktree), "released")
}

func released(t tender.Tender) bool {
	_, err := os.Stat(releasedPath(t))
	return err == nil
}

func markReleased(t tender.Tender) error {
	return os.WriteFile(releasedPath(t), []byte("released after dying without a result\n"), 0o644)
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
