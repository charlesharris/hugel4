package compost

import "testing"

func TestCommitsIn(t *testing.T) {
	tests := []struct {
		name, cmd string
		want      []string
	}{
		{"inline -m", `git commit -m "identify: let runtime choose the match"`,
			[]string{"identify: let runtime choose the match"}},
		{"combined flags", `git commit -am 'One drive is the target, and the reason is the bus'`,
			[]string{"One drive is the target, and the reason is the bus"}},
		{"heredoc", "git commit -q -F - <<'EOF'\nA subject line\n\nThe reasoning.\nEOF",
			[]string{"A subject line"}},
		{"behind a cd", "cd /src && git commit -m \"a subject line here\"",
			[]string{"a subject line here"}},
		{"not a commit", `git status`, nil},
		// The heredoc body is the message, not shell -- and unlike everywhere
		// else in the digest, here it is exactly what we want to keep.
		{"heredoc body is the record", "git commit -F - <<'EOF'\nSubject\n\ncat > decoy.go <<X\nX\nEOF",
			[]string{"Subject"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitsIn(tt.cmd)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d commits %+v, want %d", len(got), got, len(tt.want))
			}
			for i := range got {
				if got[i].Subject != tt.want[i] {
					t.Errorf("[%d] subject = %q, want %q", i, got[i].Subject, tt.want[i])
				}
			}
		})
	}
}

func TestCommitBodySeparation(t *testing.T) {
	got := commitsIn("git commit -F - <<'EOF'\nThe subject line\n\nThe reasoning, at length.\nEOF")
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].Subject != "The subject line" {
		t.Errorf("subject = %q", got[0].Subject)
	}
	if got[0].Body != "The reasoning, at length." {
		t.Errorf("body = %q", got[0].Body)
	}
}

func TestBeadRecords(t *testing.T) {
	closed := beadRecordsIn(`bd close hb-1hq.12 --reason "ProviderNet now scores every result and picks the best by confidence"`)
	if len(closed) != 1 || closed[0].Bead != "hb-1hq.12" {
		t.Fatalf("close = %+v", closed)
	}
	if closed[0].Kind != KindBeadClosed {
		t.Errorf("kind = %q", closed[0].Kind)
	}

	// Only decisions. A bug or a feature is work to be done, not knowledge
	// earned; composting the backlog would fill the pile with intentions.
	decision := beadRecordsIn(`bd create "A film auto-confirms at 0.7 with no provider id" -t decision -d 'the v1 output v2 exists to replace'`)
	if len(decision) != 1 || decision[0].Kind != KindBeadDecision {
		t.Fatalf("decision = %+v", decision)
	}
	if decision[0].Body != "the v1 output v2 exists to replace" {
		t.Errorf("body = %q", decision[0].Body)
	}

	for _, cmd := range []string{
		`bd create "Television gets none of the discipline film gets" -t bug -p 1`,
		`bd create "The TV data layer is schema with nothing behind it" -t feature`,
		`bd list`,
	} {
		if got := beadRecordsIn(cmd); len(got) != 0 {
			t.Errorf("beadRecordsIn(%q) = %+v, want nothing", cmd, got)
		}
	}
}

// A leading identifier followed by a colon is naming a work item. That position
// is what makes it safe to read an uppercase token as a bead rather than as
// "UTF-8" or "SHA-256".
func TestBeadsIn(t *testing.T) {
	if got := beadsIn("TDS-19/20: keep the reader's place"); len(got) != 1 || got[0] != "TDS-19" {
		t.Errorf("leading id = %v", got)
	}
	if got := beadsIn("hb-1hq is the epic for this"); len(got) != 1 || got[0] != "hb-1hq" {
		t.Errorf("inline id = %v", got)
	}
	for _, s := range []string{"encoded as UTF-8 throughout", "hashed with SHA-256 before storage"} {
		if got := beadsIn(s); len(got) != 0 {
			t.Errorf("beadsIn(%q) = %v, want nothing", s, got)
		}
	}
}

func TestFirstSentence(t *testing.T) {
	head, rest := firstSentence("Short reason")
	if head != "Short reason" || rest != "" {
		t.Errorf("short = %q / %q", head, rest)
	}
	long := "Parsers split out of the _linux files into sense.go and region.go. The tree now builds on any host, which unblocks laptop work entirely."
	head, rest = firstSentence(long)
	if len(head) > 120 || head == long {
		t.Errorf("head = %q", head)
	}
	if rest != long {
		t.Error("the full text should survive as the body")
	}
}
