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

## 3. Report the shape of the change

Every review states these five things, even when the answer is "none".

### 3.1 Line breakdown

Bucket the diff by path (`git diff --numstat origin/main...HEAD`) and report
added/deleted per bucket:

| Bucket | Paths |
|---|---|
| **Production Code** | `main.go`, `generate.go`, `analyzers/**`, `internal/**`, `plugin/**`, excluding tests and `testdata/` |
| **Tests** | `*_test.go`, `testdata/**` |
| **Website/Docs** | `site/**`, `docs/**`, `talks/**`, and tracked Markdown outside code directories |
| **Misc** | `.github/**`, `Makefile`, `go.mod`, `go.sum`, `hyrule.yaml`, `.golangci.yml`, `.go-version`, `scripts/**` |

Call out generated files separately inside Misc: they inflate the line count
without adding reviewable logic.

### 3.2 Intent: precision, recall, or something else

Classify the PR as one of:

- **precision** — narrows or fixes reporting; the goal is fewer wrong
  diagnostics;
- **recall** — reports a shape that was previously silent;
- **reliability** — no boundary change: refactor, shared proof, or test
  infrastructure;
- **docs or tooling** — no analyzer behavior change.

The PR description must say which. If it does not, derive it and say that you
derived it. Cross-check the claim against the fixtures: a PR claiming precision
that adds only diagnostic fixtures is mislabelled and should be questioned.

### 3.3 Narrow pattern or general reliability

Say whether the change encodes a general evidence model, a pattern-specific
rule with a written justification, or a narrow special case for one shape.

A narrow case must name the documented general contract it represents
(`AGENTS.md` requires this). When it cannot, prefer one general model over
accumulating exceptions, or conservatively decline to analyze the pattern.

### 3.4 Refactors: neater or messier

Only when the PR contains refactoring. Judge it, do not just note it:

- **neater** — fewer concepts, code moved to the layer that owns it, special
  cases removed, duplicate traversal deleted;
- **messier** — new indirection, interfaces or generics with one implementation,
  file churn without ownership change, a helper that only one caller can use.

A behavior change mixed with a refactor is two PRs unless the refactor is a
prerequisite for the change; say which it is.

### 3.5 Complexity justified by value

Compare production lines against what they buy: new accepted fixtures, correct
coverage of a new shape, or removal of existing special cases. Growth without
one of those is not justified. Say what the added production code buys in one
sentence; if that sentence needs more than one clause, treat it as unjustified.

## 4. Evidence the PR must show

| Change | Required evidence |
|---|---|
| Classifier widening | The SSA or fact dump the decision was read from, plus an accepted fixture for every new silent shape |
| New check | Catalog entry, public page, analyzer note, accepted and diagnostic fixtures, and one real-world pattern |
| False-positive fix | The code shape that misfired and the accepted fixture that now covers it |
| Shared proof change | Which analyzers share the proof and why they are unaffected |
| Docs only | `make generated-sync` output clean; no tracked generated page hand-edited |

The PR description must disclose AI assistance, per
[`docs/ai-policy.md`](../../../docs/ai-policy.md), and should state the intent
from 3.2.

## 5. Adjudicating automated review comments

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

## 6. What to run

```sh
make verify                                                 # local completion
make verify-static                                          # fast static set
go test ./internal/analysis/analyzers/<group>/<name>/...    # focused iteration
make lint
```

Run `-race` and `-cover` only in CI. Do not pass them locally, including
through `GOFLAGS`, and do not spoof CI variables. `make verify` is the local
entry point; report pending coverage and race results separately.

## 7. Reject reasons

Request changes when the PR:

- widens recall without accepted fixtures for the new silent shapes;
- adds a project-name or function-name exemption without a documented general
  contract;
- grows production code with no new accepted fixture and no removed special
  case;
- mixes a behavior change with a refactor that could stand alone;
- suggests a suppression comment as the fix;
- hand-edits generated files instead of running `make generate`;
- changes a shared proof or helper for a single analyzer;
- has more than one implementation owner on one proof;
- changes a check without updating the catalog or its docs page;
- leaves the flow query or proof policy changed with no decision record;
- omits the AI-usage disclosure or the precision/recall intent.

## 8. Verdict

Start every review with the shape block, then approve or request changes naming
the missing evidence and the section above it violates.

```text
Production: +120/-8   Tests: +240/-0   Docs: +30/-2   Misc: +4/-0 (2 generated)
Intent: precision (false-positive fix)
Scope: general evidence model in the classifier; no name exemptions
Refactor: none
Complexity: +112 production lines buy 6 new accepted fixtures and remove one special case
```

"Needs more tests" is not a review. Say which fixture, which boundary, or which
document is missing.
