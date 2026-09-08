# Technology Stack

**Analysis Date:** 2026-09-08

## Languages

**Primary:**
- Go 1.26 - Core application language for CLI and all domain logic

## Runtime

**Environment:**
- Go runtime (compiled binary)
- Linux/macOS/Unix systems (uses POSIX paths and shell features)

**Package Manager:**
- Go modules
- Lockfile: `go.sum` (present)

## Frameworks

**Core:**
- Charmbracelet Bubbletea v1.3.10 - TUI (terminal UI) framework for interactive commands
- Charmbracelet Lipgloss v1.1.0 - Terminal styling and layout
- Charmbracelet X (multiple packages) - Extended Charmbracelet utilities (ANSI, terminal, cell buffering)

**Build/Dev:**
- Go standard build tools
- Makefile for build orchestration (`Makefile`)

**Testing:**
- Go standard library testing package (no external test framework)

## Key Dependencies

**Critical:**
- github.com/charmbracelet/bubbletea v1.3.10 - Terminal UI framework for interactive sessions and tenders
- github.com/charmbracelet/lipgloss v1.1.0 - Terminal styling and rendering
- github.com/charmbracelet/x/ansi v0.10.1 - ANSI color and text utilities
- github.com/charmbracelet/x/cellbuf v0.0.13 - Cell buffer for terminal rendering
- github.com/charmbracelet/x/term v0.2.1 - Terminal capability detection
- golang.org/x/sys v0.36.0 - System-level platform utilities

**Text Processing:**
- github.com/clipperhouse/uax29/v2 v2.7.0 - Unicode text segmentation for word boundaries
- github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 - ANSI string handling
- github.com/muesli/termenv v0.16.0 - Terminal environment detection

**Terminal Control:**
- github.com/mattn/go-isatty v0.0.20 - Detect if stdout is a terminal
- github.com/mattn/go-runewidth v0.0.24 - Display width of runes/Unicode characters
- github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f - Console input handling
- github.com/muesli/cancelreader v0.2.2 - Cancellable reader for terminal input
- github.com/rivo/uniseg v0.4.7 - Unicode segmentation
- github.com/lucasb-eyer/go-colorful v1.4.0 - Color manipulation
- github.com/aymanbagabas/go-osc52/v2 v2.0.1 - OSC 52 escape sequence support for clipboard

**Utilities:**
- github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e - Terminal capability information
- golang.org/x/text v0.3.8 - Unicode and text processing utilities

## Configuration

**Environment:**
- `HUGEL_HOME` - Garden home directory (defaults to `~/.hugel`)
- `HUGEL_PILE` - Knowledge pile storage location
- `HUGEL_TRANSCRIPT_ROOT` - Claude Code session transcripts root (defaults to `~/.claude/projects`)
- `HUGEL_AGENT` - Agent identifier for tender sessions

**Build:**
- `go.mod` - Module definition and dependency versions
- `go.sum` - Dependency checksums
- `Makefile` - Build, test, installation commands

**Runtime Configuration:**
- `config.json` - User configuration stored at `$HUGEL_HOME/config.json`
  - JSON format (no external config library required)
  - Contains project alias mappings (Kin field)

## Storage

**Local Filesystem:**
- Configuration: `~/.hugel/config.json` (JSON)
- Knowledge pile: `~/.hugel/pile/` (Dolt database with git-like sync)
- Session draws log: `~/.hugel/draws.jsonl` (JSONL - JSON Lines format)
- Worktrees: `~/.hugel/beds/` (temporary git worktrees)

**No Database Server:**
- No external database (SQLite, PostgreSQL, etc.)
- All state stored as local files
- Dolt used for version control of pile entries (git-based)

## External Command Integration

**Required:**
- `git` - Version control and worktree management
- `tmux` - Terminal multiplexer for detached tender sessions

**Optional:**
- `bd` - Beads issue tracker (optional; hugel works without it)
- `claude` - Claude Code CLI (checked at tender startup, used to launch sessions)

## Platform Requirements

**Development:**
- Go 1.26 or higher
- Makefile-compatible shell (bash/zsh)
- Git installed and configured
- macOS/Linux/Unix environment

**Production/Runtime:**
- Go runtime (compiled to binary)
- Git and tmux installed
- Terminal/shell environment (no GUI required)
- `~/.claude/projects/` accessible (Claude Code session home)

**Build Output:**
- Single binary: `bin/hugel` (produced by `go build`)
- Installation target: `~/.local/bin/hugel` (by default)
- Completion script: Generated at install time (`~/.local/share/zsh/site-functions/_hugel`)

## Token Efficiency Design

The project prioritizes minimal dependencies and code size:
- No ORM (uses JSON directly)
- No config parsing library (manual JSON unmarshaling)
- Single binary, no runtime dependencies beyond system tools
- All TUI rendering via Charmbracelet (not web-based, not custom)

---

*Stack analysis: 2026-09-08*
