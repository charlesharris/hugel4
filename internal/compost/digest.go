// Package compost turns spent sessions into knowledge.
//
// Composting happens in two stages. The first, here, is mechanical and free: a
// session is distilled into a bounded Digest — what was asked, what was touched,
// what was run, what broke. The second stage hands that digest to a model to
// extract typed entries.
//
// The split exists because the alternative does not work. A long session can
// carry a million tokens of context and hundreds of thousands of characters of
// command output; feeding that to a model to "summarise the session" would cost
// more than the session it is trying to learn from. A composter that produces
// waste is not a composter.
package compost

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charris/hugel/internal/redact"
	"github.com/charris/hugel/internal/transcript"
)

// Budget bounds a digest. The defaults aim at a few thousand tokens regardless
// of how large the session was, so composting cost is roughly constant.
type Budget struct {
	Asks      int // characters of gardener prompts
	Notes     int // characters of agent prose
	AskChars  int // per-prompt truncation
	NoteChars int // per-note truncation
	Commands  int // distinct commands listed
	Files     int // files listed per category
	Errors    int // error excerpts listed
}

// DefaultBudget is tuned so that a twenty-hour session and a five-minute one
// produce digests of the same order of size.
func DefaultBudget() Budget {
	return Budget{
		Asks: 6000, Notes: 6000,
		AskChars: 600, NoteChars: 700,
		Commands: 40, Files: 25, Errors: 8,
	}
}

// FileTouch records how a path was used.
type FileTouch struct {
	Path   string `json:"path"`
	Reads  int    `json:"reads,omitempty"`
	Writes int    `json:"writes,omitempty"`
}

// CommandUse is a distinct command and how often it ran.
type CommandUse struct {
	Command string `json:"command"`
	Runs    int    `json:"runs"`
	Errored int    `json:"errored,omitempty"`
}

// Trouble is something that went wrong, kept for the failure entries it may
// support. Failures are as compostable as successes.
type Trouble struct {
	Where  string `json:"where"`
	Detail string `json:"detail"`
}

// Digest is the bounded, model-ready account of one session.
type Digest struct {
	SessionID string        `json:"session_id"`
	Bed       string        `json:"bed"`
	Directory string        `json:"directory"`
	Branch    string        `json:"branch"`
	Start     time.Time     `json:"start"`
	End       time.Time     `json:"end"`
	Duration  time.Duration `json:"duration"`

	Asks  []string `json:"asks"`
	Notes []string `json:"notes"`

	Edited   []FileTouch  `json:"edited,omitempty"`
	Read     []FileTouch  `json:"read,omitempty"`
	Commands []CommandUse `json:"commands,omitempty"`
	Records  []Record     `json:"records,omitempty"`
	Troubles []Trouble    `json:"troubles,omitempty"`

	ToolCalls  int          `json:"tool_calls"`
	Redactions []redact.Hit `json:"redactions,omitempty"`
	Truncated  struct {
		Asks     int `json:"asks,omitempty"`
		Notes    int `json:"notes,omitempty"`
		Files    int `json:"files,omitempty"`
		Commands int `json:"commands,omitempty"`
	} `json:"truncated,omitempty"`
}

// Distil reduces a session to a digest under the given budget.
func Distil(s *transcript.Session, b Budget) *Digest {
	d := &Digest{
		SessionID: s.ID, Bed: s.Bed, Directory: s.CWD, Branch: s.Branch,
		Start: s.Start, End: s.End, Duration: s.Duration(),
		ToolCalls: len(s.Tools),
	}

	d.Asks, d.Truncated.Asks = pickText(prompts(s.Asks), b.Asks, b.AskChars)
	d.Notes, d.Truncated.Notes = pickText(notes(s.Notes), b.Notes, b.NoteChars)

	files := map[string]*FileTouch{}
	cmds := map[string]*CommandUse{}
	for _, t := range s.Tools {
		switch {
		case t.Writes():
			touch(files, t.Target).Writes++
		case t.Reads():
			touch(files, t.Target).Reads++
		case t.Name == "Bash" && t.Target != "":
			d.Records = append(d.Records, commitsIn(t.Target)...)
			d.Records = append(d.Records, beadRecordsIn(t.Target)...)
			d.Records = append(d.Records, memoryRecordsIn(t.Target)...)
			// A shell session changes files by redirection, not by the Edit
			// tool. Missing those makes "what changed" -- the most useful
			// signal a digest carries -- read as empty for exactly the
			// sessions that did the most work.
			for _, p := range writtenPaths(t.Target) {
				touch(files, p).Writes++
			}
			c := cmds[normaliseCommand(t.Target)]
			if c == nil {
				c = &CommandUse{Command: normaliseCommand(t.Target)}
				cmds[c.Command] = c
			}
			c.Runs++
			if t.Errored {
				c.Errored++
			}
		}
		if t.Errored && len(d.Troubles) < b.Errors {
			if detail := firstLine(t.Stderr); detail != "" {
				d.Troubles = append(d.Troubles, Trouble{Where: describe(t), Detail: detail})
			}
		}
	}

	var edited, read []FileTouch
	for _, f := range files {
		if f.Writes > 0 {
			edited = append(edited, *f)
		} else if f.Reads > 0 {
			read = append(read, *f)
		}
	}
	d.Edited, d.Truncated.Files = topFiles(edited, b.Files)
	var dropped int
	d.Read, dropped = topFiles(read, b.Files)
	d.Truncated.Files += dropped

	all := make([]CommandUse, 0, len(cmds))
	for _, c := range cmds {
		all = append(all, *c)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Runs != all[j].Runs {
			return all[i].Runs > all[j].Runs
		}
		return all[i].Command < all[j].Command
	})
	if len(all) > b.Commands {
		d.Truncated.Commands = len(all) - b.Commands
		all = all[:b.Commands]
	}
	d.Commands = all

	return d
}

func prompts(ps []transcript.Prompt) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if harnessNoise(p.Text) {
			continue
		}
		out = append(out, p.Text)
	}
	return out
}

func notes(ns []transcript.Note) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		// Very short agent turns are acknowledgements, not accounts.
		if len(n.Text) < 120 {
			continue
		}
		out = append(out, n.Text)
	}
	return out
}

// pickText fits items into a character budget by taking from both ends: the
// opening of a session carries intent and the close carries outcome, while the
// long middle is mostly the work itself, which the tool and file records
// already describe.
func pickText(items []string, budget, each int) (kept []string, dropped int) {
	if len(items) == 0 {
		return nil, 0
	}
	trimmed := make([]string, len(items))
	for i, s := range items {
		trimmed[i] = clip(collapse(s), each)
	}
	total := 0
	for _, s := range trimmed {
		total += len(s)
	}
	if total <= budget {
		return trimmed, 0
	}
	var head, tail []string
	used, lo, hi := 0, 0, len(trimmed)-1
	for lo <= hi {
		if used+len(trimmed[lo]) > budget {
			break
		}
		head = append(head, trimmed[lo])
		used += len(trimmed[lo])
		lo++
		if lo > hi {
			break
		}
		if used+len(trimmed[hi]) > budget {
			break
		}
		tail = append(tail, trimmed[hi])
		used += len(trimmed[hi])
		hi--
	}
	for i, j := 0, len(tail)-1; i < j; i, j = i+1, j-1 {
		tail[i], tail[j] = tail[j], tail[i]
	}
	return append(head, tail...), len(trimmed) - len(head) - len(tail)
}

func touch(m map[string]*FileTouch, path string) *FileTouch {
	if path == "" {
		path = "(unknown)"
	}
	f := m[path]
	if f == nil {
		f = &FileTouch{Path: path}
		m[path] = f
	}
	return f
}

func topFiles(fs []FileTouch, limit int) ([]FileTouch, int) {
	sort.Slice(fs, func(i, j int) bool {
		a, b := fs[i].Writes+fs[i].Reads, fs[j].Writes+fs[j].Reads
		if a != b {
			return a > b
		}
		return fs[i].Path < fs[j].Path
	})
	if len(fs) > limit {
		return fs[:limit], len(fs) - limit
	}
	return fs, 0
}

// normaliseCommand collapses a shell command to its recognisable shape so that
// forty variations of the same grep count as one thing worth noticing.
//
// Three kinds of noise dominate real transcripts and none of them say anything
// about the work: a leading "cd" to set the working directory, environment
// assignments, and heredoc bodies — which can run to hundreds of lines of the
// file being written, all of which the file records already cover.
func normaliseCommand(cmd string) string {
	lines := strings.Split(cmd, "\n")
	for _, l := range lines {
		l = peelPreamble(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		return shapeCommand(l)
	}
	// Everything was positioning. Report it rather than returning nothing, so a
	// bare "cd" is still visible as the no-op it is.
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			return shapeCommand(l)
		}
	}
	return ""
}

// peelPreamble strips the leading shell noise that says where a command runs
// rather than what it does. The two kinds behave differently and must not be
// conflated: "cd /x" consumes its argument and the real command follows a
// separator, while "FOO=1" merely prefixes the command on the same line.
// Returns "" when the whole line was preamble.
func peelPreamble(l string) string {
	for {
		l = strings.TrimSpace(l)
		w, after, ok := firstWord(l)
		if !ok {
			return l
		}
		switch {
		case w == "cd":
			l = afterSeparator(after)
		case isAssignment(w):
			l = after
		default:
			return l
		}
	}
}

// firstWord splits a line into its first word and the remainder of the line.
func firstWord(l string) (word, rest string, ok bool) {
	if l == "" {
		return "", "", false
	}
	i := strings.IndexAny(l, " \t")
	if i < 0 {
		return l, "", true
	}
	return l[:i], strings.TrimSpace(l[i:]), true
}

// afterSeparator returns whatever follows the first shell command separator,
// or "" when the line ends without one.
func afterSeparator(l string) string {
	sep := -1
	width := 0
	for _, s := range []string{"&&", "||", ";"} {
		if i := strings.Index(l, s); i >= 0 && (sep < 0 || i < sep) {
			sep, width = i, len(s)
		}
	}
	if sep < 0 {
		return ""
	}
	return strings.TrimSpace(l[sep+width:])
}

// isAssignment reports whether a word is a NAME=value environment prefix. A
// path or filename containing "=" is not one.
func isAssignment(w string) bool {
	i := strings.IndexByte(w, '=')
	return i > 0 && !strings.ContainsAny(w[:i], "/\\.")
}

// shapeCommand reduces a single command to its recognisable shape: the heredoc
// marker without its document, and the head of a pipeline without its tail.
func shapeCommand(l string) string {
	if i := strings.Index(l, "<<"); i >= 0 {
		rest := l[i:]
		if j := strings.IndexAny(rest[2:], " \t"); j >= 0 {
			rest = rest[:2+j]
		}
		l = l[:i] + rest
	}
	l = collapse(l)
	if i := strings.IndexAny(l, "|&;"); i > 0 {
		l = strings.TrimSpace(l[:i]) + " …"
	}
	return clip(l, 120)
}

// harnessNoise matches turns the harness generates on the gardener's behalf —
// slash-command echoes, interrupt markers, local command output. They are not
// intent and must not crowd out the prompts that are.
func harnessNoise(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	for _, p := range []string{"<command-name>", "<local-command-stdout>", "<command-message>", "<bash-input>"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return strings.HasPrefix(s, "[Request interrupted") || strings.HasPrefix(s, "API Error")
}

// redirection matches the shell forms that create or replace a file.
var redirection = regexp.MustCompile(`(?:^|[|&;]|\s)(?:>>?|tee(?:\s+-a)?)\s+"?'?([^\s"'|&;<>]+)`)

// lastArgWriters are commands whose target is their final argument.
var lastArgWriters = regexp.MustCompile(`^(?:sed\s+-i|cp|mv|install)\b`)

// writtenPaths guesses which files a shell command created or replaced. It is
// deliberately conservative: a missed file costs the digest some detail, while
// a wrong one puts a claim in the pile about something that never happened.
func writtenPaths(cmd string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		switch {
		case p == "", seen[p]:
			return
		case strings.HasPrefix(p, "/dev/"), strings.HasPrefix(p, "&"):
			return // a device or a descriptor, not a file
		case strings.ContainsAny(p, "$`*?"):
			return // unexpanded shell: hugel does not know what this was
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, line := range commandLines(cmd) {
		for _, seg := range splitCommands(line) {
			for _, m := range redirection.FindAllStringSubmatch(seg, -1) {
				add(m[1])
			}
			if lastArgWriters.MatchString(strings.TrimSpace(seg)) {
				if f := strings.Fields(seg); len(f) > 1 {
					add(f[len(f)-1])
				}
			}
		}
	}
	return out
}

// heredocStart finds the terminator word of a heredoc opened on a line.
var heredocStart = regexp.MustCompile(`<<-?\s*["']?([A-Za-z_][A-Za-z0-9_]*)["']?`)

// commandLines yields the lines of a command that are actually commands. A
// heredoc body is data being written, not instructions being run, and reading
// it as shell invents files that were never touched -- a test fixture that
// mentions "echo hi >> notes.txt" would otherwise put notes.txt in the pile.
func commandLines(cmd string) []string {
	var out []string
	var terminator string
	for _, line := range strings.Split(cmd, "\n") {
		if terminator != "" {
			if strings.TrimSpace(line) == terminator {
				terminator = ""
			}
			continue
		}
		out = append(out, line)
		if m := heredocStart.FindStringSubmatch(line); m != nil {
			terminator = m[1]
		}
	}
	return out
}

// splitCommands breaks a line at shell separators so that the last argument of
// one command is not mistaken for the last argument of the line.
func splitCommands(line string) []string {
	segs := []string{line}
	for _, sep := range []string{"&&", "||", ";", "|"} {
		var next []string
		for _, s := range segs {
			next = append(next, strings.Split(s, sep)...)
		}
		segs = next
	}
	return segs
}

func describe(t transcript.ToolUse) string {
	if t.Name == "Bash" {
		return clip(normaliseCommand(t.Target), 60)
	}
	if t.Target != "" {
		return t.Name + " " + filepath.Base(t.Target)
	}
	return t.Name
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(s, 200)
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// Size is the digest's rendered length in characters, the number that has to
// stay roughly constant across session sizes.
func (d *Digest) Size() int { return len(d.Render()) }

func (d *Digest) String() string {
	return fmt.Sprintf("digest %s (%s): %d asks, %d notes, %d files, %d commands, %d chars",
		d.SessionID, d.Bed, len(d.Asks), len(d.Notes),
		len(d.Edited)+len(d.Read), len(d.Commands), d.Size())
}
