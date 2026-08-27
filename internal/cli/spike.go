package cli

import (
	"flag"
	"fmt"
	"os"
)

// runSpike sends an agent to find something out.
//
// Its own command rather than a flag on tender because the two ask for
// different things and answer differently: a tender is judged on its diff and
// goes through the gate, a spike leaves no diff and never does. Sharing the
// verb would put the difference in a flag a gardener has to remember, and put
// a spike in the queue of things waiting to be reviewed.
func runSpike(args []string) error {
	fs := flag.NewFlagSet("spike", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `hugel spike — find something out before anyone builds on it

usage:
  hugel spike <bead>         explore a question, unattended, in tmux
  hugel spike --attach <bead>  the same, with somebody in the pane

A spike is a tender with different rules. It runs the same way — a worktree, a
detached tmux session, a brief it reads and a result it writes — but its
product is knowledge rather than a diff. It records each finding with
bd remember as it goes, writes no code, commits nothing, and closes nothing.

The worktree is thrown away. What survives is what it recorded: those entries
compost into the pile as discoveries, where the next agent to ask a related
question will draw them as soil.

Use hugel tender --list, --show and --stop to look in on one.

flags:
`)
		fs.PrintDefaults()
	}
	var (
		attach = fs.Bool("attach", false, "a person will sit in the pane; the brief tells it to ask rather than guess")
		ask    = fs.Bool("ask-permission", false, "let the agent stop and ask; unattended, it will simply park")
		extra  = fs.String("note", "", "anything else this spike should know")
		root   = fs.String("root", "", "transcript root (default ~/.claude/projects)")
		soil   = fs.Int("soil", 1200, "tokens of soil to put in the brief; 0 for none")
	)
	rest, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("need a bead to spike")
	}
	return startTender(startOptions{
		Bead: rest[0], Root: *root, Extra: *extra,
		SkipPermissions: !*ask, Attach: *attach, Spike: true, Budget: *soil,
	})
}
