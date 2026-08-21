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

Earliest days. Three commands work.

## `hugel yield`

What sessions cost, and how much of that was hauling context rather than
producing work.

```
hugel yield                    spend rolled up by bed (last 30d)
hugel yield --all              no time limit
hugel yield --sessions         one line per session, dearest first
hugel yield --session ID       request-by-request, to find a spiral
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

Stage two, extraction into typed pile entries, is not built yet.

## `hugel pile`

The knowledge store: typed entries composted out of sessions, shared across
every bed.

```
hugel pile init              create the pile and its git repository
hugel pile import <dir>      take in legacy markdown entries
hugel pile list [--stats]    what the pile knows
hugel pile show <id>         one entry in full
```

Lives at `~/.hugel/pile` unless `HUGEL_PILE` says otherwise. **The files are the
source of truth.** Any index built over them is derived and disposable,
rebuildable by re-reading the directory. An earlier Hugel made a graph database
authoritative, which meant the knowledge died with a container volume.

Git is the storage mechanism, not something you operate: hugel initialises the
repository and commits what it writes. It supplies the temporal layer for free —
superseding an entry is a commit, and an entry's lineage is its log.

Nothing that changes when an entry is merely *read* belongs in these files.
Usage counts live in the derived index, or every soil lookup would dirty the
repository.

Writes converge rather than duplicate. An entry's identity is its scope, type
and normalised title, so re-composting a session updates in place; an entry
whose content is unchanged is not rewritten at all, so a compost run that
learned nothing leaves no diff.

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
internal/cli/         thin command drivers, no domain logic
```

## Development

```
make build test vet
```

Go module is `github.com/charris/hugel`. Dependencies: none, so far.
