# Coding Conventions

**Analysis Date:** 2026-09-08

## Language & Runtime

**Go:** 1.26 (specified in `go.mod`)

All code is Go. No other languages in the main codebase.

## Naming Patterns

**Packages:**
- All lowercase, single word where possible
- Located under `cmd/` for command-line entry points and `internal/` for library packages
- Example packages: `tend`, `tender`, `beads`, `pile`, `compost`, `soil`

**Files:**
- All lowercase, snake_case not used; multiple words are separate files
- Test files: `*_test.go` (Go standard)
- Related functionality grouped by package, not by type
- Examples: `work.go`, `activity.go`, `progress.go` (all in `tend` package)

**Functions & Methods:**
- Exported (public): PascalCase (e.g., `Gather`, `Load`, `Save`, `Rows`, `NewGarden`)
- Unexported (private): camelCase (e.g., `workRank`, `bedHeading`, `normaliseCommand`, `isAssignment`)
- Method receivers: single lowercase letter or abbreviation matching the type
  - `func (g Garden) Rows()` — receiver `g` for `Garden`
  - `func (t Tender) Done()` — receiver `t` for `Tender`
  - `func (p Progress) Idle()` — receiver `p` for `Progress`
- Package-level functions: use full descriptive name rather than receiver pattern

**Variables:**
- Local variables: short names acceptable and preferred for loop counters and temporary values
  - `s` for session, `u` for tool use, `n` for name, `d` for draw, `e` for entry
  - `b` for bead, `t` for tally or tender or time, `p` for path or progress
- Single-letter receiver: `g` for garden, `t` for tender, `p` for progress
- Single-letter method parameters: OK for well-known patterns (e.g., `t *testing.T`)

**Types & Structs:**
- Exported: PascalCase (e.g., `Bead`, `Work`, `Tender`, `Progress`, `Activity`)
- Struct fields: PascalCase for all, exported or unexported
  - Comments documenting each field above the field name
  - Example from `internal/beads/beads.go`:
    ```go
    // Ready is bd's answer, not ours. Whether a bead can be started depends on
    // dependencies, defer dates and gates, and recomputing that here would let
    // hugel's queue drift from the one tenders actually pull from.
    Ready bool `json:"-"`
    ```

**Constants:**
- Exported: PascalCase (e.g., `NeedsAttention`)
- Values for "magic strings" are defined as constants rather than inline
- Example: `const NeedsAttention = "needs-attention"` in `internal/beads/beads.go`
- Iota for enumerations:
  ```go
  type Kind int
  const (
      Heading Kind = iota
      Drawn
      Composted
      Work
  )
  ```

**Package-Level Errors:**
- Defined at package level with `errors.New()`
- Example: `var ErrNoBd = errors.New("bd is not installed")` in `internal/beads/beads.go`

## Code Style

**Formatting:**
- Tool: `gofmt` (Go standard formatter)
- Run via: `make fmt` → `gofmt -w .`
- Indentation: tabs (Go standard)
- No configuration file; uses Go defaults

**Line Length:**
- Go standard: no hard limit enforced
- Code typically keeps lines under 100 characters for readability

**Comments:**
- Package-level doc comments: always present, multi-line encouraged
  - Placed before the `package` declaration
  - Explains package purpose and design
  - Example from `internal/tend/activity.go`:
    ```go
    // Package tend is the working surface: what the garden did lately, and the
    // judgement a gardener passes on it.
    ```

- Type/Function doc comments: present for exported items
  - Placed directly before the declaration
  - First sentence is a summary, can continue with detail
  - Example from `internal/beads/beads.go`:
    ```go
    // Labeled reports whether a bead carries a label.
    func (b Bead) Labeled(name string) bool
    ```

- Struct field comments: present for all fields (exported and unexported)
  - Placed on the line above the field
  - Multi-line when explaining design decisions
  - Example from `internal/beads/beads.go`:
    ```go
    // Ready is bd's answer, not ours. Whether a bead can be started depends on
    // dependencies, defer dates and gates, and recomputing that here would let
    // hugel's queue drift from the one tenders actually pull from.
    Ready bool `json:"-"`
    ```

- Inline comments: explain "why", not "what"
  - Placed above the code block or at end of line
  - Focus on design rationale or non-obvious behavior
  - Example from `internal/tend/activity.go`:
    ```go
    // Most recently drawn first, and an entry drawn twice is listed once: the
    // question a gardener answers about it is the same either way.
    lastDrawn := map[string]time.Time{}
    ```

- Comments on functions explain the algorithm and constraints:
  ```go
  // Gather assembles the surface. It does no IO, so what is shown can be tested
  // without a terminal or a pile.
  //
  // home names the bed the gardener is standing in, and every earlier name for
  // the same project. It orders rather than filters: with a cap on each group,
  // what is shown first is what gets judged, and the project in front of you is
  // the one you can judge.
  func Gather(entries []*pile.Entry, log []draws.Draw, ...) Activity
  ```

## Import Organization

**Order:**
1. Standard library imports (alphabetical within group)
2. External packages (alphabetical)
3. Local packages (alphabetical)

Blank lines separate the groups.

**Example from `internal/tend/work_test.go`:**
```go
import (
    "strings"
    "testing"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/charris/hugel/internal/beads"
    "github.com/charris/hugel/internal/pile"
    "github.com/charris/hugel/internal/yield"
)
```

**Path Aliases:**
- Used for long external package names for readability
- Example: `tea "github.com/charmbracelet/bubbletea"`
- Not used for local packages; full paths preferred

## Error Handling

**Strategy:** Explicit error returns, error wrapping for context

**Patterns:**

1. **Return errors explicitly:**
   ```go
   func Load() (*Config, error) {
       p, err := path()
       if err != nil {
           return nil, err
       }
       // ...
   }
   ```

2. **Wrap errors with context using `fmt.Errorf` and `%w`:**
   ```go
   if err != nil {
       return "", fmt.Errorf("locate home dir: %w", err)
   }
   return fmt.Errorf("parse %s: %w", p, err)
   ```

3. **Check for specific error types with `errors.Is()`:**
   ```go
   if errors.Is(err, exec.ErrNotFound) {
       // handle specific error
   }
   ```

4. **Extract error details with `errors.As()`:**
   ```go
   var ee *some.ExecError
   if errors.As(err, &ee) {
       // handle extracted error
   }
   ```

5. **Check for existence of files gracefully:**
   ```go
   b, err := os.ReadFile(p)
   if os.IsNotExist(err) {
       return &Config{}, nil  // missing is OK
   }
   if err != nil {
       return nil, fmt.Errorf("read config: %w", err)
   }
   ```

6. **Panic for programmer errors only:**
   ```go
   // In Sandbox() function
   defer func() {
       if r := recover(); r != nil {
           t.Fatalf("a temporary garden was refused: %v", r)
       }
   }()
   ```

## Variable & Parameter Patterns

**Multiple Return Values:**
- Functions commonly return (value, error) or (value, bool)
- Example: `func ProgressOf(t Tender, sessions []*transcript.Session) (Progress, bool)`
- Example: `func Load() (*Config, error)`

**Nil Checks:**
- Explicit nil comparison
- Example: `if s == nil || filepath.Clean(s.CWD) != want { continue }`

**Map Lookup Idiom:**
```go
byID := map[string]*pile.Entry{}
for _, e := range entries {
    byID[e.ID] = e
}
// Later:
if e := byID[id]; e != nil {
    a.Delivered = append(a.Delivered, e)
}
```

**Boolean Flags in Maps:**
```go
local := map[string]bool{}
for _, n := range home {
    local[strings.ToLower(n)] = true
}
isLocal := func(e *pile.Entry) bool { return local[strings.ToLower(e.Bed)] }
```

**String Manipulation:**
- Prefer built-in `strings` package methods
- Use `strings.CutPrefix()`, `strings.IndexByte()`, `strings.IndexAny()` for parsing
- Example: `if rest, ok := strings.CutPrefix(target, "cd "); ok { ... }`

## Function Design

**Size:**
- Functions are typically 30-50 lines for complex logic, often shorter
- Largest files are around 500-600 lines with multiple functions

**Parameters:**
- Receiver + up to 5 parameters typical
- For multiple related parameters, consider a struct
- Example: Functions passing entry lists + draw logs often accept `[]*pile.Entry, []draws.Draw`

**Return Values:**
- (value, error) is standard
- (value, bool) for "found or not found" scenarios
- Multiple return values OK when logically related

## Interfaces

**Used when:** Multiple implementations exist or behavior needs to be plugged
- Example: Store interfaces in test mocking
- Example: `tea.Model` from Bubbletea framework

**Naming:** Follows Go convention of `*er` or `*or` suffix or descriptive name
- Example: `Kind` (int type for constants), not `KindInterface`

## Module Organization

**Packages in `internal/`:**
- `beads/` — reads work from bd tracker
- `cli/` — command-line interface
- `compost/` — session transcript summarization
- `config/` — garden configuration
- `tender/` — manages work sessions
- `tend/` — the working surface (garden view)
- `pile/` — knowledge store entries
- `transcript/` — session recording
- `soil/` — pile indexing and search
- `yield/` — cost analysis
- Others: `draws`, `complete`, `redact`, `survive`, `cochange`, `pricing`, `events`, `gate`

**Packages in `cmd/`:**
- `hugel/` — main command entry point

**Organization Principle:**
- Packages are organized by domain/responsibility, not by type
- A package contains types, functions, and tests for one domain
- No separate `models/`, `utils/`, or `types/` directories

## JSON/Struct Tags

**Struct tags:** Include JSON tags for serialization
- Use `json:"fieldname"` for most fields
- Use `json:"fieldname,omitempty"` for optional fields
- Use `json:"-"` for fields that should not be serialized
- Example from `internal/beads/beads.go`:
  ```go
  type Bead struct {
      ID       string    `json:"id"`
      Title    string    `json:"title"`
      Body     string    `json:"description,omitempty"`
      Ready    bool      `json:"-"`
  }
  ```

## Output

**Standard Output:**
- Use `fmt.Print()`, `fmt.Printf()`, `fmt.Println()` for user-facing output
- Use `fmt.Sprintf()` for building strings before output
- Example from `internal/cli/pile.go`:
  ```go
  fmt.Printf("%-16s %-11s %-16s %-10s  %s\n", "ID", "TYPE", "SPIKE", "WHEN", "TITLE")
  ```

**Error Output:**
- Use `fmt.Fprintf(os.Stderr, ...)` for error messages
- Main entry point wraps errors: `fmt.Fprintf(os.Stderr, "hugel: %v\n", err)`

**No Structured Logging:**
- No logging framework (slog, logrus, etc.) is used
- Tests use `t.Logf()` for debug output
- Production code uses formatted print statements

---

*Convention analysis: 2026-09-08*
