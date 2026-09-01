package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sandbox refuses a garden outside a temporary directory while a test binary is
// running.
//
// The hazard is omission, not ignorance. Every test that touches the garden
// already knows to set HUGEL_HOME; the ones that leaked were the ones that
// never thought about the garden at all — a gate test that runs a gate, and the
// gate emits. Per-test Setenv was tried and holds only until someone adds a
// test that does not know it emits, which is exactly the test that will be
// added next. The pile learned this once (it was moved under the garden because
// a test nearly wrote to the real one) and events inherited the hazard without
// the lesson, so the guard is here at the root rather than in one package.
//
// It panics rather than returning an error. Home's error return is for a
// machine with no home directory, and every caller handles it by giving up
// quietly — events.Emit swallows it by design, because an instrument that can
// break the thing it measures is worse than no instrument. A swallowed refusal
// is a test that silently measures nothing, so the refusal has to be the one
// kind of failure a test cannot ignore. It costs the shipped binary nothing:
// testing.Testing() is false there and the whole check is a branch.
//
// The rule is a temp dir rather than "HUGEL_HOME is set", so that a test which
// points the garden at a real directory is refused too.
func Sandbox(home string) string {
	if !testing.Testing() || underTemp(home) {
		return home
	}
	panic("hugel: a test resolved the garden to " + home + ", which is not a temporary directory.\n" +
		"Tests must never read or write the gardener's real garden — the events log, the pile, the draw log and the marks all live there.\n" +
		"Set HUGEL_HOME to t.TempDir() in this test, or in a TestMain for the package.")
}

// underTemp reports whether p is inside a directory the operating system hands
// out for scratch space, which is where t.TempDir lives.
func underTemp(p string) bool {
	for _, root := range []string{os.TempDir(), "/tmp"} {
		if root == "" {
			continue
		}
		if within(resolve(root), resolve(p)) {
			return true
		}
	}
	return false
}

func within(root, p string) bool {
	if root == "" || p == "" {
		return false
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolve makes a path absolute and follows every symlink it can, including the
// ones above a directory that does not exist yet: a fresh t.TempDir on macOS is
// handed out under /var/folders, which is itself reached through /private, and
// comparing the two spellings as strings would refuse a garden that is in fact
// temporary.
func resolve(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	rest := ""
	for cur := p; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
