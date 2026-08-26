package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charris/hugel/internal/events"
	"github.com/charris/hugel/internal/gate"
	"github.com/charris/hugel/internal/survival"
	"github.com/charris/hugel/internal/tender"
)

func runGate(args []string) error {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel gate — decide whether a tender's work lands

usage:
  hugel gate <bead> [--into main] [--remote origin] [--test "make test"]
  hugel gate <bead> --dry-run     everything except merging, pushing and closing
  hugel gate --grade              what became of the work it approved

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
		grade   = fs.Bool("grade", false, "report the survival rate of approved work")
		since   = fs.String("since", "90d", "with --grade: landings within this window")
		mature  = fs.String("mature", "7d", "with --grade: how long a landing must stand before it is judged")
		bed     = fs.String("bed", "", "with --grade: restrict to one bed")
		asJSON  = fs.Bool("json", false, "with --grade: emit JSON")
	)
	rest, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if *grade {
		return showSurvival(*since, *mature, *bed, *asJSON)
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

// showSurvival grades the gate on what became of the work it approved.
func showSurvival(since, mature, bed string, asJSON bool) error {
	window, err := parseSince(since)
	if err != nil {
		return err
	}
	hold, err := parseSince(mature)
	if err != nil {
		return err
	}
	log, err := events.Load()
	if err != nil {
		return err
	}
	now := time.Now()
	from := time.Time{}
	if window > 0 {
		from = now.Add(-window)
	}
	landings := survival.Landings(log, from)
	if bed != "" {
		var kept []survival.Landing
		for _, l := range landings {
			if strings.EqualFold(l.Bed, bed) {
				kept = append(kept, l)
			}
		}
		landings = kept
	}
	facts, unattributed := survival.Look(landings)
	rep := survival.Grade(landings, facts, now, hold)
	rep.Window, rep.Unattributed = window, unattributed

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	if len(rep.Verdicts) == 0 {
		fmt.Printf("the gate has approved nothing in the last %s\n", since)
		fmt.Println("\nsurvival is graded from gate.land events, so it can only speak for work")
		fmt.Println("that landed through the gate rather than by hand.")
		return nil
	}

	fmt.Printf("gate survival — landings in the last %s, judged after %s\n\n", since, mature)
	fmt.Printf("  approved and landed  %4d\n", len(rep.Verdicts))
	if rep.Young > 0 {
		fmt.Printf("  too young to judge   %4d\n", rep.Young)
	}
	fmt.Printf("  judged               %4d\n", rep.Judged())
	fmt.Printf("    held               %4d\n", rep.Held)
	if rep.Reverted > 0 {
		fmt.Printf("    reverted           %4d\n", rep.Reverted)
	}
	if rep.Reopened > 0 {
		fmt.Printf("    reopened           %4d\n", rep.Reopened)
	}
	fmt.Println()
	if rep.Measurable() {
		fmt.Printf("  survival rate  %s %s of %d judged\n",
			bar(rep.Rate(), 10), pct(rep.Rate()), rep.Judged())
	} else {
		fmt.Printf("  survival rate  nothing has stood long enough to judge\n")
	}

	var failed []survival.Verdict
	for _, v := range rep.Verdicts {
		if v.Fate == survival.Reverted || v.Fate == survival.Reopened {
			failed = append(failed, v)
		}
	}
	if len(failed) > 0 {
		fmt.Println()
		for _, v := range failed {
			when := ""
			if v.Fate == survival.Reverted {
				when = " after " + lasted(v.Age)
			}
			fmt.Printf("  %-18s %s%s  %s\n",
				truncate(v.Bead, 18), v.Fate, when, truncate(v.Why, 52))
		}
	}
	if rep.Unattributed > 0 {
		fmt.Printf("\n  %d revert(s) in these repositories name no commit the gate landed,\n", rep.Unattributed)
		fmt.Println("  and are counted nowhere in the rate above.")
	}
	fmt.Println("\n  reporting only: nothing here makes the reviewer stricter or gates a")
	fmt.Println("  file harder. It says what held, not what to do about it.")
	return nil
}

// lasted says how long approved work stood. Days rather than hours, because
// the question survival asks is measured in the time between landing and
// finding out, and 480h00m is a number nobody reads as twenty days.
func lasted(d time.Duration) string {
	if d < 24*time.Hour {
		return dur(d)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
