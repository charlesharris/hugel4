package complete

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestSpecMatchesTheFlagsTheCLIRegisters is the whole reason this table is
// allowed to exist.
//
// The shape of the CLI is now written down twice: once where the flags are
// registered, once here so a shell can be told about them. The last time this
// project had completions they came from a library, and when the library went
// the completions went with it and nothing noticed. A second copy that nothing
// checks rots the same way, only slower.
//
// So the flags are read back out of internal/cli by walking the source, and a
// flag added on one side without the other fails the build.
func TestSpecMatchesTheFlagsTheCLIRegisters(t *testing.T) {
	registered := flagsInCLI(t)

	for _, c := range Spec {
		if c.Name == "complete" || c.Name == "completion" {
			continue // these two parse no flags
		}
		got, ok := registered[c.Name]
		if !ok {
			// A command with no flags registers none, so the walk finds no
			// file for it. That is agreement, not absence.
			if len(c.Flags) > 0 {
				t.Errorf("%s: completion offers flags but internal/cli/%s.go registers none", c.Name, c.Name)
			}
			continue
		}
		declared := map[string]bool{}
		for _, f := range c.Flags {
			declared[f.Name] = true
			if !got[f.Name] {
				t.Errorf("%s: completion offers --%s, which the command does not register", c.Name, f.Name)
			}
		}
		for name := range got {
			if !declared[name] {
				t.Errorf("%s: --%s is registered but missing from the completion table", c.Name, name)
			}
		}
	}
}

// TestEveryCommandTheCLIDispatchesIsCompletable catches the other half: a new
// command added to the switch and nowhere else completes to nothing.
func TestEveryCommandTheCLIDispatchesIsCompletable(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "cli", "cli.go"))
	if err != nil {
		t.Fatal(err)
	}
	var dispatched []string
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, `case "`)
		if !ok {
			continue
		}
		name, _, ok := strings.Cut(rest, `"`)
		if !ok || strings.HasPrefix(name, "-") || name == "help" {
			continue
		}
		dispatched = append(dispatched, name)
	}
	if len(dispatched) < 10 {
		t.Fatalf("read only %v from the dispatcher; the parse is wrong, not the code", dispatched)
	}
	for _, name := range dispatched {
		if _, ok := Find(name); !ok {
			t.Errorf("hugel %s is dispatched but has no completion", name)
		}
	}
	for _, c := range Spec {
		found := false
		for _, d := range dispatched {
			if d == c.Name {
				found = true
			}
		}
		if !found {
			t.Errorf("completion offers %q, which the dispatcher does not accept", c.Name)
		}
	}
}

// TestEveryDynamicSourceIsAnswerable stops the table naming a source For
// returns nothing for, whatever the machine's state: an unknown source and an
// empty garden look identical to a shell.
func TestEveryDynamicSourceIsAnswerable(t *testing.T) {
	known := map[string]bool{}
	for _, s := range Sources() {
		known[s] = true
	}
	check := func(where string, s Source) {
		if s.Dynamic() && !known[string(s)] {
			t.Errorf("%s completes from %q, which is not a source hugel answers", where, s)
		}
	}
	for _, c := range Spec {
		check(c.Name, c.Args)
		for _, f := range c.Flags {
			check(c.Name+" --"+f.Name, f.Arg)
		}
		for _, s := range c.Subs {
			check(c.Name+" "+s.Name, s.Args)
		}
	}
}

// flagsInCLI walks internal/cli and collects every flag each command registers,
// keyed by the file it lives in -- which is the command's name.
func flagsInCLI(t *testing.T) map[string]map[string]bool {
	t.Helper()
	dir := filepath.Join("..", "cli")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		cmd := strings.TrimSuffix(name, ".go")
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		found := map[string]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "String", "Bool", "Int", "Float64", "Duration",
				"StringVar", "BoolVar", "IntVar", "Float64Var", "DurationVar":
			default:
				return true
			}
			// A flag registration is fs.String(name, default, usage): three
			// arguments, the first a string literal. Anything else with these
			// names is some other String on some other receiver.
			if len(call.Args) != 3 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			found[strings.Trim(lit.Value, `"`)] = true
			return true
		})
		if len(found) > 0 {
			out[cmd] = found
		}
	}
	if len(out) < 10 {
		var got []string
		for k := range out {
			got = append(got, k)
		}
		sort.Strings(got)
		t.Fatalf("found flags for only %v; the walk is broken, not the code", got)
	}
	return out
}
