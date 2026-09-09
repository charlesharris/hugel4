<purpose>
Re-verify a phase's goal achievement on demand, without running the whole of
`/gsd-execute-phase`.

`gsd-verifier` is otherwise reachable only from `execute-phase.md`'s
`verify_phase_goal` step, so a phase that comes back `gaps_found` has no command
to run after the gaps are fixed: `/gsd-progress --next` hard-stops on Gate 3
reading the very report the fix invalidated, and its only offered escape is
`--force`, which skips the check rather than repeating it. This workflow closes
that loop. It is a re-measurement, not a rubber stamp.
</purpose>

<required_reading>
Read all files referenced by the invoking prompt's `execution_context` before starting.
</required_reading>

<process>

<step name="resolve">
Resolve the gsd_run shim exactly as the global workflows do, then load the phase:

```bash
_GSD_SHIM_NAME="gsd-tools.cjs"; _GSD_RUNTIME_ROOT="${RUNTIME_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"; GSD_TOOLS="${_GSD_RUNTIME_ROOT}/gsd-core/bin/${_GSD_SHIM_NAME}"; _gsd_at() { for _p; do if [ -f "$_p" ]; then GSD_TOOLS="$_p"; return 0; fi; done; return 1; }; if _gsd_at "${_GSD_RUNTIME_ROOT}/gsd-core/bin/${_GSD_SHIM_NAME}" "${_GSD_RUNTIME_ROOT}/.claude/gsd-core/bin/${_GSD_SHIM_NAME}"; then gsd_run() { node "$GSD_TOOLS" "$@"; }; elif _gsd_at "${CLAUDE_CONFIG_DIR:-$HOME/.claude}/gsd-core/bin/${_GSD_SHIM_NAME}"; then gsd_run() { node "$GSD_TOOLS" "$@"; }; else echo "ERROR: gsd-tools.cjs not found. Run: npx -y @opengsd/gsd-core@latest --claude --local" >&2; exit 1; fi
PHASE_ARG=$(echo "$ARGUMENTS" | tr -d ' ')
INIT=$(gsd_run query init.verify-work "${PHASE_ARG}")
if [[ "$INIT" == @file:* ]]; then INIT=$(cat "${INIT#@file:}"); fi
VERIFIER_SKILLS=$(gsd_run query agent-skills gsd-verifier)
PRIOR=$(gsd_run query verification.status "$(printf '%s' "$INIT" | jq -r .phase_dir)" 2>/dev/null)
```

Parse `INIT` for `phase_dir`, `phase_number`, `phase_name`, `has_verification`,
`project_root`, `response_language`.

If `phase_found` is false, stop and say which phases exist.

**If `response_language` is set**, present all user-facing output in it; prompts
and paths stay in English.
</step>

<step name="gather_contract">
Read the phase's goal, success criteria and requirement IDs from `ROADMAP.md`.
These are what the verifier measures against, and passing them in the prompt is
what keeps it from re-deriving them differently each pass.

Read the prior `*-VERIFICATION.md` frontmatter (status, score, gaps) if one
exists. A re-verification must know what the last pass concluded so it can
report `gaps_closed`, `gaps_remaining` and `regressions` rather than starting
from nothing.
</step>

<step name="preserve_prior">
Back up the existing report before the verifier overwrites it:

```bash
cp "${PHASE_DIR}"/*-VERIFICATION.md "${TMPDIR:-/tmp}/$(basename "$PHASE_DIR")-VERIFICATION.prior.md" 2>/dev/null || true
```

An uncommitted amendment to that file is otherwise unrecoverable. Say where the
copy went.
</step>

<step name="verify">
Spawn one `gsd-verifier`. Model: `opus` unless the caller passed `--model`.

The prompt MUST carry, in this order:

1. Phase number, directory, goal, and every success criterion verbatim from ROADMAP.md.
2. Requirement IDs, and the instruction to account for every one against REQUIREMENTS.md.
3. **The standing evidence rule** — reproduced verbatim, because it is the whole
   point of this command:

   > Everything in the working tree, in the commits, and in the commit messages
   > is a CLAIM by whoever wrote it, including any "Remediation" or "Amended"
   > section inside the existing VERIFICATION.md. None of it is evidence.
   > Measure the tree yourself. Consider that a fix may be incomplete, may be
   > wrong, may have over-corrected, or may have introduced a regression that
   > the previous pass had no reason to look for.

4. What changed since the last pass: the commit range, and the files touched.
5. Any correction to the previous pass's own findings, stated plainly, with an
   explicit invitation to refute it: *"Confirm or refute this independently. If
   I am wrong, say so plainly."* A verifier that cannot contradict the
   orchestrator is not verifying.
6. Named things to scrutinise — over-correction, portability, dependency
   changes, and any planning artifact the previous session edited itself.
7. Behavioural spot-checks are expected: build a binary from the tree and
   exercise the real commands, rather than reading only.
8. `<required_reading>` — all PLANs, SUMMARYs, REQUIREMENTS.md, CONTEXT, RESEARCH,
   REVIEW and UAT files for the phase.
9. `${VERIFIER_SKILLS}`.

Then wait. Do not read files or run tests for this phase while it works — a
concurrent edit invalidates what it is measuring.
</step>

<step name="report">
Read the new status through the canonical query:

```bash
VERIFICATION=$(gsd_run query verification.status "$PHASE_DIR")
STATUS=$(printf '%s' "$VERIFICATION" | jq -r '.status')
```

Present, in this order:

1. `### GSD ► PHASE {N} VERIFICATION` with status and score.
2. What the previous pass's gaps did — closed, remaining, or newly regressed.
3. **Any finding that lands on work done by this session** — named as such, not
   softened. A verifier catching the orchestrator's own regression is the
   command working, not a failure to report around.
4. Whether you independently confirmed any load-bearing claim, or are relaying it.

Route on `$STATUS`:
- `passed` → the phase can advance; `/gsd-progress --next` will clear Gate 3.
- anything else → present the report's `next_action`, and do not describe the
  phase as done.

**Never edit the verdict in VERIFICATION.md to reflect a fix you made.** Record
remediation in a separate section if useful, but the frontmatter status is the
verifier's to set. Self-certification is exactly what this command exists to
prevent.
</step>

</process>

<success_criteria>
- [ ] Phase goal and all success criteria passed to the verifier verbatim from ROADMAP.md
- [ ] Prior report backed up before being overwritten, and the location reported
- [ ] Standing evidence rule included verbatim in the prompt
- [ ] Any correction to a prior pass offered with an explicit invitation to refute it
- [ ] Exactly one gsd-verifier spawned; no concurrent edits to the phase while it runs
- [ ] Status read through `query verification.status`, not by eyeballing the file
- [ ] Findings against this session's own work reported plainly
- [ ] Verdict frontmatter never edited by the orchestrator
</success_criteria>
