package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charris/hugel/internal/gate"
	"github.com/charris/hugel/internal/tender"
)

func runGate(args []string) error {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel gate — decide whether a tender's work lands

usage:
  hugel gate <bead> [--into main] [--remote origin] [--test "make test"]
  hugel gate <bead> --dry-run     everything except merging, pushing and closing

Work, test, review, test, commit. The second test runs on the branch merged with
whatever the base has become, which is the only test whose result is true of what
gets pushed.

The reviewer is a separate agent run with a different prompt: the agent that
wrote the code has every reason to approve it. A review that states no verdict
is a refusal, not a pass.

Any stage can refuse, and a refusal leaves the worktree exactly where it is.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		into    = fs.String("into", "main", "branch the work lands on")
		remote  = fs.String("remote", "origin", "remote to push to; empty pushes nothing")
		test    = fs.String("test", "", "test command (default: discovered from the project)")
		wait    = fs.Duration("wait", 20*time.Minute, "how long to give the reviewer")
		dry     = fs.Bool("dry-run", false, "stop before anything irreversible")
		ask     = fs.Bool("ask-permission", false, "let the reviewer stop and ask")
		verbose = fs.Bool("v", false, "print each stage's output")
	)
	rest, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("need a bead to gate")
	}
	t, err := tender.Load(rest[0])
	if err != nil {
		return err
	}

	o := gate.Options{
		Tender: *t, Test: *test, Into: *into, Remote: *remote,
		Wait: *wait, SkipPermissions: !*ask,
		Log: func(s string) { fmt.Fprintf(os.Stderr, "  %s\n", s) },
	}
	// A dry run still tests and reviews, which is where a gate earns its keep.
	// What it skips is only the part that cannot be undone.
	o.DryRun = *dry

	fmt.Printf("gating %s  %s\n", t.Bead, truncate(t.Title, 56))
	rep, err := gate.Run(o)
	if err != nil {
		return err
	}
	printGate(rep, *verbose)
	if !rep.Passed {
		return fmt.Errorf("%s did not pass the gate", rep.Bead)
	}
	return nil
}

func printGate(rep gate.Report, verbose bool) {
	fmt.Println()
	for _, s := range rep.Stages {
		mark := "✗"
		if s.OK {
			mark = "✓"
		}
		took := ""
		if s.Took > time.Second {
			took = fmt.Sprintf(" (%s)", short(s.Took))
		}
		fmt.Printf("  %s %-8s %s%s\n", mark, s.Stage, firstLineOf(s.Detail), took)
		if verbose && strings.Contains(s.Detail, "\n") {
			for _, l := range strings.Split(s.Detail, "\n") {
				fmt.Printf("        %s\n", l)
			}
		}
	}
	fmt.Println()
	if rep.Passed {
		fmt.Printf("%s: %s\n", rep.Bead, rep.Why)
		return
	}
	fmt.Printf("%s stopped at %s: %s\n", rep.Bead, rep.Reached, rep.Why)
	fmt.Println("the worktree is untouched; look at it before starting anything else")
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return truncate(strings.TrimSpace(s[:i]), 60) + " …"
	}
	return truncate(strings.TrimSpace(s), 66)
}
