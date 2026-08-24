---
name: hugel-soil
description: Draw prior knowledge from the hugel pile — decisions, patterns, failures and constraints composted out of finished sessions, in this project and every other one. Use before starting non-trivial work, when choosing between approaches, when a problem smells previously solved, or when asked what was decided, tried, or ruled out.
---

# Drawing soil from the pile

Hugel keeps a pile of typed entries composted out of finished sessions, shared
across every project. Soil is the small selected part of it delivered to one
piece of work. Drawing is read-only; it writes nothing.

## Draw

```
hugel4 soil "what the work is about" --bed "$(basename "$PWD")"
```

The bed is the project directory's name. It weights knowledge this project
earned above another project's, and picks up the names this project used to go
by. Pass it — without it every bed ranks alike, and another project's entry can
match the words perfectly while being the wrong knowledge.

Ask with the substance of the work, not keywords. Ranking is lexical, so use
words an entry would use: *"why the extractor only reads commit messages"*
beats *"extractor"*.

Flags worth knowing:

- `--budget N` — most tokens of soil to deliver, default 1500. Drop to ~600 for
  a narrow question. Soil that arrives early is re-sent on every later turn, so
  the budget is a real cost, not an allowance to spend down.
- `--type decision|pattern|constraint|discovery|failure` — narrow to one kind.
  `--type failure` before retrying something that may already have failed.
- `--limit N` — most entries, default 8.

## Then read, if it earns it

A draw is a survey: each entry returns as a heading, an excerpt, and an id.
Reading one in full is a second, deliberate step.

```
hugel4 pile show <id>
```

Follow up only on entries load-bearing for the decision in front of you.
Reading all eight spends the whole pile to save the budget.

## When not to draw

Every drawn token enters the session and is re-sent on every later turn. Draw
when prior knowledge would change what you do:

- starting non-trivial work in an unfamiliar area
- choosing between approaches
- hitting something that smells previously solved
- being asked what was decided, tried, or ruled out

Don't draw for a mechanical edit, for a question the open file answers, or a
second time on the same subject in one session.

## Judging what came back

If a drawn entry is wrong, stale, or replaced by something newer, say so to the
user and name the command — don't run it. Review is a human vouching:

```
hugel4 pile review <id> --reject --why "..."
hugel4 pile review <id> --superseded-by <newer-id>
```

Soil ranks vouched entries higher precisely because a person stood behind them.
An agent that accepts its own draws turns that weight into noise.

## What the entries are worth

Most were extracted mechanically and stand `unreviewed`; the few a human
vouched for rank higher and say so. An entry records what was true when it was
written and may have been superseded since — its lineage is a git log, not a
guarantee. Treat soil as a lead: check it against the code before acting, and
attribute it when repeating it.
