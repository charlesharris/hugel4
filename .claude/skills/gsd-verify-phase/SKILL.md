---
name: gsd-verify-phase
description: "Re-verify a phase's goal achievement on demand — the fix→re-verify half of the GSD loop"
argument-hint: "[phase number, e.g. '01'] [--model opus|sonnet]"
allowed-tools:
  - Read
  - Bash
  - Glob
  - Grep
  - Agent
---

<objective>
Re-measure whether a phase achieved its goal, and rewrite VERIFICATION.md with
the result.

Use after fixing gaps a previous verification found, or any time the standing
verdict is older than the tree it describes. `/gsd-progress --next` reads that
verdict at Gate 3, so a phase whose gaps are fixed stays blocked until something
re-measures — this is that something.

This is not a rubber stamp. The verifier is told to treat the working tree, the
commits and their messages as claims rather than evidence, and is explicitly
invited to contradict the session that invoked it.
</objective>

<when_to_use>
- A phase came back `gaps_found`, the gaps have been fixed, and the phase needs to advance
- `/gsd-progress --next` hard-stops on Gate 3 citing a report that predates the fix
- Work landed on a phase whose last verification is now stale
- Before shipping, when the phase's own record is the thing being trusted

**Not** for validating features against user expectations — that is
`/gsd-verify-work` (UAT), a different instrument. UAT passing does not make a
phase verified, and phase verification does not replace UAT. This session's
phase 01 passed UAT 19/19 while verification stood at `gaps_found`.
</when_to_use>

<execution_context>
To load this command's workflow spec: check for `.claude/gsd-core/workflows/verify-phase.md`
relative to the current working directory first (project-local); if it is not there,
fall back to `~/.claude/gsd-core/workflows/verify-phase.md`. If neither exists, stop.
@~/.claude/gsd-core/references/ui-brand.md
</execution_context>

<process>
Follow the workflow spec end-to-end. Preserve every gate: prior-report backup,
the verbatim evidence rule in the verifier prompt, the invitation to refute, and
the prohibition on the orchestrator editing the verdict.
</process>
