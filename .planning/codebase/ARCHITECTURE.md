<!-- refreshed: 2026-09-08 -->
# Architecture

**Analysis Date:** 2026-09-08

## System Overview

Hugel is a single-binary CLI tool that manages agentic development work by composting session transcripts into a shared knowledge pile, drawing context from the pile for new work, and tracking the cost and quality of agent work across projects.

```text
┌──────────────────────────────────────────────────────────────────────┐
│                   Work & Quality (User-Facing)                        │
├──────────────────────┬─────────────────┬──────────────┬──────────────┤
│  Tender (`/tend`)    │  Spike (`/spike`)│ Gate (`/gate`)│ Dispatch     │
│  Work Execution      │  Exploration     │  Review      │  Routing     │
│  `internal/tender/`  │  `internal/[..]/`│ `internal/`  │ `internal/`  │
│  `internal/gate/`    │                  │  `gate/`     │  `dispatch/` │
│  in tmux + brief     │  Notes, findings │ Test,review, │              │
│                      │  land/merge flow │ test again   │              │
└──────────────────────┴─────────────────┴──────────────┴──────────────┘
                        ▲                     ▲                    ▲
                        │                     │                    │
┌──────────────────────┼─────────────────────┼────────────────────┼──┐
│   Knowledge Management & Coordination Layer                      │
├─────────────────────────────────────────────────────────────────┤
│  Pile Store → Soil (context draw) → Brief → Work → Landing    │
│  `internal/pile/` `internal/soil/`   `internal/tender/start`    │
└──────────────────────┬─────────────────────────────────────────┘
                       ▲
        ┌──────────────┴──────────────┐
        │                             │
┌───────▼───────────┐      ┌──────────▼────────────┐
│ Session Input     │      │  Accounting & Grade   │
│ (Transcripts)     │      │  (`internal/yield/`)  │
│ `internal/        │      │  (`internal/survival`)│
│  transcript/`     │      │  (`internal/pricing`) │
└───────┬───────────┘      │  (`internal/draws/`)  │
        │                  └──────────┬───────────┘
        │                             │
┌───────▼──────────────────────────────▼────────────┐
│  Digestion & Knowledge Extraction                │
│  Digest → Compost → Extract → Entries            │
│  `internal/compost/` (all logic)                 │
│  Heuristic extractor (default, free)             │
└─────────────┬──────────────────────────────────┘
              │
┌─────────────▼──────────────────────────────────┐
│  CLI Dispatcher & Command Handlers             │
│  `internal/cli/` - thin layer, no domain logic │
└──────────────────────────────────────────────────┘
        ▲
        │
    main.go
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| **CLI** | Argument parsing, flag handling, output rendering | `internal/cli/*.go` |
| **Pile** | Persistent JSON entry store with git versioning | `internal/pile/store.go`, `entry.go` |
| **Soil** | Query-to-entries ranking and token budgeting | `internal/soil/index.go`, `draw.go` |
| **Compost** | Session digestion and knowledge extraction | `internal/compost/digest.go`, `extract.go` |
| **Tender** | Brief generation, tmux session management, work execution | `internal/tender/start.go`, `tender.go` |
| **Gate** | Review gatekeeping, test orchestration, landing | `internal/gate/run.go`, `review.go` |
| **Yield** | Cost accounting and retrospective analysis | `internal/yield/yield.go`, `changes.go` |
| **Survival** | Landing grading and revert detection | `internal/survival/survival.go`, `look.go` |
| **Beads** | Read-only bd integration for work items | `internal/beads/beads.go` |
| **Config** | Garden-wide settings (kin aliases, paths) | `internal/config/config.go` |
| **Events** | Landing event recording and parsing | `internal/events/events.go` |
| **Redact** | Credential/secret filtering from compostable material | `internal/redact/redact.go` |

## Pattern Overview

**Overall:** Event-sourced, cost-conscious accounting system layered over a permaculture metaphor.

**Key Characteristics:**
- **Single binary, single database:** `hugel` binary reads/writes to local Dolt DB via bd, pile stored in git
- **No ambient state:** Entries indexed per command invocation (disposable index model)
- **Token-thrifty:** Every operation is instrumented; cost per accepted change is visible
- **Principle-driven:** Entries, sessions, landings are immutable facts; judgements (review, status) are mutable wrapping
- **Work-tracking-agnostic:** Hugel reads bd but never writes it; bd is source of truth for work

## Layers

**CLI Layer:**
- Purpose: Parse arguments, delegate to domain logic, format output
- Location: `internal/cli/`
- Contains: Command handlers for yield, digest, compost, pile, soil, bed, garden, tend, tender, spike, gate, dispatch, hooks, completion
- Depends on: All internal packages (routing layer only)
- Used by: `cmd/hugel/main.go`
- Pattern: Thin handlers; no domain logic

**Pile Layer:**
- Purpose: Persistent storage and versioning of knowledge entries
- Location: `internal/pile/`
- Contains: Entry types (Decision, Pattern, Discovery, Failure, Constraint), Store (git-backed), Review state machine
- Depends on: config, redact
- Used by: compost (writes), soil (reads), tend (reads/writes reviews), pile CLI (user commands)
- Pattern: Files-first with git; index is derived and disposable

**Knowledge Extraction Layer:**
- Purpose: Convert session transcripts into composted entries
- Location: `internal/compost/`
- Contains: Digest (session snapshot), Extract (transcript→entries), Heuristic extractor, Reverts analyzer
- Depends on: pile, redact, events
- Used by: compost CLI, yield CLI
- Pattern: Pluggable extractors; heuristic (free) is baseline

**Context Selection Layer:**
- Purpose: Rank and select pile entries for a specific query within a token budget
- Location: `internal/soil/`
- Contains: Index (full-text + metadata), Draw (query handler), scoring algorithm
- Depends on: pile, config, cochange (cross-file coupling)
- Used by: soil CLI, tender/spike (brief generation)
- Pattern: Per-query indexing, token-budgeted delivery

**Transaction Execution Layer:**
- Purpose: Run work unattended (tender), explore questions (spike), gate changes (gate)
- Location: `internal/tender/`, `internal/spike/`, `internal/gate/`
- Contains: Brief generation, tmux management, test orchestration, landing flow
- Depends on: soil (context), beads (work items), transcript (session tracking), events (landing recording)
- Used by: tender/spike/gate CLI, dispatch (routing)
- Pattern: Detached tmux sessions, git worktrees per bead, state stored on disk

**Accounting Layer:**
- Purpose: Track and report costs: context tax, per-change cost, soil reach/precision
- Location: `internal/yield/`, `internal/pricing/`, `internal/draws/`
- Contains: Session cost aggregation, change attribution, draw recording
- Depends on: transcript, beads, pile, events
- Used by: yield CLI
- Pattern: Append-only logs, derived metrics

**Quality Gate Layer:**
- Purpose: Grade approved changes on survival; detect reverts
- Location: `internal/survival/`
- Contains: Landing fact gathering (Look), verdict grading (Grade)
- Depends on: events, git
- Used by: yield CLI (survival reporting)
- Pattern: Fact/verdict separation; no side effects

**Work Coordination Layer:**
- Purpose: Interface with bd issue tracker; route work to tenders
- Location: `internal/beads/`
- Contains: Bead model (read-only wrapper), Tally (counts by status)
- Depends on: bd (external subprocess)
- Used by: garden CLI, tender/gate/spike (finding work), dispatch (routing)
- Pattern: Read-only; hugel never modifies beads

## Data Flow

### Primary Request Path (Tender Execution)

1. Gardener runs `hugel tender <bead>` → `internal/cli/tender.go:startTender`
2. Find bead in bd via `internal/beads/beads.go:List`
3. Compose brief:
   - Load session index from `~/.claude/projects` → `internal/transcript/`
   - Query pile for context → `internal/soil/` (ranked entries within token budget)
   - Format brief file with instructions
4. Create git worktree and fire tmux session running Claude with brief
5. Agent reads brief, runs in worktree, writes result file
6. When done or timed out, collect result → `internal/tender/tender.go:Finish`
7. Tender commits to branch, stops (does not push/merge)

### Composting Path (Knowledge Acquisition)

1. Session ends; gardener runs `hugel digest --session <ID>` → `internal/cli/digest.go`
2. Load session transcript from `~/.claude/projects` → `internal/transcript/transcript.go`
3. Extract records (commits, bead closes, notes) → `internal/compost/digest.go`
4. Run through extractor (Heuristic by default) → `internal/compost/extract.go`
   - Extract entries from commits, bead closes
   - Group by identity (bed + type + normalized title → same entry)
   - Detect and record reverts as Contradicts links
5. Store entries to pile → `internal/pile/store.go:Put` (converges duplicates)
6. Pile auto-commits each write; re-composting is idempotent

### Landing & Grading Path (Quality Assurance)

1. Gate approves and lands a change → commits recorded in `internal/events/`
2. On interval (yield CLI), look back:
   - Parse landing events → `internal/survival/survival.go:Grade`
   - Query git for reverts → `internal/survival/look.go:Look`
   - Match reverts to landings via commit SHA
   - Assign fate: Held, Reverted, Reopened, or Young
3. Grade reviewers, briefs, soil, tender — all touch the same verdict
4. Report to yield output

### Soil Draw Path (Context Delivery)

1. Tender/spike/soil CLI queries for context → `internal/cli/soil.go:runSoil`
2. Build index from pile entries → `internal/soil/index.go:Build`
3. Score entries by relevance (BM25 + metadata boost + bed proximity)
4. Rank by score; select top-N within token budget → `internal/soil/soil.go:Draw`
5. Record draw to `~/.hugel/draws.jsonl` → `internal/draws/draws.go:Append`
   - Draw is separate from pile (doesn't dirty repo on read)
   - Soil reach and precision computed from draws at report time
6. Return formatted entries or JSON

**State Management:**
- **Pile:** Source of truth; git repo with JSON files per entry, indexed by identity
- **Draws:** Append-only log of soil queries; lives beside pile, not in it
- **Beads:** Read from bd (external); never written by hugel
- **Events:** Append-only log of gate-generated landings and reverts
- **Tenders:** State on disk (brief file, result file) and in tmux session names
- **Config:** Single `~/.hugel/config.json` with project kin aliases

## Key Abstractions

**Entry:**
- Purpose: One piece of composted knowledge
- Types: Decision, Pattern, Discovery, Failure, Constraint
- Scope: Bed (project-local) or General (shared)
- Status: Active, Superseded, Abandoned
- Review: Unreviewed, Accepted, Rejected
- Identity: Deterministic hash of (bed, type, title) → converges duplicates
- ContentHash: Detects edits; re-composting doesn't rewrite unchanged entries
- Examples: `internal/pile/entry.go`
- Pattern: Immutable facts with mutable review wrapping; git versioning

**Soil (Context):**
- Purpose: Ranked selection from pile for a query
- Contains: Query (text, bed, type filter, budget), Items (scored entries), Tokens (spent/considered)
- Ranking: BM25 term matching, metadata boost, bed proximity
- Budget: Tokens in full entry text; delivery stops at limit
- Examples: `internal/soil/soil.go`
- Pattern: Token-aware delivery; separate concern from pile

**Bead (Work Item):**
- Purpose: One task from bd issue tracker
- Read-only; Hugel never modifies
- Status: open, in_progress, done
- Ready: Derived from dependencies; blocks ready queue until met
- Blocked: Open but not ready (dependencies or deferred)
- Labels: NeedsAttention flags work for humans, not tenders
- Examples: `internal/beads/beads.go`
- Pattern: Read-through wrapper for bd output

**Brief (Tender Input):**
- Purpose: Instruction file for an agent; stored on disk
- Contains: Task description, soil (ranked entries), instructions, git branch name
- Attached: Set to true if a person will watch; tells agent to ask rather than guess
- Examples: `internal/tender/start.go:writeBrief`
- Pattern: Deterministic generation; rereadable for debugging

**Landing (Gate Output):**
- Purpose: Record that gate approved and landed a change
- Contains: Bead ID, SHA (commit or worktree HEAD), base (branch before merge), branch, timestamp
- Immutable fact; never modified once recorded
- Examples: `internal/survival/survival.go:Landing`
- Pattern: Append-only event log

**Verdict (Survival Grade):**
- Purpose: Outcome of a landing in the weeks after
- Fate: Held (survived), Reverted (taken back), Reopened (bead reopened), Young (too new)
- Why: Human reason (if revert had commit message explanation)
- Age: How long until reverted, or current age if held
- Found: Related beads opened after landing (reported, not counted as failure)
- Examples: `internal/survival/survival.go:Verdict`
- Pattern: Fact-based grading; no feedback loops (yet)

**Harvest (Extraction Output):**
- Purpose: Entries and cost from one extraction run
- Contains: []*Entry (entries), CostUSD (tokens spent)
- Convergence: Re-extracting same session, same entries de-duplicate by identity
- Examples: `internal/compost/extract.go:Harvest`
- Pattern: Pluggable extractor interface

## Entry Points

**Main Entry:**
- Location: `cmd/hugel/main.go`
- Triggers: `hugel` command
- Responsibilities: Parse args, call cli.Run

**CLI Dispatcher:**
- Location: `internal/cli/cli.go:Run`
- Triggers: Command keyword (yield, digest, compost, pile, soil, etc.)
- Responsibilities: Route to appropriate runXxx handler

**Tender Executor:**
- Location: `internal/cli/tender.go:startTender`
- Triggers: `hugel tender <bead>`
- Responsibilities: Brief generation, tmux launch, result collection

**Gate Reviewer:**
- Location: `internal/cli/gate.go:runGate`
- Triggers: `hugel gate <bead>`
- Responsibilities: Test orchestration, review, merge/land flow

**Composition Loop:**
- Location: `internal/cli/compost.go:runCompost`
- Triggers: `hugel compost --all` or `--session`
- Responsibilities: Find sessions, digest, extract, store entries

## Architectural Constraints

- **Threading:** Single-threaded event loop; no goroutines in CLI paths. Tender/gate are external tmux sessions (not goroutines)
- **Global state:** None. Config loaded per command; index rebuilt per query. Pile is external git repo.
- **Circular imports:** None detected (Go compiler enforces at build time)
- **External dependencies:** bd (work tracker), git (pile storage, landing history), tmux (tender/gate execution), Claude Code API (via tender/gate)
- **Git model:** Pile is git-backed; tender/gate use worktrees; events written to git (via events package)
- **Database:** Hugel itself is stateless; bd and pile are the databases. Transcripts read from `~/.claude/projects`.

## Anti-Patterns

### Pile mutation during read

**What happens:** Soil draw would dirty the pile repository every time context is requested, because tracking "this entry was accessed" requires a write.

**Why it's wrong:** Session context loading would cause unintended commits; every soil invocation would leave uncommitted changes.

**Do this instead:** Draws are recorded separately in `~/.hugel/draws.jsonl` (`internal/draws/`), outside the pile git repo. Reach/precision computed at report time from draw log, not from pile state.

### Duplicate entries after re-composting

**What happens:** Re-digesting the same session twice would create two Entry objects with different IDs.

**Why it's wrong:** Knowledge would split across duplicate entries; a reviewer accepting one wouldn't affect the other.

**Do this instead:** Entry Identity is deterministic (`internal/pile/entry.go:Identity()`) based on bed, type, and normalized title. Put() converges duplicates by identity; unchanged entries return Unchanged status without rewriting.

### Tender/gate side effects before landing decision

**What happens:** A tender could commit work, push, and merge before gate approval.

**Why it's wrong:** Gate couldn't block bad work; approval wouldn't mean anything.

**Do this instead:** Tender commits to its own branch and stops (`internal/tender/tender.go:Finish`). Gate takes the branch, tests it again, reviews it, then decides to land or reject. Gate writes landing events (`internal/events/`) for survival grading.

### Hugel modifying beads in bd

**What happens:** Hugel could change bead status, assignee, or labels to coordinate work.

**Why it's wrong:** Two systems would both modify beads; they'd diverge. Huge source of confusion.

**Do this instead:** Hugel reads beads only (`internal/beads/`). To change a bead, the user runs `bd` directly. Hugel's coordination is implicit (tender queues pull from bd ready list; gate lands changes; yield reports outcomes).

### Extractor inventing knowledge from agent notes

**What happens:** A model-backed extractor could infer patterns from agent reasoning prose.

**Why it's wrong:** Inferred entries are wrong more often than they're right; keeping wrong permanent knowledge is worse than missing knowledge.

**Do this instead:** Heuristic extractor (`internal/compost/extract.go:Heuristic`) only reads commit messages and bead close reasons—things someone already chose to record formally. Widen extraction only with evidence that entries are kept (yield --changes + pile review history).

## Error Handling

**Strategy:** Fail fast on configuration/permission errors; continue on data errors.

**Patterns:**
- Configuration missing (e.g., HUGEL_HOME, HUGEL_PILE): Return error immediately
- Entry validation fails: Log entry ID, return error, stop that write; don't corrupt batch
- Git operation fails: Return error with context (repo path, command attempted)
- bd not installed: Return ErrNoBd; don't fail the whole session if work tracking isn't set up
- Tender/gate crashes: Result file missing; gate reports incomplete, doesn't blind-land
- Pile missing: Return ErrNoPile; user runs `hugel pile init`

## Cross-Cutting Concerns

**Logging:** Stderr for errors, Stdout for results. No structured logging framework; simple fmt.Fprintf/fmt.Print per command.

**Validation:** Entry.Validate() before any write to pile. Bead.Blocked() before queueing. Brief sanity checks before tender launch.

**Authentication:** Hugel doesn't authenticate. It assumes the user running `hugel` has access to the pile git repo, bd, and transcript directory. Pile is private by design (composted from sessions).

**Redaction:** Secrets detected in transcripts/entries via `internal/redact/` patterns (API keys, tokens, etc.). Redacted fields recorded in entry Source as audit trail.

---

*Architecture analysis: 2026-09-08*
