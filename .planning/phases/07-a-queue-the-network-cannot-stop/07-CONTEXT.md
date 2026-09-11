# Phase 7: A Queue The Network Cannot Stop - Context

**Gathered:** 2026-09-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Writes are accepted, fsynced and acknowledged on the gardener's own disk before
any database hears about them, and drain when it is reachable. A stall is named
rather than silent. None of it is testable against a real garden.

This phase introduces `internal/store` (pool, `goose` migrations, `Tx` helper,
the AGE `AfterConnect` session plumbing) and `internal/queue` as its peer, plus
the schema-per-run test harness. Those are enabling work *inside* this phase,
not phases of their own.

**Not in this phase:** converting `events`, `draws` or `pile` to read or write
through the store. Phase 7 builds the road; Phases 8 and 9 drive on it. The one
exception is what the queue's wire format must carry on day one, because an
identity retrofitted onto rows already written is a second migration.
</domain>

<decisions>
## Implementation Decisions

### Queue form

- **D-01:** One append-only JSONL queue at a machine-local path, each line a typed
  envelope — `kind`, `uuid`, `actor`, `machine`, `payload`. It reuses phase 01's
  fsync-before-return primitive wholesale rather than inventing a second durable
  write. — **Reversibility:** costly — changing the envelope shape after records
  exist means draining the old format before the new code can run, so the change
  has to ship with a reader for both.
- **D-02:** The payload holds **domain objects, not pre-shaped SQL rows.** Mapping
  to SQL happens at drain time. A `goose` migration run while a backlog exists
  must not invalidate that backlog, which pre-shaped rows would. —
  **Reversibility:** costly — the drain mapper is the only thing that knows both
  shapes; moving the mapping earlier would require re-encoding every queued record.
- **D-03:** One queue file for all record kinds, not one per kind. UUIDv7 already
  carries global creation order, so per-kind files buy isolation of a poisoned
  record at the cost of multiplying drain, health and the read overlay by kind.

### Drain

- **D-04:** Opportunistic on any invocation, with a **sub-second dial timeout** and
  a **backoff marker**: if the last attempt failed less than N minutes ago, skip
  without dialling at all. The bounded worst case is what makes this safe on
  `hugel soil`, which is called mid-session precisely because it is cheap.
- **D-05:** The backoff marker is the same shape as phase 01's
  `events.failing-since` — a zero-byte file whose mtime is the whole payload,
  first-write-wins. It therefore also answers QUEUE-04's "when did it stop"
  without a second mechanism.

### Reads

- **D-06:** **The store layer merges; readers are unchanged.** `store.Events()` /
  `store.Draws()` return database rows merged with undrained queue entries. The
  four draws readers (`internal/cli/tend.go`, `garden.go`, `yield.go` x2) and the
  events readers need no knowledge of the queue. — **Reversibility:** costly —
  pushing the merge out to call sites later means auditing every reader, which is
  the shape phase 01 had to fix with a ten-of-ten grep.
- **D-07:** The merge **dedups by UUIDv7** and orders by it. At-least-once delivery
  means a record can be in the queue *and* already drained; without dedup the
  gardener sees doubles.

### Machine and actor identity

- **D-08:** A machine is named by a **UUID minted on first use**, written to a
  machine-local file that deliberately does not migrate to Postgres. It is stable
  across renames, network changes and a garden migration. — **Reversibility:**
  one-way — once rows carry a machine id, changing how it is derived orphans the
  provenance of everything already written; a later change needs a mapping table,
  not an edit.
- **D-09:** An **optional human label** from config (`laptop`, `desktop`) rides
  alongside the id so queries read in words rather than hex. Absent label still
  works; the id is the key and the label is decoration.
- **D-10:** **Machine-local state is a new category this phase introduces.**
  Everything else under `$HUGEL_HOME` is moving into Postgres, but an identifier
  for *this machine* cannot live in the shared store and remain meaningful. The
  queue file itself is in the same category.
- **D-11:** `actor` and `machine` are **two columns, not one.** They collapse to a
  single gardener today; they are separate because the milestone carries an actor
  id specifically so team use is configuration rather than a schema rewrite.

### Claude's Discretion

- The backoff interval N, the dial timeout value, and the queue file's exact path
  and name.
- Whether the drain mapper lives in `internal/queue` or `internal/store`.
- How the schema-per-run test harness names and tears down schemas.
- The DSN guard's exact refusal mechanism, provided it refuses **by construction**
  the way `config.Sandbox()` does rather than by convention.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### This milestone's decisions
- `.planning/PROJECT.md` — Current Milestone v0.2, Constraints, and the Key
  Decisions table. The UUIDv7 / server-sequence / actor-id split, the local-queue
  constraint, and "nothing is kept that cannot be gotten back" are all recorded there.
- `.planning/REQUIREMENTS.md` §Milestone v0.2 — STORE-02, STORE-03, STORE-10 and
  QUEUE-01..05 are this phase's requirements, verbatim.
- `.planning/ROADMAP.md` §Phase 7 — goal, five success criteria, and the note
  explaining why STORE-02/03 sit here rather than with the conversions.

### Research
- `.planning/research/SUMMARY.md` — the reconciled account; §"Implications for
  Roadmap" and §"What Must Be Deliberately Preserved" are the relevant parts.
- `.planning/research/STACK.md` — pgx v5.11.0, goose v3.28.0, testcontainers +
  the `apache/age` image. **Version facts are HIGH confidence, API-verified
  2026-09-11 — do not re-research them.** Also records that the official AGE Go
  driver is stale and must not be used, and that `embedded-postgres` and `pg_tmp`
  cannot work here because they cannot inject the AGE extension.
- `.planning/research/ARCHITECTURE.md` — the `store`/`queue` seam argument, and
  why per-test transaction rollback does not compose with pgxpool plus AGE's
  per-connection session state.
- `.planning/research/PITFALLS.md` — the silent-drain-stall failure mode and the
  queue pitfalls generally.

### Prior art in this repository, which this phase generalises rather than replaces
- `internal/events/events.go` — `Emit`'s fsync-before-return, the
  `markFailing`/`clearFailing` first-write-wins marker, and `HealthOf`. The queue's
  durability is this mechanism moved in front of a network call.
- `internal/config/sandbox.go` — `Sandbox()` panics if a test resolves the garden
  outside a temp directory. STORE-10 needs the database-era equivalent of that
  refusal, not a weaker one.
- `.planning/milestones/v0.1-phases/01-trustworthy-event-writes/` — the phase that
  built the above, including its verification record.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/events.Emit` — the fsync-before-return write. The queue's accept path
  is this, pointed at a different file.
- `internal/events.markFailing` / `clearFailing` / `failMarkPath` — first-write-wins
  marker whose mtime is the entire payload. D-05 reuses the idiom for drain backoff.
- `internal/events.HealthOf` — already distinguishes "nothing has run" from "writes
  have been failing", and already refuses to answer when it cannot verify a write
  would land. Queue depth and drain staleness fold into this surface, per the
  requirement that there be no second dashboard.
- `internal/config.Sandbox` — the "refuse by construction" idiom STORE-10 mirrors.

### Established Patterns
- Errors are returned and wrapped with `fmt.Errorf(... %w)`; an instrument never
  fails the work it measures, but it never goes silent either. Every queue failure
  path inherits this.
- No goroutines in CLI paths. D-04's opportunistic drain runs inline, which is why
  the timeout and backoff are load-bearing rather than polish.
- `CGO_ENABLED=0` builds clean today and the repo cross-compiles for Windows.
  `pgx` is pure Go and keeps that true; anything that breaks it is out.
- Build-tagged platform files already exist (`internal/events/writable_unix.go`,
  `writable_other.go`) — the precedent if machine identity needs platform code.

### Integration Points
- `internal/events`, `internal/draws`, `internal/pile` keep their public APIs in
  this phase. Only their backend changes, and not until Phases 8 and 9.
- The four `draws.Load()` readers — `internal/cli/tend.go:76`, `garden.go:78`,
  `yield.go:278`, `yield.go:391` — are the call sites D-06 exists to leave alone.
- `internal/cli/yield.go` `showHealth` is where queue depth and drain staleness
  surface.
</code_context>

<specifics>
## Specific Ideas

- The queue is deliberately greppable. STORE-09 requires reading and searching
  events without a database client **including undrained writes**, and a JSONL
  queue serves that in the same phase that creates the need.
- "One bad line costs one event, never the history" is a property of the current
  `events.Load()` that must survive into both the drain and the read overlay. A
  malformed queue line costs that line.
- The backoff marker should make an offline machine cheap, not just correct: the
  test worth writing is that N invocations while offline cost roughly one dial,
  not N.
</specifics>

<deferred>
## Deferred Ideas

- **Background or detached drain** — considered and set aside. The codebase has no
  goroutines in CLI paths and no process supervision, and a detached drain that
  dies is precisely the silent stall QUEUE-04 exists to prevent. Revisit when the
  resident garden TUI (SURF-01, deferred out of v0.2) gives it a supervisor.
- **Per-kind queue files** — set aside under D-03. Worth revisiting only if a
  poisoned record in one kind is observed blocking others in practice.
- **Pulling SUB-09 into scope** — bd's ticket↔ticket edges. Phase 11 reads `bd`
  directly at projection time instead. Recorded in the roadmap as a scope change
  to the milestone rather than a roadmap revision, if it is ever wanted.
</deferred>

---

*Phase: 7-A Queue The Network Cannot Stop*
*Context gathered: 2026-09-11*
