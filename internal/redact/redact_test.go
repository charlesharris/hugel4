package redact

import (
	"strings"
	"testing"
)

// Every credential shape hugel claims to catch, caught.
func TestFormatDetectors(t *testing.T) {
	tests := []struct {
		name, in string
		class    Class
	}{
		{"anthropic", "key is sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFFGGGG here", ClassAnthropicKey},
		{"github pat", "token ghp_AbCdEf0123456789AbCdEf0123456789 ok", ClassGitHubToken},
		{"github fine-grained", "github_pat_" + strings.Repeat("A", 60), ClassGitHubToken},
		{"aws", "id AKIAIOSFODNN7EXAMPLE done", ClassAWSKey},
		{"slack", "xoxb-1234567890-abcdefghij", ClassSlackToken},
		{"jwt", "auth eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NX0.dBjftJeZ4CVPmB92K", ClassJWT},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIabc\n-----END RSA PRIVATE KEY-----", ClassPrivateKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, hits := New().Redact(tt.in)
			if Total(hits) == 0 {
				t.Fatalf("nothing detected in %q", tt.in)
			}
			if hits[0].Class != tt.class {
				t.Errorf("class = %q, want %q", hits[0].Class, tt.class)
			}
			if !strings.Contains(out, "[redacted:") {
				t.Errorf("output not redacted: %q", out)
			}
		})
	}
}

// The failure that matters most. A knowledge base full of mangled evidence is
// worthless in a quieter way than one with a leaked secret, so anything that
// merely looks random must survive intact.
func TestDoesNotMangleInnocentHighEntropyText(t *testing.T) {
	innocent := []string{
		"commit cb429df8a3e1f4b2c6d7e8f9a0b1c2d3e4f5a6b7",
		"sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"digest AAAAB3NzaC1yc2EAAAADAQABAAABgQDZ1234567890abcdefGHIJKL",
		"uuid 76489b29-33ef-451f-9a43-61b1f0e41e27",
		"go.sum h1:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG=",
		"version 2.1.238 and port 5432",
	}
	r := New()
	for _, s := range innocent {
		out, hits := r.Redact(s)
		if out != s {
			t.Errorf("mangled innocent text\n in: %q\nout: %q\nhits: %+v", s, out, hits)
		}
	}
}

// Context is what licenses redaction, not entropy. The same opaque string is
// redacted when something calls it a token and left alone when nothing does.
func TestContextualAssignment(t *testing.T) {
	r := New()

	out, hits := r.Redact("export GITHUB_TOKEN=abc123def456ghi789")
	if Total(hits) == 0 || strings.Contains(out, "abc123def456ghi789") {
		t.Errorf("named secret survived: %q", out)
	}

	same := "export BUILD_ID=abc123def456ghi789"
	out, _ = r.Redact(same)
	if out != same {
		t.Errorf("unnamed value redacted on shape alone: %q", out)
	}
}

func TestURLCredentialKeepsTheUsefulPart(t *testing.T) {
	out, hits := New().Redact("postgres://hugel:s3cr3tpassword@localhost:5432/hugel")
	if strings.Contains(out, "s3cr3tpassword") {
		t.Fatalf("password survived: %q", out)
	}
	if !strings.Contains(out, "postgres://hugel:") || !strings.Contains(out, "@localhost:5432/hugel") {
		t.Errorf("redaction destroyed the connection shape: %q", out)
	}
	if hits[0].Class != ClassURLCredential {
		t.Errorf("class = %q", hits[0].Class)
	}
}

// Exact matching against known values is the workhorse: no false positives,
// and it catches secrets whose format hugel has never seen.
func TestKnownValuesMatchedExactly(t *testing.T) {
	secret := "wholly-unremarkable-string-9f3a"
	r := New(secret)
	out, hits := r.Redact("the deploy used " + secret + " twice: " + secret)
	if strings.Contains(out, secret) {
		t.Fatalf("known secret survived: %q", out)
	}
	if hits[0].Count != 2 {
		t.Errorf("count = %d, want 2", hits[0].Count)
	}
}

// A real environment is full of placeholders and non-secrets. Treating them as
// known values would redact ordinary words everywhere they appear.
func TestPlaceholdersAreNotTreatedAsSecrets(t *testing.T) {
	junk := []string{
		"your-anthropic-api-key", "changeme", "true", "1", "example-token-here",
		"/Users/charris/.config/creds", "some value with spaces",
	}
	r := New(junk...)
	if len(r.known) != 0 {
		t.Errorf("accepted placeholders as secrets: %q", r.known)
	}
}

// Findings must be safe to log and store. If the value appears in the hit, the
// audit trail becomes a second leak.
func TestHitsNeverCarryTheValue(t *testing.T) {
	secret := "ghp_AbCdEf0123456789AbCdEf0123456789"
	_, hits := New().Redact("token " + secret)
	for _, h := range hits {
		if strings.Contains(string(h.Class), secret) {
			t.Fatal("hit carried the secret value")
		}
	}
}

func TestRedactionIsIdempotent(t *testing.T) {
	r := New("wholly-unremarkable-string-9f3a")
	in := "key sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFF and GITHUB_TOKEN=abc123def456"
	once, _ := r.Redact(in)
	twice, hits := r.Redact(once)
	if once != twice {
		t.Errorf("second pass changed the text\n1: %q\n2: %q", once, twice)
	}
	if Total(hits) != 0 {
		t.Errorf("second pass re-detected its own markers: %+v", hits)
	}
}
