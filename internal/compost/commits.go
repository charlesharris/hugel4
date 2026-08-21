package compost

import (
	"regexp"
	"strings"
)

// Record is something written down during a session deliberately, as a record:
// a commit message, or a bead closed with a reason.
//
// These are the richest compostable material a transcript holds. Unlike agent
// prose, someone chose to write them as a record; unlike a tool call, the thing
// they describe demonstrably happened. An extractor working from records is
// reporting rather than inferring, which is why the first one can be a set of
// regexes instead of a model.
type Record struct {
	Kind    string `json:"kind"` // commit, bead-closed, bead-decision
	Subject string `json:"subject"`
	Body    string `json:"body,omitempty"`
	Bead    string `json:"bead,omitempty"`
	Revert  bool   `json:"revert,omitempty"`
}

const (
	KindCommit       = "commit"
	KindBeadClosed   = "bead-closed"
	KindBeadDecision = "bead-decision"
)

var (
	gitCommit  = regexp.MustCompile(`(?:^|[|&;]\s*|\s)git\s+commit\b`)
	dashM      = regexp.MustCompile(`-([a-zA-Z]*m[a-zA-Z]*)\s+("([^"]*)"|'([^']*)')`)
	heredocTag = regexp.MustCompile(`<<-?\s*["']?([A-Za-z_][A-Za-z0-9_]*)["']?`)
)

// commitsIn pulls commit messages out of a shell command. Both forms in real
// use are handled: an inline -m, and a heredoc piped to -F -, which is how any
// message worth composting gets written.
func commitsIn(cmd string) []Record {
	if !gitCommit.MatchString(cmd) {
		return nil
	}
	var out []Record

	lines := strings.Split(cmd, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !gitCommit.MatchString(line) {
			continue
		}
		if m := dashM.FindStringSubmatch(line); m != nil {
			if msg := m[3] + m[4]; strings.TrimSpace(msg) != "" {
				out = append(out, newCommit(msg))
			}
			continue
		}
		// A heredoc: the message is the document that follows.
		tag := heredocTag.FindStringSubmatch(line)
		if tag == nil {
			continue
		}
		var body []string
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == tag[1] {
				i = j
				break
			}
			body = append(body, lines[j])
		}
		if msg := strings.TrimSpace(strings.Join(body, "\n")); msg != "" {
			out = append(out, newCommit(msg))
		}
	}
	return out
}

func newCommit(msg string) Record {
	msg = strings.TrimSpace(msg)
	msg = stripTrailers(msg)
	subject, body, _ := strings.Cut(msg, "\n")
	subject = strings.TrimSpace(subject)
	return Record{
		Kind:    KindCommit,
		Subject: subject,
		Body:    strings.TrimSpace(body),
		// A revert is the one commit shape that records a failure rather than a
		// decision: something was tried, and taken back.
		Revert: strings.HasPrefix(strings.ToLower(subject), "revert"),
	}
}

// trivial reports whether a commit says nothing worth keeping.
func (c Record) trivial() bool {
	s := strings.ToLower(c.Subject)
	if len(c.Subject) < 15 {
		return true
	}
	for _, p := range []string{"wip", "fixup", "typo", "squash!", "fixup!", "merge branch", "merge pull"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// trailerKey matches a git trailer: a "Key: value" line in the message footer.
var trailerKey = regexp.MustCompile(`^[A-Z][A-Za-z-]*:\s`)

// stripTrailers removes the git footer. Co-Authored-By and friends are
// provenance for the repository, and in the pile they are noise that every
// entry carries and no reader wants.
func stripTrailers(msg string) string {
	lines := strings.Split(msg, "\n")
	cut := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" {
			continue
		}
		if !trailerKey.MatchString(l) {
			break
		}
		cut = i
	}
	return strings.TrimSpace(strings.Join(lines[:cut], "\n"))
}
