# Codebase Structure

**Analysis Date:** 2026-09-08

## Directory Layout

```
hugel4/
├── cmd/                    # Executable entry points
│   └── hugel/
│       └── main.go        # CLI entry point; delegates to internal/cli
│
├── internal/              # Application logic (Go internal package)
│   ├── beads/             # bd work tracker integration (read-only)
│   │   ├── beads.go       # Bead model, ready queue, listing
│   │   └── beads_test.go
│   │
│   ├── cli/               # Command handlers (thin layer)
│   │   ├── cli.go         # Dispatcher; routes commands
│   │   ├── pile.go        # pile init/import/list/show/review
│   │   ├── soil.go        # soil query handler
│   │   ├── yield.go       # Cost and quality reporting
│   │   ├── compost.go     # Knowledge extraction workflow
│   │   ├── digest.go      # Session digestion
│   │   ├── tender.go      # Agent execution launcher
│   │   ├── gate.go        # Review and landing
│   │   ├── spike.go       # Exploration mode
│   │   ├── bed.go         # Project listing and config
│   │   ├── garden.go      # Cross-project work overview
│   │   ├── tend.go        # Tender oversight (watch mode)
│   │   ├── dispatch.go    # Route work to ready tenders
│   │   ├── hooks.go       # Git hooks for harness integration
│   │   ├── complete.go    # Shell completion generator
│   │   └── [handlers]_test.go
│   │
│   ├── pile/              # Knowledge store (git-backed JSON)
│   │   ├── entry.go       # Entry type, Identity, Content hashing
│   │   ├── entry_test.go
│   │   ├── store.go       # Git-backed storage, Put/Get/List
│   │   ├── store_test.go
│   │   ├── review.go      # Review state machine (Unreviewed→Accepted/Rejected)
│   │   ├── review_test.go
│   │   ├── legacy.go      # Legacy entry import and upgrade
│   │   └── legacy_test.go
│   │
│   ├── soil/              # Context extraction and ranking
│   │   ├── index.go       # Full-text index and scorer (BM25)
│   │   ├── soil_test.go
│   │   ├── draw.go        # Query handler with token budgeting
│   │   └── [index_methods]
│   │
│   ├── compost/           # Session digestion and extraction
│   │   ├── digest.go      # Parse session into records (commits, beads, notes)
│   │   ├── digest_test.go
│   │   ├── extract.go     # Heuristic extractor (records→entries)
│   │   ├── extract_test.go
│   │   ├── files.go       # Changed files parsing from diffs
│   │   ├── files_test.go
│   │   ├── reverts.go     # Detect reverts in commit history
│   │   ├── reverts_test.go
│   │   ├── beads.go       # Extract bead closes from session
│   │   ├── commits.go     # Parse commit messages
│   │   ├── render.go      # Format entry content
│   │   ├── redact.go      # Apply redaction to entries
│   │   └── records_test.go
│   │
│   ├── tender/            # Agent execution (tmux + worktree)
│   │   ├── tender.go      # Tender lifecycle, run tracking
│   │   ├── tender_test.go
│   │   ├── start.go       # Brief generation and tmux launch
│   │   ├── progress.go    # Monitor running tenders
│   │   ├── progress_test.go
│   │   ├── main_test.go
│   │   └── [start_methods]
│   │
│   ├── gate/              # Review and landing
│   │   ├── gate.go        # Gate lifecycle, approval
│   │   ├── gate_test.go
│   │   ├── run.go         # Test and merge flow
│   │   ├── review.go      # Review decision recording
│   │   ├── land_test.go
│   │   └── main_test.go
│   │
│   ├── spike/             # Exploration runs (read-only agent work)
│   │   └── [impl via cli/spike.go]
│   │
│   ├── tend/              # Tender oversight (watch, judge)
│   │   ├── activity.go    # Activity data model
│   │   ├── activity_test.go
│   │   ├── model.go       # TUI model (bubbletea)
│   │   ├── model_test.go
│   │   ├── work.go        # Work row model
│   │   ├── work_test.go
│   │   └── [render and input handling]
│   │
│   ├── yield/             # Cost and quality reporting
│   │   ├── yield.go       # Aggregate session costs
│   │   ├── yield_test.go
│   │   ├── changes.go     # Attribution to beads
│   │   ├── changes_test.go
│   │   ├── soil.go        # Soil reach/precision metrics
│   │   ├── soil_test.go
│   │   ├── spikes.go      # Spike success metrics
│   │   └── spikes_test.go
│   │
│   ├── survival/          # Landing grading and revert detection
│   │   ├── survival.go    # Landing fate grading
│   │   ├── look.go        # Git querying for reverts
│   │   └── [grade and fact structures]
│   │
│   ├── events/            # Gate-generated landing log
│   │   ├── events.go      # Landing event parsing and recording
│   │   ├── events_test.go
│   │   ├── convention.go  # Event file naming and parsing
│   │   └── [event structures]
│   │
│   ├── compost/           # Session reading
│   │   └── [transcript integration; see compost/]
│   │
│   ├── transcript/        # Claude Code session transcripts
│   │   ├── transcript.go  # Load and parse transcripts
│   │   ├── transcript_test.go
│   │   ├── discover.go    # Find sessions on disk
│   │   ├── events.go      # Extract events from transcript JSON
│   │   └── testdata/      # Test fixture sessions
│   │
│   ├── config/            # Garden-wide configuration
│   │   ├── config.go      # Config model and persistence
│   │   ├── config_test.go
│   │   ├── sandbox.go     # Path sandboxing for tests
│   │   └── [KinOf logic]
│   │
│   ├── draws/             # Soil query log (separate from pile)
│   │   ├── draws.go       # Append soil queries to ~/.hugel/draws.jsonl
│   │   └── draws_test.go
│   │
│   ├── pricing/           # Token cost calculations
│   │   ├── pricing.go     # Price per model, prompt/completion split
│   │   └── pricing_test.go
│   │
│   ├── redact/            # Secret detection and filtering
│   │   ├── redact.go      # Regex patterns for secrets, API keys
│   │   └── [redaction patterns]
│   │
│   ├── cochange/          # Coupling detection (cross-file)
│   │   ├── cochange.go    # Find frequently co-changed files
│   │   └── cochange_test.go
│   │
│   ├── complete/          # Shell completion
│   │   ├── spec.go        # Completion spec model
│   │   ├── spec_test.go
│   │   ├── sources.go     # Data sources for completion
│   │   ├── sources_test.go
│   │   ├── zsh.go         # Zsh completion generation
│   │   └── zsh_test.go
│   │
│   └── survival/          # See above; grading logic
│       └── [Look and Grade functions]
│
├── skills/                # GSD skill definitions (hugel-soil, hugel-beads)
│   ├── hugel-beads/       # GSD skill: bd integration
│   └── hugel-soil/        # GSD skill: soil querying
│
├── .agents/               # Agent entrypoints
│   └── skills/            # Beads agent skill
│
├── .claude/               # Claude Code config
│   └── skills/            # Project-specific skills
│
├── .planning/             # Planning and documentation
│   └── codebase/          # Codebase maps (ARCHITECTURE.md, etc.)
│
├── .beads/                # Beads database (local Dolt DB, .gitignored)
│
├── .codex/                # Codex database (not used in core)
│
├── go.mod                 # Go module definition
├── go.sum                 # Go module lockfile
├── README.md              # Project overview
├── CLAUDE.md              # Agent instructions (this project)
└── AGENTS.md              # Agent definitions
```

## Directory Purposes

**`cmd/hugel/`:**
- Purpose: Executable entry point
- Contains: Single main.go that calls cli.Run()
- Key files: `main.go`

**`internal/cli/`:**
- Purpose: Command parsing and dispatching
- Contains: 15+ handler functions (one per command), no domain logic
- Key files: `cli.go` (dispatcher), one .go file per major command
- Pattern: Thin handlers that parse flags, call domain logic, format output

**`internal/pile/`:**
- Purpose: Git-backed knowledge store
- Contains: Entry model (types, hashing, validation), Store (git operations), Review state machine
- Key files: `entry.go` (Entry struct and methods), `store.go` (Put/Get/List), `review.go` (status transitions)
- Pattern: Immutable entries with mutable review wrapper; files are source of truth, index is disposable

**`internal/soil/`:**
- Purpose: Query and ranking engine for pile entries
- Contains: BM25 scorer, ranking algorithm, token budgeting
- Key files: `index.go` (Build index and score entries), `draw.go` (Query handler)
- Pattern: Per-query indexing (no persistent index); deterministic ranking

**`internal/compost/`:**
- Purpose: Session processing and knowledge extraction
- Contains: Digest (parse session into records), Extractor interface, Heuristic extractor, reverts detection
- Key files: `digest.go` (session→records), `extract.go` (records→entries), `reverts.go` (contradiction detection)
- Pattern: Pluggable extractors; convergent deduplication by entry identity

**`internal/tender/`:**
- Purpose: Agent execution and brief management
- Contains: Brief file generation, tmux session management, progress tracking
- Key files: `start.go` (launch flow), `tender.go` (lifecycle), `progress.go` (status)
- Pattern: Brief on disk, result on disk, no stdin/stdout piping; git worktree per bead

**`internal/gate/`:**
- Purpose: Review gatekeeping and landing
- Contains: Test orchestration, review decision recording, merge/landing flow
- Key files: `run.go` (test and merge), `review.go` (decision recording), `gate.go` (lifecycle)
- Pattern: Separate testing from review from landing; gate records events for survival grading

**`internal/tend/`:**
- Purpose: TUI for watching tenders and judging entries
- Contains: bubbletea Model, Activity data model, rendering
- Key files: `model.go` (TUI Model), `activity.go` (work/knowledge data)
- Pattern: Two-pane interface (work left, knowledge right); keypresses update pile state

**`internal/yield/`:**
- Purpose: Cost and quality reporting
- Contains: Session cost aggregation, per-bead attribution, soil metrics
- Key files: `yield.go` (session costs), `changes.go` (bead attribution), `soil.go` (reach/precision)
- Pattern: Read-only reporting; no side effects

**`internal/survival/`:**
- Purpose: Post-landing grading
- Contains: Landing fact gathering (Look), verdict computation (Grade)
- Key files: `survival.go` (verdict model and grading), `look.go` (git queries)
- Pattern: Fact/verdict separation; Look touches git/bd, Grade is pure arithmetic

**`internal/events/`:**
- Purpose: Landing event recording
- Contains: Event model, parsing, file storage
- Key files: `events.go` (event model and serialization), `convention.go` (naming)
- Pattern: Append-only; immutable facts

**`internal/transcript/`:**
- Purpose: Claude Code session reading
- Contains: Transcript loading, event extraction, path discovery
- Key files: `transcript.go` (load and parse JSON), `discover.go` (find sessions), `events.go` (extract events)
- Pattern: Read-only; transcripts from `~/.claude/projects`

**`internal/config/`:**
- Purpose: Garden-wide settings
- Contains: Config model, persistence, kin alias logic
- Key files: `config.go` (model and load/save), `sandbox.go` (path sandboxing for tests)
- Pattern: Single file `~/.hugel/config.json`; default values for missing config

**`internal/beads/`:**
- Purpose: bd work tracker integration
- Contains: Bead model (read-only wrapper), Work listing, ready queue logic
- Key files: `beads.go` (model and bd subprocess calls)
- Pattern: Read-only; hugel never writes to bd

**`internal/redact/`:**
- Purpose: Secret and credential filtering
- Contains: Regex patterns for API keys, tokens, credentials
- Key files: `redact.go` (pattern definitions and Hit recording)
- Pattern: Applied to entries before storage; audit trail recorded in Source.Redactions

**`internal/draws/`:**
- Purpose: Soil query logging (separate from pile)
- Contains: Draw model, file appending
- Key files: `draws.go` (model and append logic)
- Pattern: Append-only log at `~/.hugel/draws.jsonl`; not in pile git repo

**`internal/pricing/`:**
- Purpose: Token cost calculations
- Contains: Model pricing by region, prompt/completion tokens
- Key files: `pricing.go` (prices and cost calculation)
- Pattern: Static data; used by yield for cost attribution

**`internal/cochange/`:**
- Purpose: Coupling detection
- Contains: File co-change analysis
- Key files: `cochange.go` (analyze git log for frequently co-changed files)
- Pattern: Used by soil to boost related file entries

**`internal/complete/`:**
- Purpose: Shell completion
- Contains: Completion spec model, Zsh generator, data sources
- Key files: `spec.go` (spec model), `zsh.go` (generator), `sources.go` (dynamic data)
- Pattern: Zsh completion only; spec-based generation

## Key File Locations

**Entry Points:**
- `cmd/hugel/main.go`: Binary entry point
- `internal/cli/cli.go:Run()`: Command dispatcher

**Configuration:**
- `internal/config/config.go`: Garden settings
- `~/.hugel/config.json`: Persisted config (created on first use)
- `~/.hugel/pile/`: Pile directory (JSON entries + git repo)
- `~/.hugel/draws.jsonl`: Soil query log

**Core Logic:**
- `internal/pile/entry.go`: Entry model and validation
- `internal/pile/store.go`: Persistent storage
- `internal/soil/index.go`: Ranking algorithm
- `internal/compost/extract.go`: Extractor interface and Heuristic
- `internal/tender/start.go`: Brief generation
- `internal/gate/run.go`: Test/merge flow

**Testing:**
- `internal/*/[*_test.go]`: Unit tests (co-located with source)
- `internal/transcript/testdata/`: Fixture sessions
- `internal/compost/records_test.go`: Digest parsing tests

## Naming Conventions

**Files:**
- Package-level files: lowercase (e.g., `entry.go`, `store.go`, `tender.go`)
- Test files: `[module]_test.go` (e.g., `entry_test.go`)
- Large packages with many concerns: split into `[concern].go` files (e.g., `tender/start.go`, `tender/tender.go`, `tender/progress.go`)

**Directories:**
- Package name matches single directory (Go convention)
- Lowercase, hyphenated CLI command names in `internal/` (e.g., `compost/`, `tender/`, not `Compost/`)

**Functions/Types:**
- Exported: PascalCase (e.g., `Entry`, `Store`, `Bead`)
- Unexported: camelCase (e.g., `seedIndex`, `applyRedaction`)
- Handler functions: `run[Command]` (e.g., `runTender`, `runSoil`, `runYield`)

**Constants/Enums:**
- Exported: PascalCase (e.g., `Active`, `Unreviewed`, `Held`)
- Type names: descriptive (e.g., `type Status string`, `type Review string`)

## Where to Add New Code

**New CLI Command:**
1. Create handler in `internal/cli/[command].go` named `run[Command](args []string) error`
2. Add case to dispatcher in `internal/cli/cli.go:Run()`
3. Update usage string
4. Handlers stay thin; domain logic goes to internal packages

**New Knowledge Entry Type (pile):**
1. Add Type constant to `internal/pile/entry.go` (next to Decision, Pattern, etc.)
2. Add extraction logic to `internal/compost/extract.go` if needed
3. Entry Identity already handles convergence; no other changes needed

**New Ranking Signal (soil):**
1. Add scoring factor to `internal/soil/index.go:score()` function
2. Update `Build()` if new metadata is needed
3. Test convergence: re-running same query should rank identically

**New Extractor (compost):**
1. Implement `Extractor` interface in `internal/compost/` package
2. Name(), Version(), Extract(d *Digest) (Harvest, error)
3. Register in CLI `runCompost()` with `--extractor` flag
4. Baseline (Heuristic) must outperform on entries kept (yield --changes)

**New Transaction Type (gate/tender/spike):**
1. Create subpackage under `internal/` or in existing (e.g., `internal/spike/`)
2. Define Brief struct (instructions to agent)
3. Define Result struct (what agent wrote back)
4. Add CLI handler in `internal/cli/[command].go`
5. Follow tender/gate patterns: brief on disk, result on disk, detached execution

**New Metric (yield):**
1. Add analysis function to `internal/yield/` package
2. Compute from session events, beads, pile entries, survival verdicts
3. Return data structure; CLI handler formats output
4. Add flag to `runYield()` to trigger new metric

**New Grading Criterion (survival):**
1. Add Fact field in `internal/survival/survival.go:Fact`
2. Populate in `Look()` function (touches git/bd)
3. Add Verdict field in `Verdict` struct
4. Grade function stays pure (no side effects); compute from Fact

**New Test:**
1. Add `_test.go` file in same package as code under test
2. Table-driven tests preferred (`[]struct{}{...}`)
3. Use fixtures from `internal/transcript/testdata/` for transcripts
4. Keep global state minimal; use `t.Cleanup()` if needed

**New Supporting Package:**
1. Create `internal/[name]/` directory
2. Add exported interface/function at package level (allows testing)
3. Keep internal functions unexported
4. Add unit tests in `[name]_test.go`
5. Wire into CLI handler (in `internal/cli/`)

## Special Directories

**`.beads/`:**
- Purpose: Local Dolt database for bd (issue tracker)
- Generated: Yes (by `bd` command)
- Committed: No (in .gitignore)
- Do not touch manually; use `bd` command to interact

**`.codex/`:**
- Purpose: Codex cache (not actively used)
- Generated: Possibly (by future features)
- Committed: No (in .gitignore)

**`.planning/`:**
- Purpose: Project planning and documentation
- Contains: Codebase maps (ARCHITECTURE.md, STRUCTURE.md, etc.)
- Committed: Yes
- Do not remove; used by Claude agents for context

**`.claude/`:**
- Purpose: Claude Code project configuration
- Contains: skills/, settings.json, other harness files
- Committed: Yes

**`.agents/`:**
- Purpose: Agent skill definitions
- Contains: skills/ (beads, etc.)
- Committed: Yes

**`internal/transcript/testdata/`:**
- Purpose: Fixture Claude Code sessions for testing
- Format: JSON files (transcript structure)
- Committed: Yes
- Add new fixtures when testing new session shapes

---

*Structure analysis: 2026-09-08*
