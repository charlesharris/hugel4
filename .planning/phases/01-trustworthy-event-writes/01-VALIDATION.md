---
phase: "1"
slug: "trustworthy-event-writes"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-09-08"
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go standard library `testing` (Go 1.26) |
| **Config file** | none — `go test ./...` via `Makefile:test` |
| **Quick run command** | `go test ./internal/events/... ./internal/tender/... ./internal/gate/... ./internal/cli/...` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/events/... ./internal/tender/... ./internal/gate/... ./internal/cli/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

> Populated by the planner from PLAN.md task IDs. Rows below are the requirement-level
> contract the planner's tasks must satisfy; `{N}-01-01`-style task IDs are filled in
> once PLAN.md exists.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| TBD | 01 | 1 | SUB-01 | — | `Emit` returns non-nil error on write failure; never panics | unit | `go test -run TestEmit ./internal/events/...` | ✅ (rewrite `TestEmitCannotFailTheCaller`) | ⬜ pending |
| TBD | 01 | 1 | SUB-01 | — | Each of the 10 call sites reports to stderr but does not fail its enclosing operation | integration | `go test -run TestGate ./internal/gate/...` · `go test -run TestTender ./internal/tender/...` | ❌ W0 | ⬜ pending |
| TBD | 01 | 1 | SUB-02 | — | An event written before process exit is present after reopening the log | unit | `go test -run TestEmitIsDurable ./internal/events/...` | ❌ W0 | ⬜ pending |
| TBD | 01 | 1 | SUB-02 | — | `f.Sync()` is called and its error surfaces through `Emit`'s return | unit | `go test -run TestEmitIsDurable ./internal/events/...` | ❌ W0 | ⬜ pending |
| TBD | 01 | 2 | SUB-03 | — | Health surface distinguishes "nothing has run" from "writes have been failing" | integration | `go test -run TestHealth ./internal/cli/... ./internal/events/...` | ❌ W0 | ⬜ pending |
| TBD | 01 | 2 | SUB-03 | — | Failure-marker mtime pinned to the *first* failure; cleared on next success | unit | `go test -run TestFailureMarker ./internal/events/...` | ❌ W0 | ⬜ pending |
| TBD | 01 | 1 | SUB-01 (crit. 1) | — | Unwritable event path → failure reported by the emitting command | integration | `go test -run TestEmit ./internal/events/...` | ✅ (extends existing) | ⬜ pending |
| TBD | 01 | 1 | SUB-01 (crit. 4) | — | Gate and tender still complete when the event write fails | integration | `go test -run TestGate ./internal/gate/...` · `go test -run TestTender ./internal/tender/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/events/events_test.go` — rewrite `TestEmitCannotFailTheCaller` to assert a
      *returned error* rather than only "no panic"; add a durability test that writes, reopens
      and loads — covers SUB-01, SUB-02
- [ ] `internal/events/events_test.go` — new tests for the failure-marker create/pin/clear
      cycle — covers SUB-03
- [ ] `internal/gate/gate_test.go`, `internal/tender/tender_test.go` — new sub-tests forcing an
      event-write failure mid-run and asserting gate/tender still complete. **Highest-value
      gap**: the only test that proves the "never destroys the work it was instrumenting"
      guarantee end-to-end rather than at the `events` package boundary — covers criterion 4
- [ ] `internal/cli/yield_test.go` (confirm existence before planning) — tests for the `--health`
      surface in both the "healthy" and "failing since" cases — covers SUB-03
- [ ] Framework install: **none** — `testing` is stdlib and already in use

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Full-disk event write failure | SUB-01 | A genuinely full filesystem cannot be simulated portably in `go test`; the read-only/bad-`HUGEL_HOME` path is the automated proxy | On Linux, point `HUGEL_HOME` at a small full tmpfs mount, run `hugel gate`, confirm the failure is reported on stderr and the gate still completes |
| macOS `F_FULLFSYNC` semantics | SUB-02 | Go's `File.Sync()` does not issue `F_FULLFSYNC` on macOS (golang/go#26650); real power-loss durability is not observable from a test | Documented limitation — record the caveat in the plan rather than attempting to verify it |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
