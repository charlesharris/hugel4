# External Integrations

**Analysis Date:** 2026-09-08

## APIs & External Services

**Claude Code Session Transcripts:**
- **Purpose:** Read and analyze AI agent work sessions
- **Integration type:** File-based (reads JSONL session logs)
- **Location:** `~/.claude/projects/` (default) or `$HUGEL_TRANSCRIPT_ROOT` (override)
- **Client:** Direct JSON unmarshaling in `internal/transcript` package
- **Data format:** JSONL (JSON Lines) - one session record per line
- **Read-only:** Hugel only reads transcripts; never writes to them

## Data Storage

**File Storage:**
- Local filesystem with Dolt version control
- No external cloud storage

**Knowledge Pile (Dolt Database):**
- Location: `~/.hugel/pile/` (or `$HUGEL_PILE` override)
- Type: Dolt (Git-backed database)
- Sync mechanism: `refs/dolt/data` branch on git remote
- Format: Dolt database files (version-controlled as git commits)
- Purpose: Stores extracted knowledge from session transcripts

**Configuration Storage:**
- Location: `~/.hugel/config.json`
- Format: JSON
- Contents: Project alias mappings (Kin field)
- Read/Write: Both read and write operations

**Session Draws Log:**
- Location: `~/.hugel/draws.jsonl`
- Format: JSONL (JSON Lines)
- Purpose: Track pile queries and their accuracy (read-back when reviewed)
- Records: Query, budget, tokens spent, entries considered, entries kept

**Worktree Storage:**
- Location: `~/.hugel/beds/` (temporary)
- Type: Git worktrees
- Lifecycle: Created per tender, removed on completion
- Purpose: Isolated workspace for each work session

## Work Item Tracking

**Beads (bd) Integration:**
- **Type:** Optional external issue tracker
- **Client:** Command-line invocation via `os/exec`
- **Integration:** `internal/beads` package
- **Commands used:**
  - `bd list` - Read issues
  - `bd show` - Get issue details
  - `bd update` - Mark work as claimed
- **Requirement:** Optional - hugel works without bd, but `tender` and `gate` commands require it
- **Error handling:** Returns `ErrNoBd` if bd not installed (not a critical failure)

## Version Control

**Git Integration:**
- **Purpose:** Project management, worktree creation, commit analysis
- **Commands used:**
  - `git clone` - Create worktrees
  - `git commit` - Record changes (via tender/gate)
  - `git diff` - Analyze changes (for progress tracking)
  - `git for-each-ref` - List branches for completion
  - `git init` - Initialize beds
  - `git status` - Check file modifications
- **Client:** `os/exec` direct command invocation
- **Read/Write:** Both

## Terminal & Session Management

**Tmux Integration:**
- **Purpose:** Detached session management for autonomous tender work
- **Commands used:**
  - `tmux new-session` - Start new tender session
  - `tmux has-session` - Check session existence
  - `tmux send-keys` - Send commands to session
  - `tmux kill-session` - Cleanup
- **Client:** `os/exec` direct command invocation
- **Requirement:** Required for `tender` command, not needed for read-only commands
- **Session format:** Named tmux sessions (one per bead/tender)

## Claude Code CLI Integration

**Claude CLI Launcher:**
- **Purpose:** Launch Claude Code sessions for autonomous tenders
- **Integration:** Path lookup via `exec.LookPath("claude")`
- **Trigger:** Tender startup checks for claude binary availability
- **Purpose:** If found, starts interactive Claude Code session
- **Behavior:** Optional - work proceeds without it (uses brief file instead)

## Authentication & Identity

**Auth Provider:**
- None - This is a local CLI tool
- No API keys or credentials managed
- Works with user's local git/tmux/Claude setup

**Secret Redaction:**
- `internal/redact` package handles sensitive information
- Detects and masks:
  - Anthropic API keys (sk-ant-* format)
  - JWT tokens
  - Password/secret assignment patterns (regex-based)
  - Environment variable secrets (GITHUB_TOKEN, etc.)
- Purpose: Safe storage of session transcripts without leaking credentials

## Monitoring & Observability

**Error Tracking:**
- None - Standard Go error handling via return values
- Errors written to stderr
- Exit codes: 0 for success, 1 for errors

**Logging:**
- None - Uses `fmt.Printf` to stdout for output
- No persistent logging framework

**Session Analysis (Internal):**
- `internal/yield` package - Analyzes session costs
- `internal/tender` package - Tracks progress and file changes
- `internal/compost` package - Extracts meaningful entries from transcripts

## CI/CD & Deployment

**Hosting:**
- Single-binary CLI tool
- Installed to `~/.local/bin/hugel` (by default)
- No server component

**Build Process:**
- Local Go compilation via `go build`
- No CI/CD system required (user-driven)
- Installation via `make install`:
  - Compiles binary
  - Symlinks skills to `~/.claude/skills/`
  - Generates shell completion script

**Skills Integration:**
- Two GSD skills provided:
  - `skills/hugel-beads` - Symlinked to `~/.claude/skills/hugel-beads`
  - `skills/hugel-soil` - Symlinked to `~/.claude/skills/hugel-soil`

## Webhooks & Callbacks

**Incoming:**
- File-based "result" file: Tender writes completion status to `~/.hugel/tenders/[id].result`
- Garden monitors these files for work completion

**Outgoing:**
- Git push (when landing changes via `gate` command)
- Git commit creation (when recording work)

## Environment Configuration

**Required env vars:**
- `HUGEL_HOME` - Optional, defaults to `~/.hugel`
- `HUGEL_TRANSCRIPT_ROOT` - Optional, defaults to `~/.claude/projects`

**Optional env vars:**
- `HUGEL_PILE` - Pile storage location override
- `HUGEL_AGENT` - Agent identifier for tender sessions
- Standard system vars: `PATH`, `HOME`, `TMPDIR`, etc.

**Secrets location:**
- No secrets stored by hugel
- Reads user's existing git config (for commit attribution)
- Accesses Claude Code transcripts (which may contain redacted secrets)

## External Data Flows

### Inbound

1. **Claude Code Transcripts** → `internal/transcript`
   - Location: `~/.claude/projects/[project]/transcripts/`
   - Format: JSONL per harness session
   - Frequency: On-demand by user (hugel read operations)

2. **Beads Work Items** → `internal/beads`
   - Location: Via `bd` CLI command
   - Format: JSON output from bd
   - Frequency: On-demand per command (hugel garden, hugel tender, hugel gate)

3. **Git Repository State** → `internal/tender`, `internal/gate`
   - Location: CWD and worktrees
   - Format: Git objects and refs
   - Frequency: On-demand per session

### Outbound

1. **Pile Entries** → `~/.hugel/pile/`
   - Format: Dolt database (git-backed)
   - Trigger: `hugel compost` command
   - Destination: SYNC via `refs/dolt/data` (if remote configured)

2. **Draw Records** → `~/.hugel/draws.jsonl`
   - Format: JSONL
   - Trigger: `hugel soil` (pile query)
   - Frequency: Per query

3. **Git Commits** → Remote repository
   - Trigger: `hugel gate` command (landing phase)
   - Branch: To remote upstream (push)
   - Frequency: On completion of gated work

4. **Tender Sessions** → Tmux
   - Format: Terminal commands/input
   - Trigger: `hugel tender` command
   - Frequency: Per work item

---

*Integration audit: 2026-09-08*
