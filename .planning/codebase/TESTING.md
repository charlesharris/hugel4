# Testing Patterns

**Analysis Date:** 2026-09-08

## Test Framework

**Runner:**
- `go test` (Go standard library testing)
- Version: Go 1.26

**Assertion Library:**
- Go's built-in `*testing.T` (no external assertion library)

**Run Commands:**
```bash
make test              # Run all tests: go test ./...
make vet               # Lint code: go vet ./...
make fmt               # Format code: gofmt -w .
```

**Individual test runs:**
```bash
go test ./internal/tend/...
go test -run TestWorkOrdersByWhatItNeeds ./internal/tend/...
go test -v ./...       # Verbose output
go test -count=1 ./... # Ignore test cache
```

## Test File Organization

**Location:**
- Co-located with source code in the same package
- Test file naming: `*_test.go` following Go standard

**Naming:**
- Test files placed in same directory and package as code being tested
- Example: `work.go` and `work_test.go` both in `internal/tend/`

**Structure:**
```
internal/tend/
├── activity.go
├── activity_test.go
├── work.go
├── work_test.go
├── model.go
└── model_test.go
```

## Test Structure

**Basic Test Format:**

```go
func TestDescriptiveTestName(t *testing.T) {
    // Setup
    
    // Execute
    
    // Assert
}
```

**Example from `internal/tend/work_test.go`:**
```go
func TestWorkOrdersByWhatItNeeds(t *testing.T) {
    // Setup: create test data
    g := Garden{Beds: []*beads.Work{work("hugel4",
        bead("c", "blocked one", "open", false),
        bead("b", "ready one", "open", true),
        bead("a", "in flight", "in_progress", true),
    )}}
    
    // Execute
    rows := g.Rows(0)
    
    // Assert
    var got []string
    for _, r := range rows {
        if r.Kind == Work {
            got = append(got, r.Bead.ID)
        }
    }
    if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
        t.Errorf("order = %v, want in-flight, ready, blocked", got)
    }
}
```

**Naming Convention:**
- Test functions: `Test` + description of what is being tested
- Names are descriptive (e.g., `TestWorkOrdersByWhatItNeeds`, `TestKinIsSymmetric`, `TestMissingConfigIsEmptyNotAnError`)
- Many tests include comments explaining the design principle being verified

**Comments in Tests:**
- Tests include a comment above the function explaining what is being tested
- Comments are narrative, explaining "why" from a design perspective
- Example from `internal/tend/work_test.go`:
  ```go
  // The order is the question a gardener is asking: what is half-finished and
  // waiting on me, what could be started, and what cannot move.
  func TestWorkOrdersByWhatItNeeds(t *testing.T)
  ```

## Test Patterns

### Helper Functions

**Purpose:** Reduce boilerplate in test setup

**Pattern:**
```go
func helperName(t *testing.T) returnType {
    t.Helper()  // Mark this function as a helper
    // Setup code
    return value
}
```

**Examples from `internal/tend/work_test.go`:**
```go
func bead(id, title, status string, ready bool) beads.Bead {
    return beads.Bead{ID: id, Title: title, Status: status, Ready: ready, Type: "task"}
}

func work(bed string, bs ...beads.Bead) *beads.Work {
    return &beads.Work{Bed: bed, Dir: "/tmp/" + bed, Beads: bs}
}

func gardenModel(t *testing.T) Model {
    t.Helper()
    es := []*pile.Entry{ent("k1", "some knowledge", -60, "s1")}
    g := Garden{Beds: []*beads.Work{work("hugel4",
        bead("hugel4-1", "in flight here", "in_progress", true),
        bead("hugel4-2", "ready here", "open", true),
    )}}
    // ... more setup
    return next.(Model)
}
```

### Table-Driven Tests

**When Used:** Testing multiple input/output combinations

**Pattern:**
```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name string  // Test case name
        in   string  // Input
        want string  // Expected output
    }{
        {"case1", "input1", "expected1"},
        {"case2", "input2", "expected2"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := functionUnderTest(tt.in)
            if got != tt.want {
                t.Errorf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

**Example from `internal/compost/digest_test.go`:**
```go
func TestNormaliseCommand(t *testing.T) {
    tests := []struct{ name, in, want string }{
        {"bare command", "go test ./...", "go test ./..."},
        {"cd with &&", "cd /src/hellbox && git status", "git status"},
        {"cd with semicolon", "cd /src/hellbox; git status", "git status"},
        // ... more test cases
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := normaliseCommand(tt.in); got != tt.want {
                t.Errorf("normaliseCommand(%q)\n got %q\nwant %q", tt.in, got, tt.want)
            }
        })
    }
}
```

**Example from `internal/redact/redact_test.go`:**
```go
func TestFormatDetectors(t *testing.T) {
    tests := []struct {
        name, in string
        class    Class
    }{
        {"anthropic", "key is sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFFGGGG here", ClassAnthropicKey},
        {"github pat", "token ghp_AbCdEf0123456789AbCdEf0123456789 ok", ClassGitHubToken},
        // ... more cases
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
            // ... more assertions
        })
    }
}
```

## Assertion Patterns

**Error Checking:**
```go
if err != nil {
    t.Fatal(err)     // Stop test immediately
}
if err != nil {
    t.Fatalf("Load: %v", err)  // Stop test with formatted message
}
if err != nil {
    t.Error(err)     // Continue test, mark as failed
}
if err != nil {
    t.Errorf("got %d, want %d", got, want)  // Continue test with formatted message
}
```

**Comparison:**
```go
if len(got) != 3 {
    t.Errorf("length = %d, want 3", len(got))
}
if got[0] != "a" {
    t.Error("order is wrong")
}
if !strings.Contains(heading, "5 of 30") {
    t.Errorf("heading %q should say what was left out", heading)
}
```

**Nil Checks:**
```go
if rows[0].Kind != Heading || !strings.Contains(rows[0].Label, "no bed") {
    t.Errorf("rows = %+v, want one heading saying nothing is tracked", rows)
}
```

**Panic Testing:**
```go
defer func() {
    r := recover()
    if r == nil {
        t.Fatal("Home returned the gardener's real garden to a test")
    }
    if msg, _ := r.(string); !strings.Contains(msg, "HUGEL_HOME") {
        t.Errorf("the refusal does not say how to fix it: %v", r)
    }
}()
Home()
```

## Setup & Teardown

**Environment Variables:**
```go
// Set for this test only
t.Setenv("HUGEL_HOME", t.TempDir())

// After test completes, Setenv restores the original
```

**Temporary Directories:**
```go
// Each test gets a clean temporary directory
tmpDir := t.TempDir()

// Files written here are automatically cleaned up after test
os.WriteFile(filepath.Join(tmpDir, "file.txt"), data, 0o644)
```

**Cleanup Functions:**
```go
// Register cleanup code that runs after test
old := now
now = func() time.Time { return when }
t.Cleanup(func() { now = old })

// Pattern from internal/tender/tender_test.go
func fixedNow(t *testing.T) time.Time {
    t.Helper()
    when := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
    old := now
    now = func() time.Time { return when }
    t.Cleanup(func() { now = old })
    return when
}
```

## Mocking

**Approach:** Hand-written mocks and test doubles

**No Mock Framework:** Tests don't use `github.com/golang/mock` or similar

**Test Doubles Pattern:**
```go
type fakeStore struct {
    // embed real type or define fields
    commits []string
}

func (f *fakeStore) MethodName() error {
    // Test implementation
    return nil
}

func TestUsingMock(t *testing.T) {
    f := &fakeStore{}
    // Use f as if it's the real store
    if len(f.commits) != 0 {
        t.Error("unexpected commits")
    }
}
```

**What to Mock:**
- External dependencies (file systems via `t.TempDir()`)
- External services (not done in this codebase; prefer integration tests)
- Store/persistence layer when testing logic

**What NOT to Mock:**
- Standard library functions
- Local types defined in the same package
- Behavior that should be tested end-to-end

## Test Types

**Unit Tests:**
- Scope: Single function or method
- Approach: Direct function call with setup, no external I/O except temporary files
- Example: `TestWorkOrdersByWhatItNeeds` tests the `Garden.Rows()` method
- Example: `TestNormaliseCommand` tests string normalization function

**Integration Tests:**
- Scope: Multiple related functions working together
- Approach: Create fixtures, verify behavior across component boundaries
- Example: `TestRoundTrip` in `internal/config/config_test.go` — writes config, reads it back
- Example: `TestAppendAndLoad` in `internal/draws/draws_test.go` — appends draws, loads them

**End-to-End Tests:**
- Not heavily used in this codebase
- Some integration tests serve this purpose (e.g., transcript loading)

**Performance Tests:**
- Not observed in the codebase
- Marked with `Benchmark` prefix if added

## Coverage

**Requirements:** None enforced (no coverage gate in CI)

**Pragmatic Approach:**
- Critical paths are tested
- Error handling is tested
- Edge cases are tested
- Not all branches have explicit test coverage

**Types of Coverage Found:**
- Happy path: Most functions have at least one passing test
- Error paths: Error conditions explicitly tested
  - Example: `TestMissingConfigIsEmptyNotAnError` in `internal/config/config_test.go`
  - Example: `TestCorruptLineIsSkipped` in `internal/draws/draws_test.go`
- Edge cases: Boundary conditions
  - Example: `TestEmptyGardenSaysSo` in `internal/tend/work_test.go`

## Fixture & Test Data

**Test Data Creation:**
- Helper functions create test data inline
- No separate fixture files
- Example from `internal/tend/work_test.go`:
  ```go
  func bead(id, title, status string, ready bool) beads.Bead {
      return beads.Bead{ID: id, Title: title, Status: status, Ready: ready, Type: "task"}
  }
  ```

**Temporary Files:**
- Use `t.TempDir()` for file-based tests
- Example from `internal/config/config_test.go`:
  ```go
  func TestRoundTrip(t *testing.T) {
      home := t.TempDir()
      t.Setenv("HUGEL_HOME", home)
      
      c := &Config{}
      c.AddKin("hugel4", "hugel")
      if err := Save(c); err != nil {
          t.Fatal(err)
      }
      // ... more test
  }
  ```

**Fixture Location:**
- No shared fixture directory; fixtures are created per-test or via helpers
- Reduces maintenance burden and makes tests self-contained

## Common Test Patterns

### Async Testing

**Pattern:** Use callbacks or channels to verify async behavior

**Not heavily used** in this codebase; most code is synchronous

### State Mutation Testing

**Pattern:** Verify that function calls properly modify state

**Example from `internal/tend/work_test.go`:**
```go
func TestPanesKeepTheirPlace(t *testing.T) {
    m := gardenModel(t)
    m = press(m, "j")
    where := m.cursor
    m = press(m, "tab", "tab")
    if m.cursor != where {
        t.Errorf("cursor came back at %d, want %d", m.cursor, where)
    }
}
```

### Idempotency Testing

**Pattern:** Run operation twice, verify same result

**Example from `internal/redact/redact_test.go`:**
```go
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
```

### Round-Trip Testing

**Pattern:** Serialize, deserialize, verify same result

**Example from `internal/config/config_test.go`:**
```go
func TestRoundTrip(t *testing.T) {
    home := t.TempDir()
    t.Setenv("HUGEL_HOME", home)
    
    c := &Config{}
    c.AddKin("hugel4", "hugel")
    if err := Save(c); err != nil {
        t.Fatal(err)
    }
    
    back, err := Load()
    if err != nil {
        t.Fatal(err)
    }
    if len(back.KinOf("hugel")) != 2 {
        t.Errorf("round trip lost kinship: %v", back.KinOf("hugel"))
    }
}
```

### Invariant Testing

**Pattern:** Verify that an operation maintains an invariant

**Example from `internal/config/config_test.go`:**
```go
// Kinship is symmetric: asking from any name gives the whole group
func TestKinIsSymmetric(t *testing.T) {
    c := &Config{}
    c.AddKin("hugel4", "hugel", "hugel-core")
    
    for _, from := range []string{"hugel4", "hugel", "hugel-core"} {
        got := c.KinOf(from)
        if len(got) != 3 {
            t.Errorf("KinOf(%q) = %v, want all three names", from, got)
        }
    }
}
```

## Test Execution

**Parallel Tests:**
- `t.Parallel()` is NOT used
- Tests run sequentially by default
- Safe for tests that use `t.Setenv()` or write to shared temp directories

**Test Isolation:**
- Each test gets its own temp directory via `t.TempDir()`
- Environment variables are isolated per-test via `t.Setenv()`
- No global state shared between tests

**Verbose Output:**
```bash
go test -v ./...  # Shows each test as it runs
```

**Run Single Test:**
```bash
go test -run TestWorkOrdersByWhatItNeeds ./internal/tend/...
```

---

*Testing analysis: 2026-09-08*
