# Pitfalls Research: Moving a File-Backed CLI onto Shared Postgres + Local Write Queue

**Domain:** Single-binary Go CLI, currently JSONL + git-backed markdown, adding a
shared Postgres substrate, Apache AGE property graph, and a durable local write
queue — for one gardener, ~50 events/day, several machines, no CI today.

**Researched:** 2026-09-11
**Confidence:** MEDIUM overall. The queue, outbox, and Postgres-backup findings
are well-corroborated general operational knowledge (LOW-confidence individual
web sources, but the patterns are long-standing and cross-check against each
other and against this project's own phase 01 code). Apache AGE findings are
MEDIUM — sparser sources, some drawn from vendor docs (Postgres Pro / Microsoft)
rather than the AGE project itself, and version-support claims should be
re-checked against `github.com/apache/age` at the point AGE is actually pinned.

## Critical Pitfalls

### Pitfall 1: The drain loop dies and nothing says so (Queue)

**What goes wrong:**
The local queue's drain worker — the thing that notices Postgres is reachable
and replays queued writes — panics, gets stuck on a poisoned row, or simply
never gets started (e.g. only spawned from a code path that never runs on a
long-lived `garden` session). Writes keep landing locally, `hugel gate` keeps
succeeding, nothing on the CLI's exit path ever touches the network again. The
queue grows forever and the shared Postgres substrate quietly stops being
shared.

**Why it happens:**
A drain loop is background machinery with no caller waiting on its result —
exactly the shape phase 01 already identified as dangerous for `Emit`: nobody
is positioned to notice a background failure unless something is watching for
it on purpose. Unlike `Emit`, which returns its error synchronously to a
caller that ran a second ago, a drain loop's failure has no natural caller at
all.

**How to avoid:**
Treat "is the queue draining" as a first-class health fact with the same
two-value shape phase 01 built for events: `LastDrainSuccess` (when the queue
last emptied, or last shrank) and `DrainFailingSince` (when the current
failure streak started), following the exact `markFailing`/`clearFailing`
pattern already in `internal/events/events.go`. `yield --health` must surface
both, distinguishing "queue empty, nothing pending" from "queue has pending
rows and hasn't shrunk since <date>" — the same "nothing happened" vs
"something is failing" distinction phase 01 fought for with events. Do not let
"queue length is 0" be the only signal: a stalled drain with an empty queue
(because writes also stopped) looks identical to a healthy idle queue unless a
last-attempt timestamp is also tracked.

**Warning signs:**
Queue depth graphed (or eyeballed) over days only ever goes up; `yield
--health` reports the event log healthy but has nothing to say about the
queue; the drain loop's only failure path is a log line nobody reads on a CLI
that is not always attended.

**Phase to address:** Local write queue phase — health surface must ship in
the same phase as the queue itself, not deferred, mirroring the requirement
already recorded for phase 01's carry-forward.

---

### Pitfall 2: Retry after a partial local ack produces duplicate rows in Postgres (Queue)

**What goes wrong:**
The drain worker sends a batch, Postgres commits it, but the worker crashes or
loses the connection before it marks those rows as drained locally. On
restart it re-sends the same batch. Every downstream table that isn't
explicitly deduplicated now has the event, the draw, or the pile entry twice.

**Why it happens:**
At-least-once delivery is what a local durable queue can honestly promise; a
crash between "remote commit" and "local mark-sent" is not a rare edge case,
it is the modal failure of exactly this design, confirmed independently by
every outbox-pattern source consulted (pgx-outbox and others): "if Publish
succeeds but MarkDelivered fails, the next poll cycle republishes the same
row."

**How to avoid:**
Give every locally-queued write a stable identity assigned at enqueue time
(not at send time) — a ULID or a hash of its own content — and make every
insert on the Postgres side `INSERT ... ON CONFLICT (id) DO NOTHING` (or
`DO UPDATE` only where overwrite is actually correct). This makes duplicate
delivery inert rather than something the drain loop has to get exactly right.
Do not rely on "mark sent, then never resend" as the only safety net — that is
exactly the step that can fail after a successful remote write.

**Warning signs:**
Row counts in Postgres exceed row counts implied by the local queue's
lifetime enqueue count; two rows share every field except a Postgres-assigned
serial id; a restore drill (Pitfall 6) surfaces duplicates that the live
system never flagged because nothing ever compared queue-sent-count against
Postgres-received-count.

**Phase to address:** Local write queue phase. The idempotency key belongs in
the wire format from the first write, not bolted on later — retrofitting a
dedupe key onto rows already written without one is a second migration.

---

### Pitfall 3: A batched drain reorders writes relative to how they were queued (Queue)

**What goes wrong:**
The queue holds writes in enqueue order, but the drain worker sends them
concurrently (worker pool, multiple connections, or a batch that Postgres
executes without an explicit sequence column) to shave latency. Two writes to
the same bead — say a gate-stage event and the entry it produced — land in
the opposite order from how they happened. Anything downstream that infers
causality from arrival order (a graph edge, a "most recent state" read) is now
wrong in a way that is invisible unless someone diffs timestamps against
insertion order.

**Why it happens:**
Ordering is free when there is one writer and one file (`Emit`'s single mutex
and append-only file already guarantee it locally). It stops being free the
moment there are multiple queued writers, multiple machines, or a drain that
parallelizes for throughput this project does not need at ~50 events/day.

**How to avoid:**
Keep the drain worker single-threaded and strictly FIFO per queue — there is
no volume here that justifies parallel drain. Stamp every locally-queued
write with a monotonic local sequence number at enqueue time (not the
wall-clock time, which is not monotonic across NTP adjustments) and carry that
sequence into Postgres as an ordering column separate from any Postgres-side
serial id, so downstream readers can always ask "in what order did this
gardener actually do these" independent of what order rows landed in the
shared table.

**Warning signs:**
Two rows for the same bead have Postgres-assigned ids in one order and
event timestamps in the other; a graph edge derived from "the event before
this one" points at the wrong node; drain worker code contains a worker pool,
goroutine fan-out, or unordered channel where a project this size has no
throughput reason to want one.

**Phase to address:** Local write queue phase.

---

### Pitfall 4: The queue itself grows without bound and nobody notices until the disk is full (Queue)

**What goes wrong:**
Phase 01 already solved this for `events.jsonl` — a full disk is a named,
tested failure mode there. The queue is a new file (or new table) that can
repeat the exact same failure if its growth isn't bounded and surfaced the
same way. If Postgres is unreachable for an extended stretch (a laptop closed
for two weeks, a home server down), the queue is the only thing absorbing
every write in the meantime, and nothing in its own design caps how large that
gets.

**Why it happens:**
"Writes never block on the network" (a hard project constraint) is easy to
satisfy by making the local queue infinitely patient — which is exactly what
makes it grow without limit. An unbounded queue turns a connectivity gap into
silent disk pressure instead of a visible, actionable backlog.

**How to avoid:**
Apply the same writability probe `events.go` already has (`writable`,
checking both permission and free blocks) to the queue's own append path, so
a full disk marks the queue failing exactly the way it marks the event log
failing today — one shared mechanism, not two. Separately, put a soft ceiling
on queue depth or age (e.g., "queue has unsent writes older than N days") that
surfaces through `yield --health` as its own fact, distinct from "queue is
failing to write" — a queue that is accepting writes fine but has not
successfully drained in three weeks is a different problem from a queue that
cannot append at all, and conflating the two hides whichever one is not the
current cause.

**Warning signs:**
Queue file/table grows monotonically over normal single-gardener use; no
existing alert or health check distinguishes "queue large because network has
been down a while" (expected, self-healing) from "queue large because drain
is broken" (Pitfall 1); disk-full test coverage exists for `events.jsonl` but
not for the queue.

**Phase to address:** Local write queue phase — the writability probe and
health-surface work should be shared code with `internal/events`, not a
parallel reimplementation.

---

### Pitfall 5: The queue's own file/table is corrupted by a non-atomic write (Queue)

**What goes wrong:**
A process is killed mid-write to the queue (crash, `kill -9`, laptop battery
death) and the queue's storage is left with a torn record — a half-written
JSON line, a table row with some columns null that should never be null. On
next start the queue either can't be parsed at all (worse than a bad JSONL
line, because `events.Load()`'s "one bad line costs one event, never the
history" resilience assumes each line is independently parseable and the file
format doesn't have to represent partial multi-step operations) or it drains
that half-written record into Postgres.

**Why it happens:**
`events.go` earns its append-only, single-line-per-record durability by doing
exactly one `os.OpenFile(O_APPEND) → Write → Sync` per event, under a mutex,
with no multi-line records. A queue that batches multiple logical writes into
one flush, or that needs a header/footer/checksum spanning more than one
line, reintroduces the torn-write hazard `Emit` was specifically built to
avoid.

**How to avoid:**
Reuse `events.go`'s exact durability shape for the queue rather than
inventing a new one: one append-only file (or one row per statement, if
table-backed), one record per write, fsync before the enqueue call returns
success to its caller, and a parser that treats one malformed record as a
skip rather than a fatal error on load — the same "one bad line costs one
event" contract, stated for the queue instead of the event log. If the queue
is table-backed (e.g., a local SQLite file, given `pgx` staying pure-Go
argues against a cgo-based local store), use that store's own WAL mode and a
single-writer discipline instead of hand-rolling one.

**Warning signs:**
The queue format supports multi-record transactions or spans multiple lines
per logical write; drain code has a special case for "what if this record is
malformed" that either panics or silently drops without counting; there is no
test that kills the process mid-append and asserts the queue still loads
cleanly afterward (the mechanical equivalent of the disk-full tests phase 01
already has for events).

**Phase to address:** Local write queue phase.

---

### Pitfall 6: A backup exists, has run on schedule for months, and does not restore (Backup)

**What goes wrong:**
`pg_dump` (or a WAL-archiving job) has been running nightly without a single
error in its own log. The day it's needed, the restore fails — wrong Postgres
major version, a schema that changed shape after the backup script was
written and no longer matches what restore expects, a dump that's logically
consistent but missing the AGE extension's catalog rows (Pitfall 9), or a WAL
archive with silent gaps that only matter when a specific range is requested.
This is precisely the project's own recorded regret restated: "every graph
hugel kept died with the store it lived in," except this time the backup
existed and everyone believed it worked.

**Why it happens:**
A backup job's own success signal (exit code 0, a file with a plausible size
appearing in the target location) says nothing about restorability. The most
common concrete version of this failure found in general Postgres operational
material: `archive_command` starts failing silently while `pg_stat_archiver`
shows a climbing `failed_count` and a stale `last_archived_time` — a fact
sitting in the database the whole time, unread because nobody queries it.
Automation that "ran" and automation that "produced something restorable" are
different claims, and only the first one is usually monitored.

**How to avoid:**
Treat the restore as the artifact that must be verified, not the backup file.
Concretely: after every backup (or on a fixed cadence — see Pitfall 7), spin
up a disposable Postgres instance, restore into it, and run a small set of
fidelity checks against it (row counts per table, a known-entry lookup, the
AGE graph queryable with one Cypher statement) — automated, not manual,
because a manual restore drill is the first thing skipped under time
pressure. This is exactly the "restore drill that is actually run" the
milestone commits to; the drill only counts if it is a command someone (or
something) actually executes, not a runbook that exists.

**Warning signs:**
Nobody can say when the last successful *restore* (as opposed to backup) was
performed; the backup verification step, if any, only checks that a file was
produced; `pg_stat_archiver.failed_count` (or the equivalent for whatever
archiving mechanism is chosen) has never been looked at.

**Phase to address:** Backup & restore phase — and it should be sequenced
*before* or *alongside* the AGE graph work, not after, since AGE data has its
own restore hazards (Pitfall 9) that the drill needs to already be exercising.

---

### Pitfall 7: The restore drill is written once, run once, and never run again (Backup)

**What goes wrong:**
A restore script gets built, runs successfully during the phase that builds
it, gets marked "done," and is never executed again. Six months later the
schema has grown three columns, AGE has been upgraded, and the drill script
— untouched since — either silently skips checks it no longer understands or
fails on something irrelevant, and either way nobody is watching it fail
because nothing schedules it.

**Why it happens:**
For a single-gardener, ~50-events/day project there is no operations team and
no paging system watching a cron job. A drill that depends on someone
remembering to run it manually decays exactly like the "durability guarantee
nobody exercises is not a guarantee" observation the project has already
made about itself.

**How to avoid:**
Pin the drill to an event the gardener already can't avoid noticing, not to a
calendar the gardener has to remember. The cheapest mechanism at this scale:
make the restore drill a step in the existing `gate` sequence at some
low-but-nonzero frequency (e.g., part of a periodic `hugel yield --health`
invocation, or a check that fails a specific health field once N days have
passed since the last successful drill — mirroring `FailingSince` from
phase 01's pattern, but for "last restore verified" instead of "last write
failed"). A restore drill run monthly for a database that changes daily is
enough to catch schema drift and extension-version drift before they compound
across a year; annual is not enough given how much AGE and Postgres version
churn is already anticipated in this project's own decision log.

**Warning signs:**
The restore script's last-modified date and last-run date are both old and
identical; `yield --health` has no field for "restore last verified"; the
drill's own dependencies (a scratch Postgres instance, Docker, disk space) have
silently stopped being available on some of the "several machines" the garden
runs from, and nobody would know until the drill is needed for real.

**Phase to address:** Backup & restore phase, with the health-surface tie-in
built in the same phase rather than left as follow-up work — a drill with no
enforced cadence is optional, and optional things do not run.

---

### Pitfall 8: Timestamps shift silently on import (Migration)

**What goes wrong:**
`events.jsonl` stores time as `RFC3339Nano` in UTC (`e.Time.UTC().Format(...)`
per `events.go`'s `MarshalJSON`), which is exactly right — but the import
script that reads it and writes it into Postgres can still get this wrong if
the target column is `TIMESTAMP WITHOUT TIME ZONE` or if the import tool
applies a local-timezone interpretation on the way in. General Postgres
migration writeups report this as a common, silent failure: one cited
real-world case had 87% of migrated rows shifted by more than an hour, and the
data still looked plausible — dates a day off are easy to miss in a spot
check.

**Why it happens:**
The source format is already correct (UTC, explicit offset); the risk is
entirely introduced by the import path, and a `TIMESTAMP WITHOUT TIME ZONE`
column silently accepts and reinterprets a value that already looked like a
timestamp, giving no error to notice.

**How to avoid:**
Use `TIMESTAMPTZ` for every timestamp column touched by this migration, full
stop, and verify with an explicit query after import: pick N known events by
id, compare their imported `EXTRACT(EPOCH FROM ...)` against the epoch seconds
parsed directly from the original JSONL line, and fail the import if any
differ. Do not trust "the dates look about right."

**Warning signs:**
Any column declared `TIMESTAMP` without `WITH TIME ZONE`; an import script
that constructs timestamps via string concatenation or a library call that
takes a "local timezone" argument; no post-import spot check comparing
source-parsed epoch to database-stored epoch for the same event.

**Phase to address:** Migration/import phase.

---

### Pitfall 9: Import order is inferred from file position, and that assumption breaks the moment there is more than one source file (Migration)

**What goes wrong:**
`events.jsonl` today has no explicit sequence number — line order in the file
*is* the order, which works because there has only ever been one writer
process on one machine appending to one file. The moment there is more than
one machine (already true — "several machines" is stated in the milestone),
or more than one `events.jsonl` per machine to merge, "the order I read the
lines in" stops being "the order the events actually happened in": clock skew
between machines means two files interleaved by timestamp can still be wrong
relative to true causal order, and a naive importer that just concatenates
files in whatever order it lists them gets this wrong silently.

**Why it happens:**
The single-file, single-writer JSONL format never had a sequence problem, so
nothing in the source data carries one — no primary-machine flag, no Lamport
clock, no per-machine monotonic counter alongside the wall-clock timestamp.
Multi-source import is being asked to answer a question the source format was
never designed to answer.

**How to avoid:**
Decide, and write down, what "order" means across multiple source files
before the import script exists: if wall-clock timestamp is going to be the
merge key (it likely has to be, given no better signal exists in the current
format), make the import explicit about that choice and about its known
failure mode (clock skew) rather than silently assuming file-concatenation
order is fine. If a future phase widens event emission (already an Active
requirement), add a per-machine monotonic sequence number to new events now
so this stops being unanswerable going forward, even though it can't be
retrofitted onto the already-written history.

**Warning signs:**
An import script that reads multiple `events.jsonl` files and concatenates
rather than timestamp-merges them; no documented answer to "what happens if
two machines' clocks disagree by five minutes" before the import runs;
events from two machines for the same bead interleave in an order that
doesn't match the gate stages that actually ran.

**Phase to address:** Migration/import phase, informed by whatever the
substrate-widening phase decides about per-event sequencing.

---

### Pitfall 10: A converging-write key that worked for a single git-backed file collides once multiple machines import into one table (Migration)

**What goes wrong:**
The pile's converging writes are keyed on scope + type + normalized title
(per `PROJECT.md`'s description of the existing pile store) — a scheme that
works because git and a single filesystem provide the serialization. Import
two machines' independent pile histories into one Postgres table and two
entries that converged independently (same scope/type/title, written on two
machines before they ever synced) either collide on a uniqueness constraint
the importer wasn't expecting, or — worse — silently overwrite one with the
other depending on import order, discarding real content with no error.

**Why it happens:**
A key designed to make writes on *one* store converge was never designed to
be a *merge* key across two independently-written stores; those are different
problems that happen to look similar.

**How to avoid:**
Before running the one-shot import, generate a collision report: group all
source rows (across every machine's export) by the existing convergence key
and print every group with more than one distinct row. Resolve every
collision explicitly (keep both under new keys, keep newest, keep both with a
provenance note) before the import writes anything — never let the importer's
own insert-or-update logic make that call implicitly via `ON CONFLICT DO
UPDATE` or similar, which silently picks "last writer wins" as the resolution
strategy without anyone deciding that on purpose.

**Warning signs:**
The import script has an `ON CONFLICT` clause with no corresponding
pre-import collision report; entry counts after import are lower than the sum
of entries across all source files with no logged reason why; two
known-different entries (checked by hand) come back identical after import.

**Phase to address:** Migration/import phase.

---

### Pitfall 11: The import reports success while having silently dropped or truncated data (Migration)

**What goes wrong:**
The importer processes every line in the source files, logs "import
complete," exits 0 — and did in fact insert *most* rows, having skipped some
because of a parse error, a constraint violation caught and swallowed, a
batch that partially committed before a later statement in the same
transaction failed, or a line that exceeded some column's length limit. A
one-shot import that only checks its own exit code, and never compares
before/after counts, cannot distinguish "everything imported" from "almost
everything imported."

**Why it happens:**
This is the generic shape of the "test that passes with the code deleted"
lesson phase 01 already learned, restated for the migration script: a
migration that reports success by absence of a thrown exception is reporting
the wrong thing, exactly as a health check that reports the absence of a
crash instead of the presence of correctly-flowing writes reported the wrong
thing.

**How to avoid:**
Make the import's completion criterion an explicit reconciliation, not an
exit code: count source records (lines in JSONL, entries in the pile, rows in
whatever else is migrated) before the run; count destination rows after; the
import is not "complete" unless those numbers match after accounting for any
documented, deliberate exclusions (e.g., the test-exhaust lines already
excised from `events.jsonl`, per its own doc comment). Print the diff, don't
just assert equality silently. Run the import against a copy first and diff a
sample of records field-by-field (not just counts) before running it for
real — count-matching is necessary but does not catch the timestamp-shift or
id-collision failures above, which can preserve row counts while corrupting
content.

**Warning signs:**
The import script's success message is "N lines processed" rather than "N of
M source records imported, M reconciled"; no script or query exists that
independently re-derives the source count and compares it after the fact;
errors during import are caught and logged rather than aborting the run.

**Phase to address:** Migration/import phase — the reconciliation check
should be a gate the import cannot report success without passing, not a
manual step run at the operator's discretion.

---

### Pitfall 12: `pg_upgrade` is quietly incompatible with AGE, and nobody discovers this until a Postgres upgrade is already underway (Apache AGE)

**What goes wrong:**
AGE's own tables use `reg*`-family, OID-referencing column types internally.
`pg_upgrade` — the standard, fast, in-place major-version upgrade path for
Postgres — does not support databases containing those types, because OIDs
are not guaranteed stable across a major-version upgrade. A gardener (or a
future automated dependency-bump) runs a routine `pg_upgrade` the way they
would on any other Postgres database, and it either refuses outright or,
worse, "succeeds" while leaving the graph unusable.

**Why it happens:**
`pg_upgrade` is the default, well-known, low-friction way to move a Postgres
major version, so it's what gets reached for by habit unless the AGE-specific
exception is already known going in.

**How to avoid:**
Treat every Postgres major-version upgrade as a dump-and-restore event for
this database, not a `pg_upgrade` event, and write that down where whoever
does the upgrade will actually read it (a runbook, or better, a check the
upgrade tooling itself runs: "does this database have the AGE extension
installed? If so, refuse `pg_upgrade` and point at dump/restore instead").
Because the restore drill (Pitfall 6/7) already exercises dump-and-restore
regularly, a Postgres major-version upgrade becomes "run the drill's restore
step against the new version" rather than a novel procedure invented under
time pressure.

**Warning signs:**
An upgrade runbook or script that calls `pg_upgrade` without a
graph-extension check first; AGE version pinned against a Postgres version
that is approaching end-of-life, creating upgrade pressure before the team is
ready; no test that a dump taken under Postgres N restores cleanly under
Postgres N+1 with AGE reinstalled.

**Phase to address:** AGE/graph phase for the pin decision and documentation;
Backup & restore phase for making the drill exercise this path specifically.

---

### Pitfall 13: A `pg_dump` of the graph restores into a database where the graph looks empty or throws on first query (Apache AGE)

**What goes wrong:**
AGE stores each graph's data in per-graph schemas and catalog rows created by
its own DDL-like functions (`create_graph`, label creation, etc.), and those
catalog writes are only visible to other sessions once the transaction that
made them commits. A dump/restore sequence that doesn't first `CREATE
EXTENSION age` and re-establish the AGE catalog *before* the data restore
runs, or that restores data and extension in the wrong order, leaves rows
present in the underlying tables but the graph functions unable to see or
query them — "the graph is empty" or a hard error on the first Cypher query,
even though `pg_dump` "succeeded."

**Why it happens:**
`pg_dump`/`pg_restore` handle extensions and data as related-but-separate
concerns; AGE's graph structures ride on top of both an extension and a set
of catalog entries that have to be reconstructed in the right order, and nothing about
a generic Postgres dump enforces that order for you.

**How to avoid:**
Never trust a graph restore until the restore drill (Pitfall 6) runs an
actual Cypher query against it and gets the expected shape back — row counts
in raw tables are not sufficient evidence the graph is queryable. Script the
restore procedure explicitly: extension created and graph re-registered
first, data loaded second, one canonical Cypher query run and its result
compared against a known answer as the pass/fail criterion. Because the
project's own constraint already treats the graph as a droppable, replayable
projection over durable base tables (events, draws, entries, bead ids), the
cheapest and most robust "restore" for the graph specifically may not be
`pg_dump`/`pg_restore` at all, but drop-and-replay from those base tables —
worth deciding explicitly rather than defaulting to treating the graph like
any other Postgres data.

**Warning signs:**
A restore procedure that restores the whole database in one `pg_restore`
invocation with no graph-specific verification step; "restore succeeded"
measured by `pg_restore`'s own exit code rather than by a live Cypher query;
no documented decision on whether the graph is restored from dump or rebuilt
from replay.

**Phase to address:** AGE/graph phase for the replay-vs-restore decision;
Backup & restore phase for the verification query.

---

### Pitfall 14: A Cypher query that's fast on 316 entries and zero edges falls off a cliff once edges actually exist (Apache AGE)

**What goes wrong:**
The pile currently has 316 entries and zero edges — meaning every Cypher
query written and tested against real data so far has been tested against a
graph with no traversal depth at all. Variable-depth traversal (the entire
reason AGE was chosen over hand-rolled recursive CTEs, per this project's own
decision log) is also where property graphs get slow if the underlying
vertex/edge properties used in `WHERE` clauses or match patterns aren't
indexed — a query pattern that returns instantly over an empty graph can
become a multi-second (or worse) scan once code↔code, code↔ticket,
ticket↔ticket and entry↔entry edges are actually populated at any scale.

**Why it happens:**
Development and testing naturally happen against whatever data exists, and
right now that's an empty graph. A performance cliff that only appears once
the graph is populated is invisible during the exact phase where the queries
are being written.

**How to avoid:**
Before shipping any Cypher query used in the draw path (explicitly called out
as a future requirement — "graph in the draw path"), test it against a
synthetically populated graph sized to a plausible multi-year projection of
this project's own numbers (hundreds of entries, low thousands of edges — not
enterprise scale, but not zero either), not just against whatever the graph
happens to hold today. Add indexes on whatever vertex/edge properties are
actually matched or filtered on (AGE supports property indexes; don't assume
Postgres's general-purpose planner will find the right one for a graph
pattern without help).

**Warning signs:**
Every Cypher query in the codebase has only ever been run against the
current, near-empty graph; no test or benchmark exists with synthetic edges
at a representative multi-year volume; a query pattern uses unindexed
property equality inside a variable-length path match.

**Phase to address:** AGE/graph phase — the synthetic-scale benchmark should
be a gate before "graph in the draw path" ships, not discovered after.

---

### Pitfall 15: `grep`/`jq`-over-the-log stops working the day events move into Postgres, and nothing replaces it (Losing File Properties)

**What goes wrong:**
Today, "what happened Tuesday" is `grep '"2026-09-09' events.jsonl | jq .` — no
tool required beyond what's already on the machine, works with no network, works
when Postgres itself is the thing being debugged. Move events into Postgres and
that debugging path is gone unless something replaces it deliberately: now
"what happened Tuesday" requires a working Postgres connection, credentials,
and either SQL fluency or a purpose-built CLI command — a strictly worse
debugging experience for the exact moment (something's wrong with the
substrate) when the simplest possible tool matters most.

**Why it happens:**
The move to Postgres is framed around durability and queryability, both real
wins — but "queryable with SQL" is not the same property as "greppable with
tools already on the machine while nothing else is working," and the second
property doesn't survive the move unless someone notices its absence and
builds a replacement on purpose.

**How to avoid:**
Ship a `hugel events tail` / `hugel events grep <pattern>` (or equivalent)
command in the same phase that moves events into Postgres, so the gardener
never loses "quick, no-SQL, offline-capable* view into recent events" as a
capability — even though the underlying storage changed. (*Offline-capable
only for whatever's still in the local write queue, which is itself a reason
the queue's own contents should be inspectable the same way, not just a black
box waiting to drain.) Don't consider the migration complete while the only
way to answer "what happened" is a `psql` session.

**Warning signs:**
No CLI command exists for reading recent events without writing raw SQL;
debugging a live incident requires first establishing a Postgres connection,
which may be the very thing that's down; the local queue's pending writes are
not human-readable by any means simpler than deserializing whatever binary or
row format it uses.

**Phase to address:** Substrate/Postgres migration phase — ship the
read-path CLI alongside the write-path change, not as a later nice-to-have.

---

### Pitfall 16: One malformed row in a bulk import/insert kills a whole transaction instead of costing one row (Losing File Properties)

**What goes wrong:**
`events.Load()`'s parser treats a bad JSONL line as "skip it, keep going" —
"one bad line costs one event, never the history," in the code's own words.
The natural way to write a bulk loader against Postgres is one multi-row
`INSERT` or one transaction wrapping many rows, which has the opposite
failure shape: one malformed value in one row aborts the whole statement (or
the whole transaction), and either the entire batch fails or — if wrapped in
a blanket try/catch that swallows the error — the entire batch is silently
skipped, converting "one bad line, one lost event" into "one bad line, N lost
events."

**Why it happens:**
Bulk-insert code is written for throughput and correctness of the *typical*
row; the file-based resilience property was a specific, deliberate design
decision (documented in `events.go`'s own comments) that doesn't transfer
automatically just because the data moved.

**How to avoid:**
For both the one-shot migration and the ongoing queue drain, insert row by
row (or in small batches with per-row error isolation — e.g., `SAVEPOINT`
before each row, roll back to it and log-and-continue on a single-row
failure) rather than one large all-or-nothing statement, so a bad row costs
exactly that row, not its batch. Log every skipped row with enough content to
reconstruct or manually re-import it later — a skip that isn't recorded
anywhere is data loss with a clean exit code.

**Warning signs:**
Drain or import code wraps N rows in one transaction with no per-row
savepoint or isolation; a single malformed record causes an entire batch's
worth of legitimate events to disappear from a health check or a drain log;
skipped-row counts are never logged anywhere queryable.

**Phase to address:** Local write queue phase (drain path) and
Migration/import phase (one-shot path) — same principle, two different code
paths, both need it independently.

---

### Pitfall 17: The pile's git-diffable review history stops being reviewable the moment it's a Postgres table (Losing File Properties)

**What goes wrong:**
Part of what makes the pile trustworthy today is that its markdown files live
in git: a change to an entry is a diff a human can read, `git log -p` shows
who changed what and when, and a bad edit can be reverted with `git revert`
the same way code is. If the pile's storage moves fully into Postgres with no
equivalent, that review affordance disappears — edits become opaque row
updates, and "what changed and why" requires purpose-built audit-log tooling
that doesn't exist yet, rather than a tool (`git`) that already does and that
every other part of this project already trusts.

**Why it happens:**
PROJECT.md is explicit that bd stays authoritative for beads and Postgres
carries relations, but does not fully specify whether the pile's *content*
(not just relations about it) moves into Postgres or stays git-backed
markdown with Postgres holding pointers/metadata. If the former, the
diffability property needs a deliberate replacement; if the latter, it's
preserved for free — but the decision needs to be made explicitly rather than
falling out of whatever's convenient to implement.

**How to avoid:**
Decide explicitly, and record in the decision log, whether pile *content*
stays git-backed markdown (Postgres holding only ids, metadata, and edges for
querying) or moves fully into Postgres. If it moves, add an append-only
audit table (entry id, changed field, old value, new value, actor, timestamp)
from day one — bolting provenance onto a mutable table after the fact means
the history before that point is unrecoverable, which is precisely the kind
of loss this milestone exists to prevent.

**Warning signs:**
No explicit decision recorded on where pile content lives post-migration;
schema for pile entries in Postgres has no audit/history table alongside it;
"who changed this entry and why" requires anything other than a query this
project's own tooling already exposes.

**Phase to address:** Substrate/Postgres migration phase — this decision
gates the schema design and should be made before, not during, implementation.

---

### Pitfall 18: A health check for the new substrate reports the wrong thing with total confidence, the same way phase 01's did before four verification passes caught it (Testing/CI)

**What goes wrong:**
This project has already spent four verification passes learning that "a
health check can confidently report the opposite of the truth" and that "a
test which passes with the code deleted proves nothing." Every new health
signal this milestone adds — queue-drain health, backup-restore health,
Postgres-reachability health — is exactly as capable of repeating that
mistake as the original event-health check was, and there's no reason to
assume the lesson automatically applies to new code just because it was
learned once on old code.

**Why it happens:**
A health check is trivially easy to write in a way that's green by
construction — e.g., checking "did the last operation throw" rather than
"did the last operation's stated postcondition actually hold" — and that
mistake is invisible until someone deliberately tries to break the thing being
checked and confirms the health check turns red.

**How to avoid:**
Apply phase 01's own verification method to every new health field this
milestone adds, not just the queue-drain one flagged in Pitfall 1: for each
new health check, write (and keep) a test that deletes or breaks the
underlying mechanism the check claims to monitor and asserts the health check
turns unhealthy — a mutation test for the health check itself, not just a
happy-path test that it returns healthy on a healthy system. A health check
with no companion "prove it can go red" test should be treated as unverified,
exactly the standing this project now holds prior health checks to.

**Warning signs:**
A new health field (queue health, restore-drill-freshness, Postgres
reachability) has tests only for the healthy case; nobody has tried unplugging
the network, killing the drain process, or corrupting a backup on purpose and
confirmed the relevant health field notices; code review for a health check
doesn't ask "what does this report if the thing it watches is silently
broken."

**Phase to address:** Every phase that adds a health signal (queue, backup,
Postgres reachability) — this is a standing practice, not a one-time fix, and
should be named as an acceptance criterion in each phase's plan rather than
left to be rediscovered.

---

### Pitfall 19: Postgres-dependent code either goes untested, or gets tested against the real garden — both of which the project's own rules forbid (Testing/CI)

**What goes wrong:**
`config.Sandbox()` panics if a test resolves the garden outside a temp
directory, and there is no CI at all today — no `.github/workflows`, nothing
that could spin up a scratch Postgres automatically on every push. Faced with
that, the path of least resistance for anyone writing queue-drain, migration,
or AGE-query code is to either (a) skip testing the Postgres-touching parts
and only unit-test the parts that don't need a database, silently shrinking
test coverage exactly where this milestone's riskiest new code lives, or (b)
point tests at a real, already-running Postgres instance because that's what's
locally available, which either violates the spirit of `Sandbox()` (a shared
garden's Postgres instance is exactly the kind of real, persistent resource
`Sandbox()` was built to keep tests away from) or requires a parallel
sandboxing mechanism that doesn't exist yet for Postgres the way it exists for
the filesystem-backed garden.

**Why it happens:**
`Sandbox()` was built around a `t.TempDir()`-based test double for a
filesystem-backed store; Postgres has no directory to point at, so the same
mechanism doesn't directly generalize, and building the Postgres-equivalent
sandbox is exactly the kind of infrastructure work that's easy to defer once
"just point it at localhost" gets something running today.

**How to avoid:**
Build the Postgres-equivalent of `Sandbox()` before or alongside the first
Postgres-touching code, not after: a disposable, ephemeral Postgres instance
per test run (a real Postgres binary managed directly, or a Docker-based
harness) that is provably isolated the same way `t.TempDir()` is — fresh data
directory, random port, torn down after — and that a package-level helper
(`postgrestest.Sandbox()` or similar) refuses to hand back anything that looks
like a real, persistent connection string, mirroring `config.Sandbox()`'s own
refusal shape. Because there is no CI yet, this harness must work identically
run locally by hand today as it will when CI exists later — that constraint
should shape the choice of mechanism (e.g., prefer something that doesn't
assume a CI-provided Docker daemon with no local equivalent) rather than
building something that only works once CI is added.

**Warning signs:**
Any test file that connects to a Postgres DSN read from an environment
variable pointing at a real, developer-maintained database; Postgres-touching
code with no test coverage at all, distinguishable from the rest of the
codebase's test density; a test helper for Postgres that doesn't have the
same "refuses anything that isn't provably disposable" property
`config.Sandbox()` has for the filesystem garden.

**Phase to address:** Should be its own early phase (a testing-infrastructure
phase preceding or bundled with the first phase that writes to Postgres) —
every subsequent phase (queue, migration, AGE) depends on this harness
existing, and retrofitting it after several phases have already written
untested or unsafely-tested Postgres code is much more expensive than
building it first.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems, sized for this
project's actual scale (~50 events/day, one gardener, several machines).

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|-----------------|------------------|
| Point the drain worker's writes directly at Postgres with no idempotency key, "because retries are rare" | Ships faster, simpler wire format | Silent duplicate rows the first time a crash lands between remote-commit and local-ack (Pitfall 2) | Never — the key costs almost nothing to add up front and is expensive to retrofit onto already-written rows |
| Use `pg_dump`/`pg_restore` for the AGE graph the same way as for everything else, with no graph-specific check | One restore code path instead of two | Restores that "succeed" while leaving the graph unqueryable (Pitfall 13) | Only if the restore drill's verification step includes a live Cypher query — otherwise never |
| Test Postgres-touching code against a real, shared Postgres instance because a sandbox doesn't exist yet | Unblocks feature work immediately | Violates the project's own hermetic-test rule, and real-garden-touching tests are exactly the failure `config.Sandbox()` exists to prevent (Pitfall 19) | Never, for anything that runs via `go test`; acceptable only for a clearly-labeled manual smoke-test script that is not part of the test suite |
| Run the restore drill once, during the phase that builds it, and call it done | Phase closes on schedule | Drill decays with schema/AGE-version drift and nobody notices until a real restore is needed (Pitfall 7) | Never as a permanent state — acceptable as the *first* run, provided a recurring trigger ships in the same phase |
| Batch import rows in one large transaction for import speed | Faster one-shot migration | One bad row aborts the whole batch, or a swallowed error silently drops all of it (Pitfall 16) | Acceptable only with per-row `SAVEPOINT` isolation inside the batch — never as one all-or-nothing statement |
| Skip building a `hugel events` read command because `psql` can already query Postgres directly | No new CLI surface to build | Loses the "grep it while everything else is down" property with nothing to replace it (Pitfall 15) | Acceptable temporarily only if a person, not automation, is expected to always have working `psql` access at incident time — not a safe assumption for a single gardener under stress |

## Integration Gotchas

Mistakes specific to *this* project's specific integrations — not generic
Postgres advice.

| Integration | Common Mistake | Correct Approach |
|-------------|-----------------|-------------------|
| `pgx` (pure-Go driver) | Assuming pure-Go means no operational surface — treating the connection as always-available and letting `hugel gate` block on it | Writes always go through the local queue first; `pgx` connection failures degrade reads only, per the project's own explicit constraint |
| Apache AGE | Running `pg_upgrade` on a database with AGE installed, the default habit for any other Postgres upgrade | Dump/restore only for major-version upgrades on this database (Pitfall 12); check for the AGE extension before ever running `pg_upgrade` |
| Apache AGE + psycopg-style transaction helpers (if any Python tooling is ever added alongside the Go binary) | Using a `with connection.transaction():`-style helper inside an already-open transaction, silently creating a savepoint instead of a new transaction | Know explicitly whether AGE DDL-like calls (`create_graph`, label creation) are running inside an existing transaction, and that their catalog effects aren't visible to other sessions until that outer transaction commits |
| `bd` (beads) via bead ids carried in Postgres | Treating bead ids in Postgres as a second source of truth and letting them drift from bd's own state | Postgres carries bead ids only, read-only relative to bd; bd stays authoritative, per the project's own constraint |
| Local write queue → Postgres | Treating a successful local enqueue as equivalent to a successful remote write anywhere downstream that reads Postgres directly | Any code that reads "the current state" from Postgres must account for writes that are queued locally but not yet drained — otherwise reads on one machine can appear to regress relative to writes just made on that same machine |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|-----------------|
| Untested Cypher patterns against an effectively-empty graph | Queries feel instant in development | Benchmark against a synthetically populated graph before shipping any query into the draw path (Pitfall 14) | The day the graph actually accumulates a few thousand edges from real substrate-widening work |
| Parallel/batched drain "for throughput" | Looks like a sensible optimization to write | Keep the drain worker single-threaded, strictly FIFO — there is no throughput problem at ~50 events/day to justify the ordering risk (Pitfall 3) | Never breaks from *lack* of parallelism at this volume; only ever breaks by *adding* it |
| Full-table `pg_dump` used as the only restore mechanism for a graph meant to be a droppable/replayable projection | Restore appears to work in the common case | Prefer drop-and-replay from base tables (events, draws, entries, bead ids) for the graph specifically, keeping `pg_dump` for the tables that are the actual source of truth | Whenever the graph's schema and the base tables' schemas drift out of sync, which a dump/restore of both together can silently paper over |

## Security Mistakes

Domain-specific to a shared, multi-machine, single-gardener Postgres instance
— not generic web-app security.

| Mistake | Risk | Prevention |
|---------|------|------------|
| One shared Postgres credential embedded in a config file synced across "several machines" with no per-machine or per-actor distinction | A leaked credential from one machine grants full access from anywhere, and the project's own "actor id on every row" design goal (for future team use) has nothing to attach abuse to | Provision credentials such that the actor id already required for future team use is meaningful from day one — even a single gardener's several machines can each carry a distinct actor/connection identity, so the mechanism is proven before a second human ever needs it |
| Restore drills performed against a scratch Postgres instance that's reachable from the same network as production, with no isolation | A drill script pointed at the wrong DSN by mistake could restore-over or query the real garden | Scratch/drill Postgres instances should be trivially distinguishable from the real one (different port, different host, or ephemeral container) so a misconfigured drill fails loudly rather than touching the real garden |
| Backup files (`pg_dump` output, WAL archives) stored with weaker access control than the live database, because "it's just a backup" | A leaked backup file is a leaked complete copy of everything the garden has ever recorded, including anything not yet redacted at ingestion | Treat backup artifacts as carrying the same sensitivity as the live database, since `internal/redact` already establishes that not everything reaching the pile is safe to expose |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|--------------|-------------------|
| `yield --health` grows a field for every new subsystem (queue, restore, Postgres reachability) with no unifying "is everything actually fine" summary | The gardener has to mentally AND together five booleans every time they check health, and will eventually stop checking closely | Keep a single top-level healthy/unhealthy verdict that's true only when every constituent check is true, with the detail fields available for drill-down — exactly the shape `events.Health` already uses (`Healthy` derived from `Reachable && FailingSince == nil`), extended rather than replaced |
| A drain-in-progress state with no visible indication while `hugel garden` is open | The gardener can't tell "queue is draining normally" from "queue is stuck" by looking at the surface they sit in front of all day | Surface queue depth and last-drain time somewhere in the persistent `hugel garden` view (planned as session-persistent this milestone), not only in a separate `--health` command the gardener has to remember to run |
| Restore-drill failures reported only in a log file nobody reads | A silently-failing drill is exactly as useless as no drill, restated for the UX layer | Route drill failures through the same attention mechanism the project already has for everything else needing a human (`needs-attention` via bd), rather than inventing a second place to look for problems |

## "Looks Done But Isn't" Checklist

- [ ] **Local write queue:** Often missing a drain-health surface distinct from
  write-health — verify `yield --health` reports queue-drain staleness, not
  just event-append success.
- [ ] **Idempotent drain:** Often missing an idempotency key assigned at
  enqueue time — verify a forced crash-and-replay of the drain worker
  produces no duplicate rows in Postgres.
- [ ] **Backup automation:** Often missing restore verification — verify a
  restore was actually performed against a disposable instance recently
  enough to matter (see Pitfall 7's cadence discussion), not just that a
  backup file exists.
- [ ] **AGE graph restore:** Often missing a live-query check — verify the
  restore procedure runs at least one real Cypher query against the restored
  graph and checks its answer, not just that `pg_restore` exited 0.
- [ ] **One-shot migration:** Often missing reconciliation — verify source
  record counts and destination row counts are compared and matched
  explicitly, and that a sample of records is diffed field-by-field, not just
  counted.
- [ ] **Postgres major-version upgrade path:** Often missing an AGE-awareness
  check — verify the upgrade procedure does not call `pg_upgrade` on a
  database with the AGE extension installed.
- [ ] **Postgres test coverage:** Often missing entirely, or covering only the
  happy path — verify a Postgres-equivalent of `config.Sandbox()` exists and
  that tests use it rather than a real, shared instance.
- [ ] **New health checks generally:** Often missing a "prove it can go red"
  test — verify each new health field has a companion test that breaks the
  underlying mechanism and confirms the check notices, following phase 01's
  own hard-won standard.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|-----------------|------------------|
| Duplicate rows discovered in Postgres from a retried drain (Pitfall 2) | LOW, if an idempotency key exists to identify duplicates after the fact; HIGH if it doesn't | With a key: query and delete duplicates keeping the earliest, then add the `ON CONFLICT` guard going forward. Without a key: reconstruct de-duplication from content-hash heuristics, accept some false merges, and add the key before this can happen again |
| A backup discovered not to restore, found during an actual emergency rather than a drill (Pitfall 6) | HIGH | Fall back to whatever the local write queue still holds undrained, plus the JSONL/markdown sources the Postgres substrate was originally projected from (the project's own "substrate is rebuildable" design is the actual recovery path here) — this is the scenario the milestone's replay/rebuild requirement exists to make survivable |
| A one-shot migration found, after the fact, to have silently dropped rows (Pitfall 11) | MEDIUM, if the original source files are untouched (they should be, since JSONL/markdown sources are never deleted by a migration) | Re-run the import from the original, never-deleted source files with the reconciliation check now in place, rather than trying to patch the gap from the already-imported, already-suspect Postgres state |
| A drain worker found to have been silently stalled for weeks (Pitfall 1) | LOW to MEDIUM depending on queue size | Fix the drain, let it catch up (it's a bounded, known-size local queue, not an open-ended stream), then add the health signal that would have caught this sooner |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|-------------------|----------------|
| Drain loop dies silently (1) | Local write queue phase | A test that kills the drain worker mid-run and asserts `yield --health` reports it |
| Duplicate delivery on retry (2) | Local write queue phase | A test that force-crashes the worker after remote commit but before local ack, then confirms no duplicate row after restart |
| Ordering violated by batched/parallel drain (3) | Local write queue phase | A test that enqueues writes with a strict sequence and asserts they land in Postgres in the same order |
| Unbounded queue growth (4) | Local write queue phase | A disk-full test on the queue path, mirroring the one `events.go` already has |
| Queue's own corruption (5) | Local write queue phase | A kill-mid-append test asserting the queue still loads cleanly, one bad record costing one record |
| Backup that doesn't restore (6) | Backup & restore phase | An automated restore into a disposable instance with row-count and content checks, not just backup-job exit code |
| Restore drill decays / never re-runs (7) | Backup & restore phase | A `yield --health` field for "restore last verified," with a defined maximum acceptable age |
| Timestamp/timezone corruption on import (8) | Migration/import phase | Per-record epoch comparison between source and destination for a sample, not just "looks right" |
| Ordering assumed from file position across multiple sources (9) | Migration/import phase | A written, explicit decision on cross-machine merge order, checked against a synthetic multi-machine clock-skew scenario |
| Convergence-key collisions across machines (10) | Migration/import phase | A pre-import collision report that must be resolved before the import runs |
| Import reports success while dropping data (11) | Migration/import phase | Source-count vs destination-count reconciliation as a hard gate on "import complete" |
| `pg_upgrade` incompatible with AGE (12) | AGE/graph phase (pin decision); Backup & restore phase (drill) | Upgrade tooling refuses `pg_upgrade` when AGE is present; drill exercises dump/restore across a version bump |
| `pg_dump` restore leaves graph unqueryable (13) | AGE/graph phase; Backup & restore phase | Restore drill runs a live Cypher query and checks its answer |
| Cypher performance cliff once edges exist (14) | AGE/graph phase | Benchmark against a synthetically populated graph before "graph in the draw path" ships |
| Losing `grep`/`jq` debuggability (15) | Substrate/Postgres migration phase | A `hugel events` read command shipped alongside the write-path change, usable without raw SQL |
| Bulk insert loses many rows for one bad row (16) | Local write queue phase (drain); Migration/import phase (one-shot) | Per-row isolation (savepoints) verified by a test with one deliberately malformed row in a batch |
| Losing git-diffable pile history (17) | Substrate/Postgres migration phase | An explicit, recorded decision on where pile content lives, with an audit table from day one if it moves into Postgres |
| New health checks that can't go red (18) | Every phase adding a health signal | A "break the mechanism, confirm the check notices" test per health field, not just a happy-path test |
| Untestable or unsafely-tested Postgres code (19) | Its own early testing-infrastructure phase, before or bundled with the first Postgres-writing phase | A Postgres-equivalent of `config.Sandbox()` exists and is what every Postgres-touching test uses |

## Sources

- `internal/events/events.go`, `internal/config/sandbox.go` (this repository)
  — grounded the queue and health-check pitfalls in the project's own existing
  durability and test-isolation mechanisms rather than generic advice.
- `.planning/PROJECT.md` (this repository) — milestone goal, constraints, and
  the recorded regret this milestone reverses.
- Web search, aggregated general findings (LOW-confidence individual sources,
  cross-checked against each other and against this project's own code):
  outbox-pattern and idempotent-consumer discussions (DEV Community, freeCodeCamp,
  `github.com/nikolayk812/pgx-outbox`); Postgres backup/restore and
  `pg_stat_archiver` failure-mode writeups (OneUptime, DEV Community, and
  related PostgreSQL backup guides); JSONL/timestamp import pitfalls (a
  published PostgreSQL-migration timestamp-corruption case study, PostgreSQL
  mailing list threads on timestamp import shifts); Apache AGE operational
  documentation (`github.com/apache/age`, Postgres Pro Enterprise docs,
  Microsoft Learn's Azure Database for PostgreSQL AGE overview,
  `matheusfarias03.github.io/AGE-quick-guide`); Testcontainers-based hermetic
  database testing writeups (multiple DEV Community posts on Go/Postgres
  integration testing).
- Confidence caveat: AGE version-support and `pg_upgrade`-incompatibility
  claims should be re-verified directly against `github.com/apache/age`'s own
  documentation at the point the AGE version is actually pinned, since this
  research relied partly on third-party vendor docs rather than the AGE
  project's primary documentation.

---
*Pitfalls research for: Hugel v0.2 "The Shared Garden" — file-backed CLI to
shared Postgres + Apache AGE + local write queue*
*Researched: 2026-09-11*

