package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/charris/hugel/internal/config"
)

const hooksUsage = `hugel hooks — what the harness runs on hugel's behalf

usage:
  hugel hooks session-start    compost what has changed, before work begins

Wire it into Claude Code's SessionStart hook. It writes nothing to stdout: a
SessionStart hook's output is prepended to the session's context, and a
composting report re-read on every turn is exactly the waste this measures.
`

func runHooks(args []string) error {
	if len(args) == 0 {
		fmt.Print(hooksUsage)
		return nil
	}
	switch args[0] {
	case "session-start":
		return runSessionStart()
	case "-h", "--help", "help":
		fmt.Print(hooksUsage)
		return nil
	default:
		fmt.Fprint(os.Stderr, hooksUsage)
		return fmt.Errorf("unknown hook %q", args[0])
	}
}

// runSessionStart composts on the way in rather than on the way out.
//
// The moment pile freshness is worth anything is immediately before a draw, so
// collecting at the start of a session means soil pulled during it already
// contains the last session's lessons. It is also self-healing: a session that
// ends in a crash, a reboot or a kill still gets collected by the next start,
// where an end-of-session hook would lose it silently.
//
// It never fails. A hook that can refuse to let work begin is a worse problem
// than a stale pile, so every error is swallowed to the log and the exit status
// stays zero.
func runSessionStart() error {
	io.Copy(io.Discard, os.Stdin) // the harness sends JSON; none of it is needed

	started := time.Now()
	err := compostSessions(compostOpts{all: true, onlyNew: true, quiet: true})
	note := "ok"
	if err != nil {
		note = err.Error()
	}
	logHook("session-start", time.Since(started), note)
	return nil
}

// logHook keeps a hook's history where a person can read it. Hooks run
// unattended and silently by design, which is precisely why they need somewhere
// to say what happened.
func logHook(name string, took time.Duration, note string) {
	home, err := config.Home()
	if err != nil {
		return
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(home, "hooks.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %-14s %6dms  %s\n",
		time.Now().Format(time.RFC3339), name, took.Milliseconds(), note)
}
