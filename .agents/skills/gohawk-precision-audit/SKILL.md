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
3. **Investigate before widening coverage.** A false positive is a precision
   defect. Fix it through the failure ladder in
   [gohawk-analyzer-change](../gohawk-analyzer-change/SKILL.md): widen
   `unknown`, or accept the false negative, or add one structural predicate
   with a fixture and a commit-pinned link. Minimize the pattern into a local
   fixture rather than copying the external repository into the test suite.
4. **Record the batch.** Append the batch to `audits/README.md` and commit it
   as `record batch-N precision audit`, separately from any analyzer change it
   motivated.

## Reading a replay failure

The replay runs with every check enabled, so an opt-in audit that becomes
noisy fails the gate exactly like a default check. When a label fails:

- If the analyzer changed, decide whether the new behaviour is the intended
  precision improvement (update the label) or a regression (fix the analyzer).
- If the label was wrong, correct it and say why in the audit record.
- An audit whose labels keep failing should be retired, not refined.

## Guard rails

- Prefer stable false negatives over an analyzer whose precision depends on an
  open-ended catalog of framework and naming conventions.
- Do not add project-name or function-name exemptions unless they represent a
  documented, general API contract.
- Do not use suppression comments as a substitute for fixing a recurring
  false-positive pattern.
