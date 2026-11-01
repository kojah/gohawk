---
description: Use when reviewing a gohawk pull request, adjudicating an automated review comment, or deciding whether an analyzer change is safe to merge.
metadata:
    source: project
name: gohawk-pr-review
---

# Reviewing a gohawk pull request

A gohawk change is acceptable only when its precision claim is supported by
evidence, not merely plausible. This is the reviewer's procedure. For writing
the change itself, use [gohawk-analyzer-change](../gohawk-analyzer-change/SKILL.md);
this skill assumes that procedure was followed and checks the result.

Reviewing classifier logic without the analyzer's accepted-boundary list is
guessing. Load the boundary first.

## 1. Load the boundary before the diff

- `docs/development/analyzers/<analyzer>.md` — accepted and reported
  boundaries, the fixtures that pin them, known gaps.
- `docs/development/decisions/` — the decision record when the PR moves a
  boundary that already has one.
- `benchmarks/precision/round-N/labels.csv` — existing verdicts for the check,
  so a claimed fix can be compared against what was actually labelled.
- `analyzers/catalog_specs.go` — the catalog entry the check must match.

## 2. Review in this order

1. **Catalog.** Check ID, `Kind`, `Tier`, `Doc`, and `Help` in
   `analyzers/catalog_specs.go` match what the analyzer now does and what the
   public page claims.
2. **Implementation.** Identify which stage moved: obligation finder,
   classifier, or flow query. A classifier change is the normal case; a change
   to the flow query or to shared proof is a red flag that needs its own
   justification.
3. **Fixtures.** Accepted cases first. Every newly reported shape needs an
   accepted sibling that must stay silent. Fixtures live under
   `testdata/src/<scenario>/` as `accepted.go` and `diagnostics.go` (or a
   `diagnostic/` directory).
4. **Docs.** The analyzer note and the public page agree with the new boundary.
5. **Precision impact.** Which cohort or existing labels change, and does the
   PR say so?
6. **Repository rules.** One implementation owner (see `AGENTS.md`), generated
   files untouched, directory file limits respected.

## 3. Evidence the PR must show

| Change | Required evidence |
|---|---|
| Classifier widening | The SSA or fact dump the decision was read from, plus an accepted fixture for every new silent shape |
| New check | Catalog entry, public page, analyzer note, accepted and diagnostic fixtures, and one real-world pattern |
| False-positive fix | The code shape that misfired and the accepted fixture that now covers it |
| Shared proof change | Which analyzers share the proof and why they are unaffected |
| Docs only | `make generated-sync` output clean; no tracked generated page hand-edited |

The PR description must disclose AI assistance, per
[`docs/ai-policy.md`](../../../docs/ai-policy.md), and should name which row
above applies.

## 4. Adjudicating automated review comments

Greptile, Copilot, and similar tools review the diff; they do not know the
accepted-boundary list. Treat every comment as a hypothesis.

- **Reproduce before applying.** Express the supposed bug as a fixture. If it
  cannot be expressed as one, it is not a finding.
- **Accept** only when the comment names a concrete feasible path to a
  violation. Speculative lifecycle reasoning is rejected; uncertainty is
  reported as nothing.
- **Reject style, naming, and formatting** suggestions outright.
  golangci-lint owns those.
- **A real false positive** is fixed with an accepted fixture or a broader
  evidence model. Never with a suppression comment, a `//nolint`, or a
  project/function-name exemption.
- **When a comment contradicts a documented boundary**, reply with the note
  and resolve the thread. Do not silently widen the analyzer to satisfy it.
- **Never** let a review comment change shared proof for one analyzer.

## 5. What to run

```sh
make verify                                                 # local completion
make verify-static                                          # fast static set
go test ./internal/analysis/analyzers/<group>/<name>/...    # focused iteration
make lint
```

Run `-race` and `-cover` only in CI. Do not pass them locally, including
through `GOFLAGS`, and do not spoof CI variables. `make verify` is the local
entry point; report pending coverage and race results separately.

## 6. Reject reasons

Request changes when the PR:

- widens recall without accepted fixtures for the new silent shapes;
- adds a project-name or function-name exemption without a documented general
  contract;
- suggests a suppression comment as the fix;
- hand-edits generated files instead of running `make generate`;
- changes a shared proof or helper for a single analyzer;
- has more than one implementation owner on one proof;
- changes a check without updating the catalog or its docs page;
- leaves the flow query or proof policy changed with no decision record;
- omits the AI-usage disclosure.

## 7. Verdict

Approve, or request changes naming the missing evidence and the section above
it violates. "Needs more tests" is not a review. Say which fixture, which
boundary, or which document is missing.
