// Package cli wires hugel's commands. Commands stay thin: they parse flags,
// call into internal packages, and render. No domain logic lives here.
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const usage = `hugel — tend the garden

usage:
  hugel yield [flags]     what sessions cost, and how much of it was context
  hugel digest [flags]    distil a session into compostable material
  hugel compost [flags]   turn spent sessions into pile entries
  hugel pile <cmd>        the knowledge store
  hugel soil <query>      draw context from the pile
  hugel bed <cmd>         the projects the garden knows
  hugel tend              the working surface: judge what the garden did
  hugel hooks <name>      what the harness runs on hugel's behalf

run "hugel <command> -h" for flags.
`

// Run dispatches a command line.
func Run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	switch args[0] {
	case "yield":
		return runYield(args[1:])
	case "digest":
		return runDigest(args[1:])
	case "compost":
		return runCompost(args[1:])
	case "pile":
		return runPile(args[1:])
	case "soil":
		return runSoil(args[1:])
	case "bed":
		return runBed(args[1:])
	case "tend":
		return runTend(args[1:])
	case "hooks":
		return runHooks(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// parseSince accepts Go durations plus a "d" (day) and "w" (week) suffix,
// which are the units a gardener actually thinks in.
func parseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if n, ok := strings.CutSuffix(s, "d"); ok {
		v, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, fmt.Errorf("bad duration %q: %w", s, err)
		}
		return time.Duration(v * float64(24*time.Hour)), nil
	}
	if n, ok := strings.CutSuffix(s, "w"); ok {
		v, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, fmt.Errorf("bad duration %q: %w", s, err)
		}
		return time.Duration(v * float64(7*24*time.Hour)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad duration %q: %w", s, err)
	}
	return d, nil
}

// --- rendering helpers -----------------------------------------------------

func money(v float64) string {
	if v == 0 {
		return "-"
	}
	if v < 0.01 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func tokens(n int) string {
	switch {
	case n == 0:
		return "-"
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	default:
		return strconv.Itoa(n)
	}
}

func pct(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }

func dur(d time.Duration) string {
	switch {
	case d <= 0:
		return "-"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// bar draws a proportional meter, used to make a growing context legible.
func bar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	n := int(frac*float64(width) + 0.5)
	return strings.Repeat("█", n) + strings.Repeat("·", width-n)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// parseInterleaved lets flags appear after positional arguments, so
// `hugel soil "a question" --bed x` works. Go's flag package stops at the
// first non-flag, which would silently fold "--bed x" into the question.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// currentBed names the project the gardener is standing in.
//
// A bed is the basename of the directory a session ran in, which is how
// transcripts name them, so the same rule read from the shell gives the same
// answer. It is used where a bed *weights* -- soil's ranking, tend's ordering
// -- and deliberately not where a bed *filters*: defaulting a filter to the
// working directory would let a command run from the wrong place silently hide
// everything rather than show nothing.
func currentBed() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Base(wd)
}
