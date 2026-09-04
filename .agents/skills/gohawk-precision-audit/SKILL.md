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

A cohort labels findings the analyzer reported, so it measures precision and
cannot measure recall: silence is never labelled. That matters most for a
conservative check, where dogfooding produces a zero and the zero has two very
different meanings.

Separate them before drawing any conclusion:

- **No candidates seen.** The corpus had nothing to look at. A repository with
  no `sync.RWMutex` says nothing about a read-lock check, and a clean run on it
  is empty rather than clean.
- **Candidates seen and all declined.** The check is systematically blind, and
  the `considered` trace reasons say which rule is eating them.

Counting `considered` events by reason across a corpus turns silence into a
distribution and costs nothing beyond the tracing already required at a
precision boundary.

For real ground truth, `scripts/mine-race-fixes.py` replays a check against the
revision before a fix commit, where the message states what the defect was.
Two rules keep the result honest:

- **Seed on the symptom, never the mechanism.** Searching for the syntax a
  check keys on -- an `RLock` that became a `Lock` -- pre-filters the corpus to
  bugs shaped like the detector, and the resulting number restates the
  detector's own assumptions. Search for fixed races; let the shapes fall where
  they fall.
- **Label, then count.** Review says whether each candidate is in the check's
  class. That yields prevalence, which decides whether the check is worth
  having, alongside recall, which decides whether it works.

Mutation testing -- injecting the defect into correct code -- measures whether
an implementation holds up across code shapes. It cannot measure class coverage,
because the mutation generates exactly the pattern the check looks for.

## Guard rails

- Prefer stable false negatives over an analyzer whose precision depends on an
  open-ended catalog of framework and naming conventions.
- Do not add project-name or function-name exemptions unless they represent a
  documented, general API contract.
- Do not use suppression comments as a substitute for fixing a recurring
  false-positive pattern.
