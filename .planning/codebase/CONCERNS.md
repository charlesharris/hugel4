# Codebase Concerns

**Analysis Date:** 2026-09-08

## Tech Debt

**Instrumentation error swallowing:**
- Issue: Events logging intentionally ignores write errors to avoid breaking the instrumented code. If the event log becomes unwritable (disk full, permission denied, etc.), failures go undetected.
- Files: `internal/events/events.go:173`
- Impact: Silent data loss. Operational issues in the events.jsonl file will never surface to the user. This makes debugging production issues harder and hides scaling problems.
- Fix approach: Add health checks that periodically verify the events log can be written. Log a single warning to stderr if writing fails, keeping it non-fatal but visible.

**Concurrent pile access without synchronization:**
- Issue: The Store type in `internal/pile/store.go` has no locking mechanism. Multiple goroutines (if used in future) or parallel processes could read/write entries simultaneously, corrupting the index.
- Files: `internal/pile/store.go:41-45`, `internal/pile/store.go:198-225`
- Impact: Data corruption if concurrent access patterns emerge. Currently mitigated by single-threaded design, but fragile if architecture changes.
- Fix approach: Add a sync.RWMutex to Store and protect the load() and Put operations. Consider adding a process-level lock file if multiple processes may access the pile.

**Race condition in git commit:**
- Issue: The Commit function in `internal/pile/store.go` checks if there are staged changes (line 289) but does not prevent new changes from being written between the check and the actual commit.
- Files: `internal/pile/store.go:282-295`
- Impact: Small: a Put() call between the diff check and commit could result in changes not being committed. The pile stays consistent but a write gets lost from git history.
- Fix approach: Move the diff check into the commit operation or use git commit directly without checking.

**Manual YAML front matter parsing:**
- Issue: Legacy entry import uses hand-rolled parsing for YAML-like front matter rather than a parser. Fragile to malformed input.
- Files: `internal/pile/legacy.go:112-150`
- Impact: Malformed legacy entries could panic or silently drop fields. If legacy data is corrupted, important metadata is lost.
- Fix approach: Add explicit field validation and return errors for malformed front matter rather than silently skipping fields.

## Known Bugs

**Test suite pollution risk:**
- Symptoms: If a test forgets to set HUGEL_HOME and the sandbox check is bypassed, tests write to the real home directory pile/events/draws logs.
- Files: `internal/config/sandbox.go:32-39`, `internal/tender/main_test.go`, `internal/gate/main_test.go`
- Trigger: Add a new test that does not know about HUGEL_HOME and does not call testing.Testing() correctly. The sandbox panic would be caught by test framework.
- Workaround: The current sandbox implementation with panic catches most cases, but it relies on the testing package which could behave unexpectedly in some edge cases (Go forks, external test runners).

**Tmux session cleanup errors ignored:**
- Symptoms: If tmux is not running or a session does not exist, kill-session errors are silently ignored, leaving stray processes.
- Files: `internal/gate/review.go:162`, `internal/gate/review.go:165`, `internal/gate/review.go:182`, `internal/gate/review.go:193`
- Trigger: Run the gate with tmux unavailable or with a session that was already killed.
- Workaround: Currently mitigated because kill-session is idempotent if the session doesn't exist.

**Unreadable review produces rejection, not error:**
- Symptoms: If a review file is corrupted or a reviewer produces output the gate cannot parse, it silently rejects the work rather than surfacing the parsing error.
- Files: `internal/gate/review.go:32-43`
- Trigger: Have a reviewer output something that doesn't contain a "## Verdict" section or doesn't match the expected format.
- Workaround: The conservative behavior (reject on unreadable) is correct, but the loss of parsing error details makes debugging harder.

## Security Considerations

**Event log contains sensitive data:**
- Risk: The events.jsonl log records bead IDs, session IDs, branches, and arbitrary field values. If an events file leaks, it could reveal work in progress, branch names, and correlations.
- Files: `internal/events/events.go`
- Current mitigation: File is placed in the user's home directory (~/.hugel/events.jsonl) with standard permissions. README notes it is private.
- Recommendations: Add a warning in documentation that events logs should not be committed. Consider adding a sanitization mode that redacts sensitive fields before export.

**Pile contains composted session data:**
- Risk: The pile entries are composted from session transcripts which may contain code snippets, API responses, error messages, and internal details.
- Files: `internal/pile/entry.go`, `internal/compost/digest.go`
- Current mitigation: Entries are stored in git repository under ~/.hugel/pile which is marked private.
- Recommendations: Add warnings in compost output when sensitive keywords are detected (password, key, secret, token, credential).

**Temporary worktrees not cleaned up on failure:**
- Risk: If a tender or gate process crashes, worktrees may be left behind in ~/.hugel/beds/.
- Files: `internal/tender/start.go:116`
- Current mitigation: Worktree removal errors are ignored (by design - the failure has already happened).
- Recommendations: Add a cleanup utility command `hugel cleanup` that lists and removes stray worktrees.

## Performance Bottlenecks

**Pile index rebuilt on every query:**
- Problem: The soil package rebuilds the BM25 index every time soil is drawn, reading and parsing all entry files from disk.
- Files: `internal/soil/index.go:57-80`
- Cause: Index is not persisted; it is rebuilt on every operation to keep the source of truth in git. This is by design but could be slow with large piles.
- Improvement path: Cache the index in memory with invalidation on writes. Document the expected pile size limits (< 10k entries recommended for instant queries).

**Legacy import scans all markdown files:**
- Problem: ImportLegacyDir walks every file in the directory, checking file extensions, then parsing each markdown entry sequentially.
- Files: `internal/pile/legacy.go:68-91`
- Cause: No filtering or parallel processing. For a directory with 1000+ legacy entries, this becomes noticeable.
- Improvement path: Add a filter for .md files at the OS level and consider parallel parsing if needed.

**Tender and test output stored fully in events:**
- Problem: Tender duration, spike status, and test output are all included in event fields, which can grow large with verbose test output.
- Files: `internal/gate/run.go:48-56`
- Cause: Every field is captured as-is without truncation for observability.
- Improvement path: Truncate large fields to 1000 chars and add a --verbose flag to show full output.

## Fragile Areas

**Garden view pane switching and state preservation:**
- Files: `internal/tend/model.go:74-128`
- Why fragile: The model tracks cursor position per pane (cursors slice) and switches between work and knowledge panes. If the panes are reordered or new panes added, cursor indices could reference wrong panes.
- Safe modification: Add assertions that len(m.cursors) == len(m.panes) after any pane operation. Add tests for switching with different numbers of panes.
- Test coverage: Tests exist in `internal/tend/model_test.go` but do not cover multi-pane switching and cursor restoration.

**Gate review verdicts parsed with regex:**
- Files: `internal/gate/review.go:25`, `internal/gate/review.go:32-43`
- Why fragile: The regex `(?im)^\s*(pass|changes-needed|reject)\b` is case-insensitive and matches anywhere in the document after "## Verdict". If a reviewer's reasoning text contains "pass" before the actual verdict line, the wrong one could be matched.
- Safe modification: Tighten the regex to require the verdict section delimiter. Add a test that verifies a "findings" section mentioning "reject" does not trigger rejection.
- Test coverage: `internal/gate/gate_test.go:48-58` covers this but only with simple cases. Add a test with a complex review document.

**Compost digest truncation with hard budgets:**
- Files: `internal/compost/digest.go:28-46`
- Why fragile: Budget values are set globally (DefaultBudget) and do not account for session length or complexity. A session with very large commands or errors could hit truncation limits unexpectedly.
- Safe modification: Make budget configurable per-session and log warnings when truncation occurs. Add a --no-truncate flag for debugging.
- Test coverage: No tests verify truncation behavior or warn when limits are hit.

**Worktree cleanup with force remove:**
- Files: `internal/tender/start.go:116`
- Why fragile: Uses `git worktree remove --force` which silently ignores locked/broken worktrees. If a tender crashes mid-run, the worktree may be left in a bad state and future tenants of that bed fail.
- Safe modification: Before removing, check the worktree status and log warnings if it looks corrupted. Add a recovery command.
- Test coverage: No tests for worktree cleanup or recovery scenarios.

## Scaling Limits

**Pile git repository size:**
- Current capacity: Tested with ~200 entries; typical projects will accumulate hundreds over years.
- Limit: Git repository will grow indefinitely as entries are updated (each update is a new commit). With weekly composting, a 2-year-old pile could have 100+ commits per entry.
- Scaling path: Implement git squashing or archiving. Move old entries to a cold store after 1 year. Consider shallow clones for syncing.

**Events log file size:**
- Current capacity: Events are appended to a single file. No rotation or archiving.
- Limit: A single file with 1M+ events (months of work) becomes slow to read for reporting. No truncation or cleanup.
- Scaling path: Implement log rotation (new file per month). Add cleanup: `hugel yield --purge-before 2y` to remove old events.

**Soil search over pile:**
- Current capacity: BM25 index in memory, tested up to a few hundred entries.
- Limit: Entries are filtered by bed and review status, but no limits on returned results. A query with few terms could return 100+ results.
- Scaling path: Limit results to top K ranked entries. Add pagination or streaming for large result sets.

## Dependencies at Risk

**Dolt sync not proven:**
- Risk: The pile uses git for storage but documentation mentions Dolt for sync. Dolt integration is not yet implemented (refs/dolt/data is prepared but not used).
- Impact: Cross-machine sync of the pile currently requires manual git push/pull. Multi-user setups may diverge.
- Migration plan: Implement Dolt sync with conflict resolution. Or document the expected git workflow for multi-user piles.

**Bubbletea TUI framework tight coupling:**
- Risk: The tend command uses Bubbletea for the TUI (garden view). If Bubbletea breaks or is abandoned, the entire interactive interface becomes unmaintainable.
- Files: `internal/tend/model.go:1-11`
- Impact: Medium priority — a CLI-only fallback exists (hugel pile review) but is less ergonomic.
- Migration plan: Keep the CLI-only codepath functional and tested. If Bubbletea becomes a burden, switch to it.

## Missing Critical Features

**No pile recovery mechanism:**
- Problem: If the pile git repository becomes corrupted, there is no way to recover. A single bad commit or corrupted entry file could make the entire pile unreadable.
- Blocks: Long-term pile reliability. New projects cannot rely on hugel for knowledge they cannot recover.
- Recommendation: Add a `hugel pile verify` command that checks git history and entry files, and a `hugel pile backup` command that exports to a tarball.

**No dry-run mode for compost:**
- Problem: `hugel compost --dry-run` exists but does not show what entries will be created/updated. The user cannot preview before the pile is modified.
- Blocks: Safe testing of new extractors or changes to composting logic.
- Recommendation: `--dry-run` should show the full entry JSON that would be written, with a diff if updating.

**No manual entry creation UI:**
- Problem: Entries can only be created by compost (automated extraction). A gardener cannot add entries manually or edit existing ones with hugel commands.
- Blocks: Adding ephemeral knowledge without waiting for compost, or fixing mistakes in entries.
- Recommendation: Add `hugel pile write` to create/update entries interactively or from stdin.

## Test Coverage Gaps

**Gate test coverage incomplete:**
- What's not tested: The full flow of a merge commit (the Commit method in run.go is called in finish() but not tested as part of a passing gate run). The merge into a specific branch (--into) is not tested.
- Files: `internal/gate/run.go:160-200`, `internal/gate/gate_test.go`
- Risk: A change to merge or commit logic could pass tests but fail in production.
- Priority: High — the gate is critical infrastructure.

**Tender worktree cleanup not tested:**
- What's not tested: The removal of worktrees with --force, or recovery from a failed tender run.
- Files: `internal/tender/start.go:110-120`
- Risk: Worktrees could accumulate, filling disk space.
- Priority: Medium.

**Soil ranking not tested with real data:**
- What's not tested: BM25 ranking with varied entry content. No tests verify that relevant entries actually rank higher.
- Files: `internal/soil/index.go:140-180`
- Risk: Soil could return irrelevant results, making the pile less useful.
- Priority: Medium — the feature works but is not validated for correctness.

**Pile sync with remote not tested:**
- What's not tested: Git push/pull with the remote or detection of pile drift.
- Files: `internal/pile/store.go:270-295`
- Risk: Remote sync could fail silently or leave the local pile out of sync.
- Priority: High once multi-user piles are deployed.

---

*Concerns audit: 2026-09-08*
