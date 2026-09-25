---
description: Use when running a gohawk precision round or dogfood batch, labelling findings on real repositories, reading a precision-regression failure, or recording a batch audit.
metadata:
    source: project
name: gohawk-precision-audit
---

# gohawk precision audit

The precision audit is how gohawk earns the claim that every diagnostic is
actionable. Analyzers are run against real repositories, each finding is
labelled, and the labels become a regression gate. This is the most frequent
recurring task in the repository, so keep it mechanical.

## Where things live

- `benchmarks/precision/README.md` — the precision regression cohorts and how
  a round is structured.
- `benchmarks/precision/audits/README.md` — the 500-repository audit, one
  section per batch, with the labelled findings and their verdicts.
- `benchmarks/precision/round-N/` — the artifacts of each round.

Those READMEs are the authoritative procedure; this skill is the checklist
around them.

## The loop

1. **Run the cohort.** `make dogfood` runs the analyzers against the
   representative repositories; `make precision-regression` replays the
   labelled cohort as a gate.
2. **Label every new finding** as a true positive, a false positive, or an
   accepted false negative. A label is a reviewed judgement about a real
   pattern, not a guess.
3. **Reassess before fixing.** Group false positives by evidence family and,
   for each affected check, apply
   [Reassess the check](../gohawk-analyzer-change/SKILL.md#2-reassess-the-check):
   expand the model, narrow the check, or retire it. Repeated noise requires
   this check-level assessment, not another suppression. Record representative
   findings, the missing evidence, whether a bounded general model is
   possible, and the chosen direction with its complexity and coverage
   tradeoff. Mark unresolved assessments explicitly rather than guessing.
   Record proposed retirement or demotion as pending user approval; ask before
   removing, disabling, or demoting the check, as required by analyzer-change.
   For retained checks, follow that skill's failure ladder and minimize the
   pattern into a local fixture instead of copying the external repository.
4. **Record the batch.** Append the batch to `audits/README.md` and commit it
   as `record batch-N precision audit`, separately from any analyzer change it
   motivated.

## Parallel large-batch audits

For large batches, use two subagents when available so scanning and triage do
not wait on fixes:

- **Audit agent:** owns cloning, scanning, incremental triage, and batch audit
  records. Hand off false positives as they are reviewed, then continue with
  the remaining repositories without waiting for a fix.
- **Fix agent:** groups incoming false positives by evidence family, applies
  the reassessment above, and owns authorized analyzer changes and minimized
  regression fixtures. Assess related reports together rather than adding one
  exception per repository. Retirement and demotion still require approval.
- **Main agent:** assigns non-overlapping file ownership, reviews uncertain
  verdicts, coordinates shared infrastructure changes, and verifies the final
  replay and separate audit/fix commits. Agents share the checkout; do not
  overwrite one another's edits or switch its branch.

Keep the audit binary, its revision/hash, repository pins, and scan profile
fixed for the entire batch. Build corrected binaries at separate paths; never
replace the executable used by an in-progress scan. Keep candidate execution
limited to static analysis: do not run their tests, generators, or applications.

Each handoff should include the repository/SHA, analyzer/check and source
location, diagnostic, relevant cleanup or ownership path, why it appears false,
and any uncertainty. Persist that evidence in the audit artifacts, not only
agent messages. Keep original scan verdicts distinct from fix status and replay
results; a proposed fix does not make a false positive resolved.

Replay affected pinned cases with the corrected binary, alongside nearby true
positive controls, before recording a fix as verified. Bound scan and replay
concurrency together so competing work does not exhaust CPU or memory. If
delegation is unavailable, keep the same handoff records and finish triage
before working through the fix queue.

## Reading a replay failure

The replay runs with every check enabled, so an opt-in audit that becomes
noisy fails the gate exactly like a default check.

A repository that could not be analysed is reported as `unscannable:` and its
labels are excluded from the counts rather than reported as lost. A module
that fails to build contributes no findings, so every label in it would
otherwise read as a lost true positive, and every false-positive label in it
would read as a pass nobody earned. Such a repository needs its checkout
fixed or its labels dropped; `REQUIRE_SCANNABLE=1` turns it into a failure
once a cohort is clean.

Read the provenance first. Every failure prints when its label was last
confirmed, such as `(last confirmed at af2b29b on 2026-08-14)`, or
`(provenance unknown)` for a label written before the field existed. A label
confirmed on the commit you branched from is evidence your change broke it. A
label last confirmed long ago, or one whose provenance is unknown, may have
drifted for reasons that have nothing to do with your change, so establish
that before bisecting: replay the cohort with a binary built from the commit
you started from, and compare. Then:

- If the analyzer changed, decide whether the new behaviour is the intended
  precision improvement (update the label) or a regression (fix the analyzer).
- If the label was wrong, correct it and say why in the audit record.
- An audit whose labels keep failing should be retired, not refined.

`make precision-regression STAMP=1` records the running revision on every
label that still holds, so provenance means "last confirmed at" rather than
"first written at". Stamp from a clean tree: a stamp taken from a modified
worktree is recorded as `<revision>-dirty`, because it names a commit that
does not contain the behaviour it certifies.

## Measuring what the analyzer missed

A cohort labels only what the analyzer reported, so it measures precision and
cannot measure recall. See [the recall audit](../gohawk-recall-audit/SKILL.md).

## Guard rails

- Prefer stable false negatives over an analyzer whose precision depends on an
  open-ended catalog of framework and naming conventions.
- Do not add project-name or function-name exemptions unless they represent a
  documented, general API contract.
- Do not use suppression comments as a substitute for fixing a recurring
  false-positive pattern.
