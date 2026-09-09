# Project Instructions for AI Agents

This file provides instructions and context for AI coding agents working on this project.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:6cd5cc61 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->


## Build & Test

```bash
go build ./...                 # must pass for every commit
go vet ./...                   # must pass for every commit
gofmt -l internal/ cmd/        # must print nothing
go test ./...                  # 18 packages; use -count=1 when it matters
go test -race ./internal/...   # before anything touching concurrency or files
GOOS=windows go build ./...    # cross-compile is load-bearing; see writable_other.go
```

## Architecture Overview

A Go CLI (`cmd/hugel`) over `internal/` packages. The ones that matter most:

- `events` — the append-only wide-event log (`$HUGEL_HOME/events.jsonl`) and its
  health surface. One line per unit of work, carrying ids rather than counts.
- `pile` / `soil` / `compost` — knowledge composted out of finished sessions and
  drawn back into new ones.
- `gate` / `tender` / `dispatch` — running work and reaching verdicts on it.
- `yield` / `draws` / `transcript` — reading Claude Code transcripts and
  reporting what work cost.

## Conventions & Patterns

### A test must be shown to fail without the code it claims to hold

**This is the rule that catches what review misses.** After writing a test that
pins a behaviour, delete or invert the behaviour, run the test, and confirm it
fails *for the stated reason*. Restore, and say in the commit what the mutation
showed.

It is not optional, and TDD does not replace it. `01-03-SUMMARY.md` records a
genuine RED phase that passed immediately, and that is how a test asserting
`Emit`'s fsync shipped while passing with the fsync deleted. Phase 01 UAT found
two more of the same shape.

One mutation is often not enough. A test can survive the obvious mutation and
still be hollow — pinning the ordering of a check needed a second, targeted one.
Mutate toward the *plausible wrong implementation*, not just toward absence.

### Restoring after a mutation

Copy the file to the scratchpad and copy it back. **Never `git checkout --` a
file with uncommitted work** — it takes the work with it. This has already cost
one full edit this repo.

### Commits

- Commit at every green point: one logical change plus its tests, gates passing.
  Not at the end of a stretch.
- Never leave work uncommitted across a subagent run, a risky command, or a
  checkpoint. A verifier reviewing uncommitted work cannot tell a claim from a
  change.
- One bead may span several commits. One commit never spans several beads.
- Code and planning artifacts go in separate commits.
- Every commit builds and passes the gates **on its own** — check by walking the
  branch, not by assuming.
- A `Refs`/`Closes`/`Part-of <bead>` trailer is required; `.beads/hooks/commit-msg`
  enforces it. The escape hatch is `Bead: none — <reason>`, not `--no-verify`.
- Commit messages record the *evidence*: what was measured, what the mutation
  showed, what was rejected and why.

### Branching

`git.branching_strategy` is `phase`, `main` is protected. Phase work goes to
`gsd/phase-{N}-{slug}`; fixes found mid-phase go to a `fix/` branch. Land with
`--ff-only`. Never commit to `main` directly.

### GSD is the spine; beads carry what GSD finds

- **GSD owns planned forward work**: ROADMAP → phase → PLAN → SUMMARY →
  VERIFICATION. Requirements live in `.planning/REQUIREMENTS.md`, not in beads.
- **Beads own defects and chores discovered mid-phase** — what a review, a UAT
  checkpoint or a verifier turns up. File the bead, reference it from the commit,
  and name it in the phase artifact that found it.
- Don't mirror phases into beads or requirements into beads. Two trackers holding
  the same state disagree eventually.

### The GSD loop

| Situation | Command |
|---|---|
| Don't know where things stand | `/gsd-next` |
| Advance to the next step | `/gsd-progress --next` |
| Validate features against expectations (UAT) | `/gsd-verify-work <N>` |
| Re-measure whether a phase met its goal | `/gsd-verify-phase <N>` |
| Gaps fixed, phase still blocked at Gate 3 | `/gsd-verify-phase <N>`, then `--next` |

`/gsd-verify-phase` is project-local (`.claude/gsd-core/workflows/verify-phase.md`),
built because `gsd-verifier` is otherwise reachable only from inside
`/gsd-execute-phase`. Promote it to `~/.claude/` once its prompt settles.

**UAT and verification are different instruments.** UAT passing 19/19 left phase
01 at `gaps_found`. Neither substitutes for the other.

### Never self-certify

The orchestrator does not edit a verdict in `VERIFICATION.md` to reflect its own
fix. Record remediation in a separate section; the frontmatter status belongs to
the verifier. Across phase 01 the verifier caught two regressions in fixes that
had already been mutation-tested and reported as done — including one that made
the output strictly worse than `main`.

### STATE.md drifts

It is not updated by committing, and `/gsd-next` reads it. When it lags, the
situation report is wrong in a way that reads as authoritative (`0% · ready to
verify` during active remediation). Check `state_commit_stale` before trusting a
routing decision.
