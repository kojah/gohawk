---
description: Use when asking where a gohawk check loses real bugs, why a check stays silent on a corpus, which decline reasons eat the most candidates, or when replaying a check against fix commits to measure recall.
metadata:
    source: project
name: gohawk-recall-audit
---

# gohawk recall audit

A precision audit labels findings the analyzer reported. It says how often a
report is wrong, and never how often the analyzer stayed silent when it should
not have. A recall audit labels that silence at the point where the analyzer
decided to stay silent, and turns it into a ranked list of where a better
model would pay off.

It reuses the precision audit's corpus: the same pinned repositories, scan
profile, build-cache and disk guards, and static-analysis-only rule. Follow
[the precision audit](../gohawk-precision-audit/SKILL.md) and
`benchmarks/precision/README.md` for those; this skill covers only what is
different.

## Recall labels never feed the gate

Precision labels feed `make precision-regression`, and a failing label starts
the failure ladder. Recall labels are exploratory. Keep them out of every
precision TSV and cohort, and never let a recall result become a gate input.
Records live under `benchmarks/recall/`, not `benchmarks/precision/`.

A missed bug is not a licence to widen a check. It goes through the same
[failure ladder](../gohawk-analyzer-change/SKILL.md#3-apply-the-failure-ladder) as any other change: add
coverage only for a reusable structural contract with exact value provenance
and feasible-path semantics, and otherwise record it as an accepted false
negative. Without this rule a recall audit becomes pressure to add special
cases, which is exactly what the precision policy forbids.

## First separate the two kinds of silence

A conservative check that reports nothing on a corpus has produced a zero with
two very different meanings:

- **No candidates seen.** The corpus had nothing to look at. A repository with
  no `sync.RWMutex` says nothing about a read-lock check, and a clean run on it
  is empty rather than clean.
- **Candidates seen and all declined.** The check looked and gave up, and its
  `considered` and `decision` reason codes say which rule stopped it.

Only the second kind is a recall question. Select repositories where the check
has candidates, but do not select for the shape the check reports.

## Stage 1: cutoff census

Scan the corpus with tracing on and count every candidate by its final
decision and reason code, per check. This costs almost nothing when the scans
already run.

```sh
gohawk -json -gohawk-include-tests -enable-checks=CHECKS \
  -gohawk-trace=ANALYZERS -gohawk-trace-file=TRACE ./...
```

Count each candidate once, by check, candidate, reason, and outcome, and only
in the repository's own source (no `vendor/`, standard library, or module
cache). A `decision` event gives the outcome; the `considered` events before it
say which proofs were tried and did not hold. The result is one row per check:

| Check | Candidates | Reported | Top decline reasons |
| --- | ---: | ---: | --- |
| `resourcelifetime/missing-release` | … | … | reason-a 38%, reason-b 21%, … |

Candidate definitions differ per analyzer. Every `os.Open` is easy to count;
every lock pair is not. Compare counts within a check, never across checks.

**A tracing gap is a finding.** A check that emits no `candidate` or
`decision` events cannot take part in the census. List such checks explicitly
in the record, and add the missing events with
[the tracing skill](../gohawk-analyzer-tracing/SKILL.md) before guessing at why
the check is silent.

## Stage 2: label a sample of declined candidates

For the top decline reasons of a check, sample 10–20 declined candidates per
reason with a fixed seed, and label each one:

- **missed** — a real defect on a feasible path that the check should own;
- **correctly declined** — the code is sound, or the ownership really is
  ambiguous;
- **out of class** — a real defect, but not the kind this check targets.

That gives every reason code a missed-bug rate. A reason covering 38% of
candidates with a 2% missed rate is doing its job; one covering 9% with a 40%
missed rate is where recall is lost. Pin revisions and record each label's
source link, as a precision audit does. Labelling is expensive, so sample
rather than exhaust.

## Stage 3: ground-truth replay

Stages 1 and 2 cannot see a bug the analyzer never registered as a candidate,
because its obligation finder did not recognize it. Fix commits supply that
ground truth: the revision before a fix is a labelled defect, and its message
says what the defect was. `scripts/mine-race-fixes.py` gathers fix commits and
replays a check against the parent revision. Two rules keep the result honest:

- **Seed on the symptom, never the mechanism.** Search for "fix fd leak" or
  "fix data race", not for the syntax a check keys on, such as a missing
  `Close` or an `RLock` that became a `Lock`. Seeding on the mechanism
  pre-filters the corpus to bugs shaped like the detector, and the resulting
  number restates the detector's own assumptions.
- **Label, then count.** Review says whether each candidate is in the check's
  class. That yields prevalence, which decides whether the check is worth
  having, alongside recall, which decides whether it works.

A revision that does not build is recorded as unanalysable, not as a miss.

Mutation testing -- injecting the defect into correct code -- measures whether
an implementation holds up across code shapes. It cannot measure class
coverage, because the mutation generates exactly the pattern the check looks
for.

## Recording the audit

Write `benchmarks/recall/<scope>-<date>.md` with a TSV beside it, and commit
it as `record <scope> recall audit`, separately from any analyzer change it
motivates. [The cycle-check audit](../../../benchmarks/precision/audits/cycles-2026-09-24.md)
is an earlier example of the shape. The record states:

- the binary revision and hash, the corpus pins, and the scan command;
- repositories with no candidates, kept apart from those that declined;
- checks that could not be censused because of a tracing gap;
- the ranked table of (check, reason code, share of candidates, sampled missed
  rate), which is the output that matters;
- for each missed bug, its source link and whether a reusable structural
  contract would cover it, or why it stays an accepted false negative.
