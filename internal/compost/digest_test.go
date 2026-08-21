package compost

import (
	"strings"
	"testing"
	"time"

	"github.com/charris/hugel/internal/redact"
	"github.com/charris/hugel/internal/transcript"
)

// Real transcripts are dominated by shell noise that says where a command ran
// rather than what it did. Left in, it crowds the digest and makes forty
// distinct commands look like one.
func TestNormaliseCommand(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"bare command", "go test ./...", "go test ./..."},
		{"cd with &&", "cd /src/hellbox && git status", "git status"},
		{"cd with semicolon", "cd /src/hellbox; git status", "git status"},
		{"cd on its own line", "cd /src/hellbox\n\ngit status", "git status"},
		{"nested cd", "cd /a && cd b && make", "make"},
		{"bare cd is a no-op, not nothing", "cd /src/hellbox", "cd /src/hellbox"},
		{"env prefix", "BD_NON_INTERACTIVE=1 bd init", "bd init"},
		{"env and cd", "cd /src && FOO=1 make test", "make test"},
		{"pipeline keeps its head", "grep -rn x internal | head -30", "grep -rn x internal …"},
		{"heredoc body dropped", "cat > f.rb <<'RUBY'\nmodule X\nend\nRUBY", "cat > f.rb <<'RUBY'"},
		{"heredoc behind a cd", "cd /src && cat > f.rb <<'RUBY'\nmodule X\nRUBY", "cat > f.rb <<'RUBY'"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normaliseCommand(tt.in); got != tt.want {
				t.Errorf("normaliseCommand(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

// A path with a dot or slash before "=" is a filename, not an assignment.
func TestIsAssignment(t *testing.T) {
	for _, w := range []string{"FOO=1", "BD_NON_INTERACTIVE=1"} {
		if !isAssignment(w) {
			t.Errorf("isAssignment(%q) = false", w)
		}
	}
	for _, w := range []string{"go", "./x=y", "a/b=c", "=x"} {
		if isAssignment(w) {
			t.Errorf("isAssignment(%q) = true", w)
		}
	}
}

// Slash-command echoes and interrupt markers are the harness talking, not the
// gardener, and must not displace real prompts in the budget.
func TestHarnessNoise(t *testing.T) {
	noise := []string{
		"<command-name>/exit</command-name>",
		"<local-command-stdout>Bye!</local-command-stdout>",
		"[Request interrupted by user]",
		"   ",
	}
	for _, s := range noise {
		if !harnessNoise(s) {
			t.Errorf("harnessNoise(%q) = false", s)
		}
	}
	if harnessNoise("let's talk about the drive count") {
		t.Error("a real prompt was treated as noise")
	}
}

// The point of the digest is that its size does not track the session's. A
// twenty-hour session and a five-minute one must cost about the same to
// compost, or composting is not worth doing.
func TestDistilIsBounded(t *testing.T) {
	small := synthetic(5)
	huge := synthetic(2000)

	b := DefaultBudget()
	ds, dh := Distil(small, b), Distil(huge, b)

	if dh.Size() > 4*ds.Size()+8000 {
		t.Errorf("digest grew with the session: small=%d huge=%d", ds.Size(), dh.Size())
	}
	if dh.Size() > 30000 {
		t.Errorf("digest of a huge session = %d chars, want it bounded", dh.Size())
	}
	if dh.ToolCalls != 2000 {
		t.Errorf("ToolCalls = %d, want the true count even when the listing is capped", dh.ToolCalls)
	}
	if dh.Truncated.Commands == 0 {
		t.Error("truncation should be reported, not silent")
	}
}

// Prompts are taken from both ends: intent lives at the start of a session and
// outcome at the end, while the middle is the work itself, which the file and
// command records already describe.
func TestPickTextKeepsBothEnds(t *testing.T) {
	items := []string{"first", "a", "b", "c", "d", "e", "last"}
	kept, dropped := pickText(items, 12, 100)
	if len(kept) == 0 {
		t.Fatal("nothing kept")
	}
	if kept[0] != "first" {
		t.Errorf("first item dropped: %v", kept)
	}
	if kept[len(kept)-1] != "last" {
		t.Errorf("last item dropped: %v", kept)
	}
	if dropped == 0 {
		t.Error("expected a reported drop")
	}
}

func TestDistilSeparatesReadsFromWrites(t *testing.T) {
	s := &transcript.Session{ID: "s", Bed: "b", Start: time.Now(), End: time.Now()}
	s.Tools = []transcript.ToolUse{
		{Name: "Read", Target: "a.go"},
		{Name: "Read", Target: "a.go"},
		{Name: "Edit", Target: "b.go"},
		{Name: "Bash", Target: "go test ./...", Errored: true, Stderr: "FAIL: boom"},
	}
	d := Distil(s, DefaultBudget())

	if len(d.Edited) != 1 || d.Edited[0].Path != "b.go" {
		t.Errorf("Edited = %+v", d.Edited)
	}
	if len(d.Read) != 1 || d.Read[0].Reads != 2 {
		t.Errorf("Read = %+v, want a.go with 2 reads", d.Read)
	}
	if len(d.Commands) != 1 || d.Commands[0].Errored != 1 {
		t.Errorf("Commands = %+v", d.Commands)
	}
	if len(d.Troubles) != 1 || !strings.Contains(d.Troubles[0].Detail, "FAIL") {
		t.Errorf("Troubles = %+v, want the failure kept", d.Troubles)
	}
}

func TestRenderMentionsWhatWasOmitted(t *testing.T) {
	out := Distil(synthetic(2000), DefaultBudget()).Render()
	if !strings.Contains(out, "more distinct commands") {
		t.Error("render hides truncation; a digest must admit what it dropped")
	}
}

func synthetic(n int) *transcript.Session {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	s := &transcript.Session{
		ID: "synthetic", Bed: "bed", CWD: "/src/bed",
		Start: start, End: start.Add(time.Duration(n) * time.Minute),
	}
	for i := 0; i < n; i++ {
		s.Asks = append(s.Asks, transcript.Prompt{Text: strings.Repeat("ask ", 50)})
		s.Notes = append(s.Notes, transcript.Note{Text: strings.Repeat("note ", 60)})
		s.Tools = append(s.Tools, transcript.ToolUse{
			Name: "Bash", Target: "echo " + strings.Repeat("x", i%97),
		})
	}
	return s
}

// A bypass-mode session changes files by redirection rather than by the Edit
// tool. Missing those makes "what changed" read as empty for exactly the
// sessions that did the most work.
func TestWrittenPaths(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"heredoc", "cat > internal/x.go <<'EOF'\npackage x\nEOF", []string{"internal/x.go"}},
		{"append", "echo hi >> notes.txt", []string{"notes.txt"}},
		{"tee", "make 2>&1 | tee build.log", []string{"build.log"}},
		{"sed in place", "sed -i '' 's/a/b/' internal/y.go", []string{"internal/y.go"}},
		{"copy", "cp a.go b.go", []string{"b.go"}},
		{"two redirects", "cat > a.txt <<E\nE\ncat > b.txt <<E\nE", []string{"a.txt", "b.txt"}},
		{"device is not a file", "cmd > /dev/null", nil},
		{"stderr merge is not a file", "cmd 2>&1", nil},
		{"unexpanded shell is not a claim", "cat > $OUT/x.go <<E", nil},
		{"read only", "grep -rn foo internal", nil},
		// A heredoc body is data being written, not shell being run. Reading it
		// as commands invents files that were never touched -- which is exactly
		// how a test fixture mentioning a redirect ends up in the pile.
		{"heredoc body is not shell", "cat > t_test.go <<'EOF'\n{\"append\", \"echo hi >> notes.txt\"}\nEOF", []string{"t_test.go"}},
		{"heredoc then real command", "cat > a.go <<'EOF'\nx\nEOF\ncp a.go b.go", []string{"a.go", "b.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writtenPaths(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("writtenPaths(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The fixture that matters: an injected secret must not survive into anything
// the pile could ever see.
func TestRedactedDigestNeverCarriesTheSecret(t *testing.T) {
	const secret = "ghp_AbCdEf0123456789AbCdEf0123456789"
	s := &transcript.Session{ID: "s", Bed: "b", Start: time.Now(), End: time.Now()}
	s.Asks = []transcript.Prompt{{Text: "deploy with token " + secret}}
	s.Notes = []transcript.Note{{Text: strings.Repeat("x", 130) + " using " + secret}}
	s.Tools = []transcript.ToolUse{
		{Name: "Bash", Target: "curl -H 'Authorization: Bearer " + secret + "' https://api", Errored: true,
			Stderr: "401 with " + secret},
		{Name: "Edit", Target: "/tmp/" + secret + "/config.go"},
	}

	d := Distil(s, DefaultBudget())
	if !strings.Contains(d.Render(), secret) {
		t.Fatal("fixture is not exercising anything: the secret never reached the digest")
	}

	hits := d.Redact(redactor())
	out := d.Render()
	if strings.Contains(out, secret) {
		t.Errorf("secret survived redaction:\n%s", out)
	}
	if len(hits) == 0 || len(d.Redactions) == 0 {
		t.Error("redaction was silent; it must be recorded")
	}
	for _, h := range d.Redactions {
		if strings.Contains(string(h.Class), secret) {
			t.Error("the record of the redaction leaked the value")
		}
	}
}

func redactor() *redact.Redactor { return redact.New() }
