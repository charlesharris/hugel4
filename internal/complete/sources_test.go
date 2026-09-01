package complete

import (
	"strings"
	"testing"
)

// TestAnEmptyGardenCompletesToNothing, not to an error. A completer runs on
// every tab, in any directory, before anything has been set up.
func TestAnEmptyGardenCompletesToNothing(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	t.Setenv("HUGEL_PILE", t.TempDir())
	t.Setenv("HUGEL_TRANSCRIPT_ROOT", t.TempDir())
	for _, s := range []Source{Ready, Tended, Tenders, Live, Spikes, Entries, Beds, Sessions} {
		if got := For(s, t.TempDir()); len(got) != 0 {
			t.Errorf("%s returned %d candidates from an empty garden", s, len(got))
		}
	}
}

// TestAnUnknownSourceIsEmptyRatherThanFatal. The shell has no way to show an
// error, and a panic in a completer wedges the terminal.
func TestAnUnknownSourceIsEmptyRatherThanFatal(t *testing.T) {
	if got := For(Source("no-such-source"), t.TempDir()); got != nil {
		t.Errorf("unknown source returned %v", got)
	}
}

// TestStaticSourcesNeedNoGarden: types and hooks are facts about the binary.
func TestStaticSourcesNeedNoGarden(t *testing.T) {
	t.Setenv("HUGEL_HOME", t.TempDir())
	if got := For(Types, ""); len(got) != 5 {
		t.Errorf("want 5 pile types, got %d", len(got))
	}
	if got := For(Hooks, ""); len(got) != 1 || got[0].Value != "session-start" {
		t.Errorf("hooks completed to %v", got)
	}
}

// TestALineCannotBeSplitByItsOwnContent. A pile entry title is a sentence
// somebody wrote; a bead id comes from bd. Neither is hugel's to promise.
func TestALineCannotBeSplitByItsOwnContent(t *testing.T) {
	c := Candidate{Value: "odd:id", Desc: "a title\nwith a newline"}
	line := c.Line()
	if strings.Count(line, "\n") != 0 {
		t.Errorf("description broke the line: %q", line)
	}
	value, _, _ := strings.Cut(line, ":")
	if value != `odd\` {
		t.Errorf("colon in the value was not escaped: %q", line)
	}
}

// TestDynamicReportsWhoAnswers keeps the two file sources out of the set the
// binary claims to enumerate.
func TestDynamicReportsWhoAnswers(t *testing.T) {
	if None.Dynamic() || Dirs.Dynamic() {
		t.Error("a shell-side source should not be hugel's to answer")
	}
	if !Ready.Dynamic() {
		t.Error("the ready queue is hugel's to answer")
	}
}
