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

Earliest days. One command works.

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
