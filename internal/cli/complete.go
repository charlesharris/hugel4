package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charris/hugel/internal/complete"
)

// runComplete emits candidates for a shell to offer.
//
// One line per candidate, "value:description", which is the format zsh's
// _describe reads directly. There is no flag parsing and no error output: a
// completer runs on every tab and must be silent, fast, and impossible to
// break by being asked something odd.
func runComplete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hugel complete <source>\nsources: %s",
			strings.Join(complete.Sources(), ", "))
	}
	cwd, _ := os.Getwd()
	cands := complete.For(complete.Source(args[0]), cwd)

	prefix := ""
	if len(args) > 1 {
		prefix = args[1]
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for _, c := range cands {
		if prefix != "" && !strings.HasPrefix(c.Value, prefix) {
			continue
		}
		fmt.Fprintln(out, c.Line())
	}
	return nil
}

func runCompletion(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stderr, `hugel completion — emit a shell completion script

usage:
  hugel completion zsh

Install it where zsh looks for completions, then restart the shell:

  mkdir -p ~/.local/share/zsh/site-functions
  hugel completion zsh > ~/.local/share/zsh/site-functions/_hugel

If that directory is not on $fpath, add this above compinit in ~/.zshrc:

  fpath=(~/.local/share/zsh/site-functions $fpath)

make install does all of this.
`)
		if len(args) == 0 {
			return fmt.Errorf("which shell?")
		}
		return nil
	}
	switch args[0] {
	case "zsh":
		fmt.Print(complete.Zsh())
		return nil
	default:
		return fmt.Errorf("no completion for %q; hugel speaks zsh", args[0])
	}
}
