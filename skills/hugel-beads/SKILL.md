---
name: hugel-beads
description: Draft work as beads and epics with bd, review them with the user, then file them. Use when planning what to build, breaking an idea into tasks, splitting an epic, or when the user says to file, plan, or write up work — and before starting any task not already tracked as a bead.
---

# Drafting beads

`bd` tracks work: beads, dependencies, and a ready queue that background tenders
pull from. A bead is also future knowledge — closing one with a reason is what
the pile composts — so a bead written well is paid for twice.

## Draft first, file second

Propose the beads in the conversation before creating anything. An agent that
files work unreviewed fills the queue with intentions, and a queue nobody
trusts is worse than a list nobody wrote. Show the user titles, types, and how
they depend on each other; file only what they agree to.

```
bd create "title" -t task -p 1 -d "the description"
bd create "title" -t feature --parent <epic-id>
bd dep add <blocked> <blocker>        # blocker must finish first
bd ready                              # what could be started now
```

Types: `epic`, `feature`, `task`, `bug`, `chore`, `decision`. Priorities are
0–4, 0 highest. `bd create` prints the new id; hierarchical children come back
as `<epic>.1`, `<epic>.2`.

## What makes a bead worth filing

**The description carries the why.** A title says what; the body says why this
and not the obvious alternative, and what it is trading away. That reasoning is
what a tender needs to do the work and what the pile keeps afterwards. A
description that restates the title in longer words has recorded nothing.

**Say what done looks like.** Use `--acceptance` where it can be stated. A bead
a reviewer cannot check is a bead a reviewer will approve.

**One reviewable unit each.** If a bead cannot be finished and reviewed in one
sitting, it is an epic with children hiding in it. Split it and record the
dependencies — an ordering left implicit is an ordering that gets lost.

**Order is an argument.** What blocks what says which risk is being taken first.
Say so in the description when the ordering is the interesting part.

## Before drafting

Ask the pile what has been decided about this area already:

```
hugel4 soil "what the work is about"
```

Plenty of proposed work is a decision someone already took, or already
abandoned. Filing it again is how a backlog fills up with settled questions.

## What not to do

- Don't file speculative work. A bead nobody will start is a note; write it in
  the description of a real bead instead.
- Don't close beads you didn't do, and don't close one without `--reason`. The
  reason is the compostable part — `bd close <id> --reason "..."` writes the
  knowledge, and closing without one throws it away.
- Don't create a bead for something already tracked. Check `bd list` first.
