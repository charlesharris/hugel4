# hugel

Hügelkultur is the permaculture practice of burying woody debris to build a
raised bed that feeds itself for years. Hugel applies the same idea to agentic
software work: a session's exhaust — decisions, patterns, dead ends, failures —
is buried in a pile that feeds the next session, in this project or any other.

Design constraints, in priority order:

1. **Produce no waste.** Token thrift is a primary constraint, not an
   optimization. Anything that costs more to maintain than it saves gets pulled.
2. **Use small and slow solutions.** One binary, a laptop, SQLite. Three earlier
   attempts died of canopy before roots.
3. **Obtain a yield.** Cost per accepted change is visible, always.
4. **Observe and interact.** Measure before building. A feature that cannot be
   falsified does not ship.

## Status

Earliest days. Nine commands work.

## `hugel yield`

What sessions cost, and how much of that was hauling context rather than
producing work.

```
hugel yield                    spend rolled up by bed (last 30d)
hugel yield --all              no time limit
hugel yield --sessions         one line per session, dearest first
hugel yield --session ID       request-by-request, to find a spiral
hugel yield --soil             whether the pile is asked, and whether it was right
```

It reads Claude Code's session transcripts from `~/.claude/projects` and writes
nothing. Override the source with `--root` or `HUGEL_TRANSCRIPT_ROOT`.

### The number that matters

**Context tax** is the share of spend that went to re-reading the conversation
instead of producing output. Every token that enters a session is re-sent on
every later turn, so a token's true cost is set by *where it enters*, not how
big it is. A permanent preamble pays that multiplier on every turn whether or
not it is ever used; a subagent that forages in its own context and returns a
summary pays it on almost nothing.

Context tax above ~85% means the session is mostly paying to remember itself.

### Whether any of this works

`hugel yield --soil` reports the two numbers soil and its skill shipped without.

**Reach** is the share of sessions that asked the pile anything. Pull delivery
fires only when an agent recognises the moment, and that miss rate was the
stated cost of choosing a skill over a hook. If reach stays near zero, the
choice was wrong and the push version is the answer.

**Precision** is the share of delivered entries a human later kept. It counts
only entries someone has actually ruled on, so it cannot be flattered by
leaving the pile unreviewed — an unjudged entry is unknown, not good. It is
also the zero a model-backed extractor will have to beat.

Draws are recorded to `~/.hugel/draws.jsonl`, beside the pile rather than in it:
nothing that changes when an entry is merely read belongs in the pile's files.

## `hugel digest`

Stage one of composting: mechanical, free, bounded.

```
hugel digest --session ID      distil one session
hugel digest --all             size report across every session
```

A session becomes a few thousand characters describing what was asked, what
changed, what ran, and what broke. The bound is the point. A twenty-hour
session that read 350M tokens of context distils to the same ~4k tokens as a
five-minute one, so composting costs about the same whatever it is composting.
A composter that produces waste is not a composter.

Distillation is lossy on purpose and says so: every section reports what it
dropped. Prompts and agent notes are taken from both ends of the session —
intent lives at the start and outcome at the end, while the middle is the work
itself, which the file and command records already describe.

## `hugel compost`

Stage two: turn distilled sessions into pile entries.

```
hugel compost --session ID     compost one session
hugel compost --all            compost every session
hugel compost --all --dry-run  show what would be written
```

The first extractor is free and reads only **deliberate records** — commit
messages, beads closed with a reason, and `bd remember` memories. Those are things someone chose to
write down as a record, describing changes that demonstrably happened, so the
extractor reports rather than infers. Agent prose is richer and would yield more
entries, but prose is where an extractor invents things, and an invented entry
in a permanent cross-project pile is worse than a missing one.

A memory is the highest-intent record of the three: someone read a session,
decided one sentence of it should outlive the session, and wrote it down for no
other purpose. It is also the weakest evidence — a commit describes a change
that demonstrably happened, while a memory is an assertion with no diff behind
it, possibly written by an agent. So memories compost as `discovery` rather than
`decision`, at a lower confidence than a recorded change. bd keeps its memories
for priming; the pile keeps them for every other project.

That baseline matters beyond precision: any model-backed extractor has to beat
free entries a gardener actually keeps, and now there is a zero to measure it
against.

Entries arrive `unreviewed`, and this extractor always proposes `scope: bed`.
An earlier version guessed at generality and marked 89 of 138 entries general,
including dense project-specific notes whose only crime was arguing a decision
without citing a filename. Absence of evidence of bed-specificity is not
evidence of generality — and guessing wrong in that direction sends an entry
into every other bed's soil, which is what scope exists to prevent.

## `hugel garden`

Where the work is, across every bed.

```
hugel garden [--since 7d] [--limit 12]
```

The entry point. What is in flight, what is ready to start, what cannot move —
every bed at once, not just the one you are standing in. Tab switches to the
knowledge that came back from the last flight.

Work and knowledge are two panes of one surface rather than two programs,
because they are two halves of one sitting: you look at what is in flight, and
you judge what the last flight left behind. Each pane keeps its place, so a
glance at one does not cost your position in the other.

Within a bed the order is the question actually being asked — what is
half-finished and waiting on you, what could be started, what cannot move. Each
bed caps at `--limit` and says what it left out, so one bed with thirty ready
beads cannot bury the rest.

Work is read from bd and never written here. Starting, claiming and closing a
bead are bd's business; a verdict is the pile's. Pressing a judgement key on a
bead does nothing on purpose.

## `hugel tend`

The working surface: what the garden did lately, and the judgement you pass on
it.

```
hugel tend [--since 7d] [--bed NAME] [--limit 25]
```

Left is what happened, right is the entry in full. You judge without navigating
away — `a` keeps, `r` throws out, `x` abandons, `u` undoes a mis-key, and `s`
twice supersedes: once on the stale entry, once on the one that replaced it, so
nothing has to be typed. A sitting is one act of gardening and commits as one.

Entries from the project you are standing in are listed first, under every name
that project has had. Nothing is hidden — the pile is shared and another bed's
entries still need a verdict — but with a cap on each group, what is listed
first is what actually gets judged.

**Bounded by time, never by backlog.** There is no view of every unreviewed
entry, for the same reason `hugel pile review` has no inbox: a queue of hundreds
is not judged, it is abandoned. Sit down after three days away and you are shown
three days.

Two groups, and the order is the argument. **Delivered** entries were drawn into
a session and so have cost tokens rather than disk; they are judged first.
**New** entries were composted in the window. An empty Delivered group is left
in place, because "the pile was never asked" is a finding, not a blank.

Each group caps at `--limit` and says what it left out. A bulk import or a first
compost run drops hundreds of entries inside any window, and a truncated list
that looks complete is worse than a long one.

## `hugel hooks`

```
hugel hooks session-start      compost what has changed, before work begins
```

Composting runs on the way in, not the way out. The moment pile freshness is
worth anything is immediately before a draw, so collecting at the start of a
session means soil pulled during it already holds the last session's lessons.
It is self-healing too: a session that ends in a crash or a reboot is collected
by the next start, where an end-of-session hook would lose it silently.

Only transcripts written since the last run are parsed, because parsing is the
entire cost of composting. A full pass over 21 sessions takes ~1s; the steady
state is ~19ms, and stays there as sessions accumulate.

The hook writes nothing to stdout and always exits zero. A SessionStart hook's
output is prepended to the session's context and re-read on every turn, which
is the waste `hugel yield` exists to name; and a hook that can refuse to let
work begin is a worse problem than a stale pile.

Push is right here and wrong for delivery, which is worth saying plainly rather
than sliding past: composting writes to disk and costs a session nothing, while
soil enters a live context window and is paid for on every later turn.

## `hugel pile`

The knowledge store: typed entries composted out of sessions, shared across
every bed.

```
hugel pile init              create the pile and its git repository
hugel pile import <dir>      take in legacy markdown entries
hugel pile list [--stats]    what the pile knows
hugel pile show <id>         one entry in full
```

Lives in the garden, at `~/.hugel/pile`, unless `HUGEL_PILE` says otherwise.
The pile hangs off the garden rather than off your home directory, so
`HUGEL_HOME` moves the whole garden together — config, draw log, marks and
entries. **The files are the
source of truth.** Any index built over them is derived and disposable,
rebuildable by re-reading the directory. An earlier Hugel made a graph database
authoritative, which meant the knowledge died with a container volume.

Git is the storage mechanism, not something you operate: hugel initialises the
repository and commits what it writes. It supplies the temporal layer for free —
superseding an entry is a commit, and an entry's lineage is its log.

Nothing that changes when an entry is merely *read* belongs in these files.
Usage counts live in the derived index, or every soil lookup would dirty the
repository.

```
hugel pile review <id> --accept        vouch for it; soil ranks it higher
hugel pile review <id> --reject        it is wrong; keep it out of soil
hugel pile review <id> --abandon       what it describes is dead
hugel pile review <id> --unreview      take back a verdict
hugel pile review <id> --superseded-by <id>
```

Review is what turns an automated pile into trusted knowledge, and it is
deliberately **driven by draws, not by an inbox**. There is no command that
walks the 247 unreviewed entries, because working a queue that size costs more
than the pile saves — the first thing this garden refuses to build. The entries
soil actually surfaced are the only ones whose quality has cost anything, and
you are holding the context to judge them exactly then.

A verdict can be taken back. `--unreview` returns an entry to unreviewed rather
than to the opposite verdict — not knowing is a real state, and pretending
otherwise is how a wrong keystroke becomes a fact. The TUI's `u` does the same
thing.

Status and review answer different questions. `--reject` says the entry is
wrong; `--abandon` says the approach it faithfully records is dead. Both drop
out of soil, and only one is a complaint about the entry.

Superseding sinks the old entry rather than deleting it — it was true once, and
why it stopped being true is worth reaching. The link is written on the newer
entry, pointing back, because that is the direction a reader travels.

The reason goes in the commit message, not in the entry. The pile's git history
is already its temporal layer, so why something was thrown out costs nothing to
keep there and would cost a schema field anywhere else.

Writes converge rather than duplicate. An entry's identity is its scope, type
and normalised title, so re-composting a session updates in place; an entry
whose content is unchanged is not rewritten at all, so a compost run that
learned nothing leaves no diff.

## `hugel soil`

Draw context from the pile.

```
hugel soil "what the work is about" [--bed NAME] [--budget 1500]
```

Soil is not the pile — it is the small, selected part of it delivered to one
piece of work, and delivering less of it is the whole discipline. **The budget
is the feature.** An unbounded lookup that returned everything relevant would
reproduce the problem soil exists to solve: context that arrives early, stays
for the session, and is re-read on every turn.

Retrieval is BM25, local, no embeddings and no service. At this size a good
ranking function beats a vector database, and the pile has to prove it deserves
one before it gets one.

The bed defaults to the working directory's name, which is how beds are named
everywhere else: a bed is the basename of the directory a session ran in. It is
a default where a bed *weights* — soil's ranking, tend's ordering — and
deliberately not where a bed *filters*, since defaulting a filter to the working
directory lets a command run from the wrong place hide everything rather than
show nothing.

Relevance alone is the wrong ranking for a shared pile — another project's entry
can match the words perfectly and still be the wrong knowledge. So lexical score
is weighted by three things a search engine has no reason to care about: whether
this bed earned the knowledge, whether a human vouched for it, and how long ago
it was true. Rejected and abandoned entries are dropped rather than
down-weighted; knowledge someone threw away should not surface at all.

One entry cannot spend the whole budget. Soil is a survey of what the pile
knows; reading an entry in full is a second, deliberate step.

## The soil skill

Nothing draws soil unless something asks for it, and the cheapest thing that
can ask is the agent itself.

```
make install        hugel4 on PATH, and the skill into ~/.claude/skills
```

The skill is the pull path: an agent decides it needs prior knowledge, draws a
budgeted survey, and reads in full only what earns it. That is deliberately not
a SessionStart hook. A preamble enters at turn zero and is re-sent on every
later turn whether or not it was ever relevant — the exact waste `hugel yield`
exists to name. A skill costs its name and description until it fires, and what
it draws enters mid-session, where fewer turns remain to re-pay it.

The cost of pull is that it only fires when the agent recognises the moment.
That miss rate is real, and it is measurable, which is why it is the first thing
tried rather than the fallback. If it proves too lossy, the push version hooks
`UserPromptSubmit`, not `SessionStart` — the prompt is the query, and at session
start there is nothing to ask about yet.

It installs as `hugel4`. An earlier generation still owns `hugel` on PATH and is
wired into live hooks; it keeps the name until it retires.

## `hugel bed`

```
hugel bed kin hugel4 hugel hugel-core    record that these are one project
hugel bed list                           where each bed lives, and what work is open
```

A bed is named for a working directory, and until now that name was all hugel
kept — enough to group knowledge, not enough to find the project again.
Transcripts record the cwd of every session, so the mapping is derived rather
than configured, and the most recent session wins: a project that moved leaves
sessions under both paths, and the one worked in last is the one that exists.

Open work comes from **bd**, which owns it. hugel reads and never writes: bd is
the work model — beads, dependencies, a ready queue — and hugel is the knowledge
and accounting model. Two views of one tracker beat two trackers.

`blocked` is derived rather than stored, the same way bd derives it, so hugel
cannot invent a fourth status nobody else uses. Readiness is bd's answer, asked
for directly, because recomputing it here would drift on defer dates and gates
away from the queue tenders actually pull from.

Beds with pile entries but no recorded directory are named at the foot of the
table. An imported corpus, or a project not worked in since hugel started
watching, has knowledge with nowhere to point — better said than quietly left
out of the count.

A project renamed across rewrites leaves knowledge filed under every name it
ever had. Without kinship, soil in the new bed treats the old bed as another
project's business — so a project's oldest and most settled decisions rank
lowest exactly because they are old. Measured, not hypothetical: declaring
kinship moved the entry that actually answered a question from sixth place to
first.

## Two things the transcripts get wrong if you read them naively

- **Usage is duplicated.** Claude Code writes one record per assistant content
  block, each carrying the *same* usage snapshot for the request. Summing
  records instead of requests overstates cost by ~40%. Requests are deduped by
  `requestId`.
- **Cache writes have two prices.** The 5-minute TTL bills at 1.25× input, the
  1-hour TTL at 2×. Older transcripts report a flat `cache_creation_input_tokens`
  with no TTL split; those are attributed to the cheaper tier so an unknown TTL
  never inflates the bill.

Prices are Anthropic list rates compiled into `internal/pricing`. If you are on
a subscription plan these are *equivalent API cost*, not what you were charged —
still the right signal for waste, not a bill.

## Layout

```
cmd/hugel/            entrypoint
internal/transcript/  harness session logs -> requests and usage
internal/pricing/     usage -> dollars
internal/yield/       accounting and roll-ups
internal/draws/       what the pile was asked, and what it handed over
internal/tend/        the working surface
internal/beads/       reading work from bd
internal/cli/         thin command drivers, no domain logic
skills/hugel-soil/    the agent-facing way to draw soil
```

## Development

```
make build test vet
make install
```

Go module is `github.com/charris/hugel`. Dependencies: Bubbletea and lipgloss,
for `hugel tend`, and nothing else. Everything that is not the terminal surface
still stands on the standard library.
