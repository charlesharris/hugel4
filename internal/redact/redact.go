// Package redact removes credentials before anything durable is written.
//
// The pile is permanent and shared across every bed. A secret that leaks into a
// transcript is embarrassing; a secret that leaks into the pile is there
// forever, and travels to every project. So redaction runs before persistence,
// not after, and it is deliberately biased: it would rather refuse to identify
// a secret than mangle a git SHA, because a knowledge base full of corrupted
// evidence is worthless in a quieter, harder-to-notice way.
//
// The strategy, in order of trust:
//
//  1. Exact match against values hugel already knows are secret (the
//     environment). This is the workhorse and has no false positives.
//  2. Format detectors for credentials that announce themselves.
//  3. A contextual heuristic for assignments that name a secret, which fires
//     only when a key-like word is adjacent — never on entropy alone.
//
// Detections record their class and count. They never record the value.
package redact

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Class names a kind of credential without naming the credential.
type Class string

const (
	ClassAnthropicKey  Class = "anthropic-key"
	ClassGitHubToken   Class = "github-token"
	ClassAWSKey        Class = "aws-key"
	ClassSlackToken    Class = "slack-token"
	ClassOpenAIKey     Class = "openai-key"
	ClassPrivateKey    Class = "private-key"
	ClassJWT           Class = "jwt"
	ClassURLCredential Class = "url-credential"
	ClassEnvSecret     Class = "env-secret"
	ClassAssignment    Class = "named-secret"
)

// Hit is one class of finding and how many times it fired. The value is
// deliberately absent: hugel must be able to log and store its own redaction
// activity without that record becoming a second leak.
type Hit struct {
	Class Class `json:"class"`
	Count int   `json:"count"`
}

type pattern struct {
	class Class
	re    *regexp.Regexp
}

// Format detectors. Each matches a credential shape distinctive enough that a
// match is not plausibly anything else.
var patterns = []pattern{
	{ClassPrivateKey, regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)},
	{ClassAnthropicKey, regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`)},
	{ClassOpenAIKey, regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9_\-]{32,}`)},
	{ClassGitHubToken, regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{ClassGitHubToken, regexp.MustCompile(`github_pat_[A-Za-z0-9_]{50,}`)},
	{ClassAWSKey, regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{ClassSlackToken, regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`)},
	{ClassJWT, regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\b`)},
	// user:password@host in a URL — the password is the part that matters.
	{ClassURLCredential, regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://[^\s:/@]+):([^\s:/@]{3,})@`)},
}

// assignment matches NAME=value / NAME: value where NAME names a secret. The
// key word is what licenses the redaction — never the shape of the value — so
// a git SHA or a base64 fixture passes through untouched unless something
// nearby calls it a token.
var assignment = regexp.MustCompile(`(?i)\b([A-Za-z0-9_\-]*(?:password|passwd|secret|token|api[_\-]?key|access[_\-]?key|private[_\-]?key|credential)[A-Za-z0-9_\-]*)\s*[:=]\s*["']?([^\s"'&;|]{6,})["']?`)

// envSecretName matches environment variable names that hold credentials.
var envSecretName = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_\-]?key|access[_\-]?key|private[_\-]?key|credential|auth)`)

// Redactor removes credentials from text.
type Redactor struct {
	known []string // exact values, longest first
}

// New returns a redactor that also matches the given literal values exactly.
func New(known ...string) *Redactor {
	r := &Redactor{}
	for _, k := range known {
		if isPlausibleSecret(k) {
			r.known = append(r.known, k)
		}
	}
	// Longest first, so a value that contains another is replaced whole.
	sort.Slice(r.known, func(i, j int) bool { return len(r.known[i]) > len(r.known[j]) })
	return r
}

// FromEnv returns a redactor primed with the secret-looking values in the
// current environment. Exact matching against values hugel can already see is
// the highest-confidence detection available and costs nothing.
func FromEnv() *Redactor {
	var known []string
	for _, kv := range os.Environ() {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !envSecretName.MatchString(name) {
			continue
		}
		known = append(known, value)
	}
	return New(known...)
}

// isPlausibleSecret filters out the placeholders and obvious non-secrets that
// litter a real environment. Redacting the string "true" everywhere it appears
// would be worse than missing a secret.
func isPlausibleSecret(v string) bool {
	if len(v) < 12 {
		return false
	}
	l := strings.ToLower(v)
	for _, junk := range []string{"your-", "changeme", "example", "placeholder", "xxxx", "<", "dummy", "notset"} {
		if strings.Contains(l, junk) {
			return false
		}
	}
	// A path or a sentence is not a credential.
	if strings.ContainsAny(v, " \t") || strings.HasPrefix(v, "/") {
		return false
	}
	return true
}

// Redact returns the text with credentials replaced by class markers, and a
// summary of what fired.
func (r *Redactor) Redact(s string) (string, []Hit) {
	counts := map[Class]int{}

	for _, k := range r.known {
		if n := strings.Count(s, k); n > 0 {
			s = strings.ReplaceAll(s, k, marker(ClassEnvSecret))
			counts[ClassEnvSecret] += n
		}
	}

	for _, p := range patterns {
		if p.class == ClassURLCredential {
			s = p.re.ReplaceAllStringFunc(s, func(m string) string {
				counts[ClassURLCredential]++
				sub := p.re.FindStringSubmatch(m)
				return sub[1] + ":" + marker(ClassURLCredential) + "@"
			})
			continue
		}
		s = p.re.ReplaceAllStringFunc(s, func(string) string {
			counts[p.class]++
			return marker(p.class)
		})
	}

	s = assignment.ReplaceAllStringFunc(s, func(m string) string {
		sub := assignment.FindStringSubmatch(m)
		if alreadyRedacted(sub[2]) {
			return m
		}
		counts[ClassAssignment]++
		return strings.Replace(m, sub[2], marker(ClassAssignment), 1)
	})

	hits := make([]Hit, 0, len(counts))
	for c, n := range counts {
		hits = append(hits, Hit{Class: c, Count: n})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Class < hits[j].Class })
	return s, hits
}

// Strings redacts a slice in place-ish, returning the cleaned slice and the
// combined findings.
func (r *Redactor) Strings(in []string) ([]string, []Hit) {
	out := make([]string, len(in))
	counts := map[Class]int{}
	for i, s := range in {
		clean, hits := r.Redact(s)
		out[i] = clean
		for _, h := range hits {
			counts[h.Class] += h.Count
		}
	}
	hits := make([]Hit, 0, len(counts))
	for c, n := range counts {
		hits = append(hits, Hit{Class: c, Count: n})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Class < hits[j].Class })
	return out, hits
}

func marker(c Class) string { return fmt.Sprintf("[redacted:%s]", c) }

func alreadyRedacted(s string) bool { return strings.HasPrefix(s, "[redacted:") }

// Total is the number of findings across all classes.
func Total(hits []Hit) int {
	n := 0
	for _, h := range hits {
		n += h.Count
	}
	return n
}
