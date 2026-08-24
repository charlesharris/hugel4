package compost

import (
	"regexp"
	"strings"
)

// bd is hugel's issue tracker at runtime, and a bead closed with a reason is a
// decision record its author wrote by hand. Mining those is reporting, not
// inference.
var (
	beadClose    = regexp.MustCompile(`(?:^|[|&;]\s*|\s)bd\s+close\s+(\S+)[^\n]*?--reason\s+(?:"([^"]*)"|'([^']*)')`)
	beadCreate   = regexp.MustCompile(`(?:^|[|&;]\s*|\s)bd\s+create\s+(?:"([^"]*)"|'([^']*)')`)
	beadRemember = regexp.MustCompile(`(?:^|[|&;]\s*|\s)bd\s+remember\s+(?:"([^"]*)"|'([^']*)')`)
	beadType     = regexp.MustCompile(`-t\s+(\w+)`)
	beadDesc     = regexp.MustCompile(`-d\s+(?:"([^"]*)"|'([^']*)')`)
	beadID       = regexp.MustCompile(`\b([a-z]{2,5}-[a-z0-9]*[a-z][a-z0-9]*(?:\.\d+)*)\b`)
	// A record whose subject opens with an identifier and a colon is naming its
	// work item. That leading position is what makes it safe to read an
	// uppercase token as a bead rather than as "UTF-8" or "SHA-256".
	beadLead = regexp.MustCompile(`^([A-Za-z]{2,5}-[A-Za-z0-9]{1,8}(?:\.\d+)*)(?:/\S+)?\s*:`)
)

// beadRecordsIn pulls deliberate records out of bd invocations: a bead closed
// with a stated reason, and a bead created explicitly as a decision.
//
// Only "-t decision" is taken from creations. A bug or a feature is work to be
// done, not knowledge that was earned, and composting the backlog would fill
// the pile with intentions rather than findings.
func beadRecordsIn(cmd string) []Record {
	if !strings.Contains(cmd, "bd ") {
		return nil
	}
	joined := strings.ReplaceAll(cmd, "\\\n", " ")
	var out []Record

	for _, m := range beadClose.FindAllStringSubmatch(joined, -1) {
		reason := strings.TrimSpace(m[2] + m[3])
		if reason == "" {
			continue
		}
		subject, body := firstSentence(reason)
		out = append(out, Record{
			Kind: KindBeadClosed, Bead: m[1], Subject: subject, Body: body,
		})
	}

	for _, seg := range splitCommands(joined) {
		m := beadCreate.FindStringSubmatch(seg)
		if m == nil {
			continue
		}
		kind := beadType.FindStringSubmatch(seg)
		if kind == nil || kind[1] != "decision" {
			continue
		}
		title := strings.TrimSpace(m[1] + m[2])
		var body string
		if d := beadDesc.FindStringSubmatch(seg); d != nil {
			body = strings.TrimSpace(d[1] + d[2])
		}
		out = append(out, Record{Kind: KindBeadDecision, Subject: title, Body: body})
	}
	return out
}

// firstSentence splits a reason into a headline and the rest, so a paragraph
// becomes a titled entry rather than a title the length of a paragraph.
func firstSentence(s string) (head, rest string) {
	s = collapse(s)
	if len(s) <= 90 {
		return s, ""
	}
	for _, end := range []string{". ", "; ", " -- ", ", and "} {
		if i := strings.Index(s, end); i > 20 && i < 120 {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s)
		}
	}
	cut := 90
	if i := strings.LastIndex(s[:cut], " "); i > 40 {
		cut = i
	}
	return strings.TrimSpace(s[:cut]) + "…", strings.TrimSpace(s)
}

// beadsIn finds bead identifiers mentioned in text.
func beadsIn(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.SplitN(s, "\n", 2) {
		if m := beadLead.FindStringSubmatch(strings.TrimSpace(line)); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
		break
	}
	for _, m := range beadID.FindAllStringSubmatch(s, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// memoryRecordsIn pulls `bd remember` out of a shell command.
//
// A memory is the highest-intent record bd produces: someone read a session,
// decided one sentence of it should outlive the session, and wrote it down for
// no other purpose. That is what the pile is for, and mining it is reporting
// rather than inference.
//
// It is not, however, evidence in the way a commit is. A commit message
// describes a change that demonstrably happened; a memory is an assertion with
// no diff behind it, and it may be written by an agent rather than a person.
// So it is composted with a lower confidence than a recorded change and, like
// everything else, arrives unreviewed.
func memoryRecordsIn(cmd string) []Record {
	if !strings.Contains(cmd, "bd ") {
		return nil
	}
	joined := strings.ReplaceAll(cmd, "\\\n", " ")
	var out []Record
	for _, m := range beadRemember.FindAllStringSubmatch(joined, -1) {
		text := strings.TrimSpace(m[1] + m[2])
		if text == "" {
			continue
		}
		subject, body := firstSentence(text)
		out = append(out, Record{Kind: KindMemory, Subject: subject, Body: body})
	}
	return out
}
