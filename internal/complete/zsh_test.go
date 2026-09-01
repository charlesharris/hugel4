package complete

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheGeneratedScriptIsValidZsh is the test that matters most here.
//
// Everything this file generates is quoting: descriptions carry apostrophes
// ("each stage's output"), colons ("with --grade, restrict to one bed" used
// to), and brackets, and every one of those is structure to _arguments. A
// generator that produces a script zsh cannot parse fails silently -- the
// shell simply offers nothing, which looks exactly like no completion at all.
func TestTheGeneratedScriptIsValidZsh(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("no zsh here to judge the script")
	}
	path := filepath.Join(t.TempDir(), "_hugel")
	if err := os.WriteFile(path, []byte(Zsh()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(zsh, "-n", path).CombinedOutput()
	if err != nil {
		t.Fatalf("zsh cannot parse the generated script: %v\n%s", err, out)
	}
}

// TestDescriptionsCannotEndAnOptionEarly pins the escaping rather than trusting
// that no description will ever contain a colon again.
func TestDescriptionsCannotEndAnOptionEarly(t *testing.T) {
	got := flagSpec(Flag{Name: "grade", Desc: "with --grade: a [bracket] too"})
	want := `--grade=[with --grade\: a \[bracket\] too]:value:`
	if got != want {
		t.Errorf("flagSpec escaped badly\n got %s\nwant %s", got, want)
	}
}

// TestBooleansOfferNoArgument keeps the script honest about Go's flag package,
// which will not take a bool's value from the following word.
func TestBooleansOfferNoArgument(t *testing.T) {
	got := flagSpec(Flag{Name: "dry-run", Desc: "change nothing", Bool: true})
	if strings.Contains(got, ":") {
		t.Errorf("boolean flag offers an argument: %s", got)
	}
}

// TestEveryCommandGetsACompleter guards the generated case statement, which is
// what routes a word to its flags.
func TestEveryCommandGetsACompleter(t *testing.T) {
	script := Zsh()
	for _, c := range Spec {
		if c.Name == "complete" || c.Name == "completion" {
			continue
		}
		fn := "_hugel_" + fnName(c.Name) + "()"
		if !strings.Contains(script, fn) {
			t.Errorf("no completer function for %s", c.Name)
		}
	}
	if !strings.HasPrefix(script, "#compdef hugel\n") {
		t.Error("script does not begin with its #compdef line")
	}
}

// TestTheScriptAsksTheBinaryForWhatChanges is the argument for generating a
// thin script: a source added later must not require everyone to regenerate.
func TestTheScriptAsksTheBinaryForWhatChanges(t *testing.T) {
	script := Zsh()
	if !strings.Contains(script, "hugel complete $1") {
		t.Error("the script does not call hugel complete; it has baked in what changes")
	}
	for _, s := range []Source{Ready, Tenders, Entries} {
		if !strings.Contains(script, "{_hugel_dyn "+string(s)+"}") {
			t.Errorf("nothing in the script completes from %s", s)
		}
	}
}

// TestADynamicActionIsShellCode pins the shape that took a live shell to find.
//
// _arguments reads an unbraced action as a single action word, so
// ":ready:_hugel_dyn ready" parses, loads, and completes nothing at all --
// which from the keyboard is indistinguishable from having no completion. The
// braces are what make it a command with an argument.
func TestADynamicActionIsShellCode(t *testing.T) {
	got := argSpec(Ready, "*")
	if !strings.HasSuffix(got, "{_hugel_dyn ready}") {
		t.Errorf("dynamic action is not braced shell code: %s", got)
	}
	if strings.Contains(argSpec(Dirs, ""), "_hugel_dyn") {
		t.Error("directories are the shell's job, not hugel's")
	}
}
