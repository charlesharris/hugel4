package tender

import (
	"testing"
	"time"

	"github.com/charris/hugel/internal/transcript"
)

func at(min int) time.Time {
	return time.Date(2026, 8, 26, 12, min, 0, 0, time.UTC)
}

// The point of the reading is that a tender can be watched without being
// trusted to report on itself: everything here comes from the transcript the
// harness writes, so a tender that has wandered off cannot say otherwise.
func TestProgressReadsWhatATenderHasBeenDoing(t *testing.T) {
	td := Tender{Bead: "x-1", Worktree: "/garden/x-1/work"}
	sessions := []*transcript.Session{
		{CWD: "/garden/x-1/work",
			Requests: make([]transcript.Request, 3),
			Tools: []transcript.ToolUse{
				{At: at(1), Name: "Read", Target: "internal/tender/start.go"},
				{At: at(2), Name: "Edit", Target: "internal/tender/start.go"},
				{At: at(3), Name: "Edit", Target: "internal/tender/tender.go"},
				{At: at(4), Name: "Bash", Target: "go test ./...", Errored: true},
			}},
		// Another tender's session, in another worktree. Its work is not this
		// tender's, and counting it would flatter or damn the wrong bead.
		{CWD: "/garden/other/work",
			Requests: make([]transcript.Request, 9),
			Tools:    []transcript.ToolUse{{At: at(9), Name: "Edit", Target: "elsewhere.go"}}},
	}

	p, ok := ProgressOf(td, sessions)
	if !ok {
		t.Fatal("no progress found for a tender whose session is right there")
	}
	if p.Turns != 3 {
		t.Errorf("turns = %d, want 3 -- the other worktree's session leaked in", p.Turns)
	}
	if p.Files != 2 {
		t.Errorf("files = %d, want 2 distinct files edited", p.Files)
	}
	if p.Trouble != 1 {
		t.Errorf("trouble = %d, want the failing command counted", p.Trouble)
	}
	if p.Doing != "run go test ./..." {
		t.Errorf("doing = %q, want the last thing it did", p.Doing)
	}
	if !p.Last.Equal(at(4)) {
		t.Errorf("last = %v, want the most recent activity", p.Last)
	}
}

// A tender's own account of what it is doing beats an inventory of its tool
// calls, when it is the more recent of the two.
func TestTheTendersOwnWordsWinWhenTheyAreLatest(t *testing.T) {
	td := Tender{Bead: "x-1", Worktree: "/w"}
	sessions := []*transcript.Session{{
		CWD:      "/w",
		Requests: make([]transcript.Request, 1),
		Tools:    []transcript.ToolUse{{At: at(1), Name: "Edit", Target: "config.go"}},
		Notes:    []transcript.Note{{At: at(2), Text: "The pooler needs session mode. Transaction mode drops LISTEN."}},
	}}
	p, _ := ProgressOf(td, sessions)
	if p.Doing != "The pooler needs session mode" {
		t.Errorf("doing = %q, want the tender's own account", p.Doing)
	}
	// An older note must not outrank a newer tool call, or a tender that said
	// something once would look like it was still saying it.
	sessions[0].Notes[0].At = at(0)
	p, _ = ProgressOf(td, sessions)
	if p.Doing != "edit config.go" {
		t.Errorf("doing = %q, want the newer tool call", p.Doing)
	}
}

// A tender nobody has a transcript for is reported as unseen rather than as
// idle: those are different, and only one of them is alarming.
func TestATenderWithNoSessionIsNotReportedAsIdle(t *testing.T) {
	p, ok := ProgressOf(Tender{Worktree: "/nowhere"}, []*transcript.Session{{CWD: "/w"}})
	if ok {
		t.Error("found progress for a tender with no session")
	}
	if p.Idle() != 0 {
		t.Errorf("idle = %v on a tender never seen, want zero", p.Idle())
	}
}

// A tender's last command is read at a glance, so the glance has to land on
// what it was doing rather than on where it already was.
func TestTheLastCommandIsReadableAtAGlance(t *testing.T) {
	cases := map[string]string{
		"cd /Users/charris/src/hugel4; go test ./...": "run go test ./...",
		"go test ./...":                   "run go test ./...",
		"cd /somewhere && make check":     "run make check",
		"python3 - <<'PY'\nimport os\nPY": "run python3 - <<'PY'",
	}
	for cmd, want := range cases {
		got := describe(transcript.ToolUse{Name: "Bash", Target: cmd})
		if got != want {
			t.Errorf("describe(%q) = %q, want %q", cmd, got, want)
		}
	}
}
