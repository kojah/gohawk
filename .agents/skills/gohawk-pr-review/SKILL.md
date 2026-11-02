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

### 3.6 Labels

Labels record the shape so pull requests can be filtered. Each prefix is one
dimension:

| Dimension | Values | Count | Source |
|---|---|---|---|
| `category:` | `improves-precision`, `improves-recall`, `new-check`, `reliability`, `performance`, `refactor`, `docs`, `tooling` | exactly one | 3.2 |
| `scope:` | `general-model`, `pattern-rule`, `special-case` | one, analyzer changes only | 3.3 |
| `area:` | `concurrency`, `resources`, `engine`, `catalog` | one or more | changed paths |
| `touches:` | `shared-proof`, `flow-query`, `output-format` | zero or more | section 2 |
| `risk:` | `low`, `medium`, `high` | exactly one | section 6 |
| `needs:` | `accepted-fixture`, `negative-control`, `decision-record`, `docs`, `ai-disclosure`, `intent` | zero or more | sections 4 and 9 |
| `hunt:` | `regression-found`, `slowdown-found` | set by `/hunt-for-regressions` | 5.1, 5.2 |

A behavior change mixed with a refactor takes the behavior's category.
`category:improves-recall` is for an existing check; a new check ID is
`category:new-check`.

Who sets each label:

- **`PR labels` workflow** (`scripts/pr-labels.sh`, on every open, edit and
  push): `area:` from changed paths; `category:` from the template's single
  ticked Intent box, or `docs`/`tooling` when every changed file is one of
  those; `needs:intent` and `needs:ai-disclosure` while those template
  sections are empty, removed once they are filled.
- **`pr-classify` agentic workflow** (when a pull request opens, after the
  above): `category:` when still unset, `scope:`, and `touches:`.
- **Reviewer**: `risk:` and the remaining `needs:` labels, removing a `needs:`
  label once its evidence arrives.
- **`/hunt-for-regressions` workflows**: `hunt:` labels, cleared when a
  rerun finds nothing.

Workflows only fill a dimension that has no label yet. A label a maintainer
set wins: when your reading disagrees with it, say so in the review instead
of relabelling.

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

## 5. Tests: added, and falsifiable

Adding a fixture is not the same as pinning behavior. Rate the test evidence:

- **Fails without the change.** Revert only the production change, or check out
  the parent commit, and confirm the new fixture fails. A fixture that passes
  either way is vacuous.
- **Asserts the intended claim.** Exact position and expected diagnostic, not
  "some output changed".
- **Has a negative control.** A widened rule needs an accepted case that must
  stay silent. A new silent shape needs a diagnostic sibling.
- **Fails for the intended reason.** The fixture must trip the check under
  review, not a neighbouring one.
- **Is minimal.** One behaviour per case, with no incidental setup the analyzer
  could trip over instead.

Rate `strong`, `partial`, `vacuous`, or `missing`, and name the failure mode
each fixture pins.

### 5.1 Hunt a counterexample

Passing fixtures show the rule works on the cases the author thought of. Try to
break it with cases they did not. This is the check that found the reacquire
regression in #58 while its own fixtures passed.

- **Build both sides.** Use a worktree at the merge base and one at the PR head,
  and build each binary once. Run the same input through both.
- **Start from the changed decision.** Reacquire after a stale saved result,
  sibling branches that share held state, loop re-entry and repeated calls,
  aliasing through embedded fields or interfaces, deferred release on only some
  returns, early exits, each result of a multi-value call, closures, and
  build-tag variants.
- **Classify each candidate** by what the two binaries say:

  | Before | After | Meaning |
  |---|---|---|
  | correct | wrong | regression — report it |
  | wrong | correct | the intended fix |
  | wrong | wrong | a gap, not a regression |
  | correct | correct | no change |

- **Confirm the mechanism** with the evidence trace, so the report explains why
  it happens rather than only that it does.
- **Bound the claim.** Say which shapes you tried and which you did not. Finding
  nothing proves nothing; it only narrows the risk statement in section 6.

Exploratory code stays out of the tree. A counterexample that holds becomes a
minimal regression fixture, either on the PR or as a follow-up.

### 5.2 Hunt a slowdown

Correct output is not enough: gohawk runs on every package in a build, so a
change that is fast on fixtures can still be unacceptable on real code. Assess
the cost of the change and try to construct an input that makes it blow up.

- **Read the cost from the diff first.** For every new loop, traversal, cache,
  or fixed point, name the input size it scales with and its bound: per
  instruction, per function, per call edge, per points-to set, or whole
  program. Flag nested traversals, repeated walks of the same function or
  call graph, recursion without a visited set or memo, caches keyed so they
  never hit or never release, and new work outside an existing proof budget.
- **Measure both sides.** With the merge-base and PR-head worktrees from 5.1,
  run `make benchmark BENCHMARK_ARGS='--only self --runs 3'` in each, under the
  same `scripts/with-ram-go.sh` workspace settings
  ([performance workflow](../../../docs/development/performance-workflow.md)).
  Compare runtime, CPU, and peak memory; add `--profile` when a delta needs
  explaining. Add a manifest entry that exercises the changed analyzer when
  `self` does not.
- **Construct an adversarial input.** Grow the axis the new code scales with
  and nothing else: a huge switch or package initializer, deep nesting, a long
  call chain, wide structs, many closures or defers, large method sets,
  recursive types, or many aliases of one value. Use the generators in
  `internal/testsupport/stress` as the starting point. Run both binaries at
  two or three sizes (for example 1x, 4x, 16x) and compare how time and memory
  grow, not just their values at one size.
- **Judge the growth.** A constant-factor slowdown on ordinary code needs a
  stated reason. Growth worse than linear along an axis that was linear
  before, unbounded memory, or a shape that only stops at a timeout is a
  regression. A shape that exceeds a proof budget must make the analysis give
  up on it, not grow with it ([stress suite](../../../docs/development/stress-suite.md)).
- **Bound the claim.** Say which axes you grew and which you did not.

#### Profiling report

Every analyzer change includes a before/after profile of the hot paths it
touches, on the same input and settings for both binaries:

- **CPU.** Run each binary with `GOHAWK_PROFILE_DIR=<dir>` and a fresh build
  ID or cleared vet cache (a cached unit records nothing). Summarize with
  `go run ./tools/profilesummary <dir>`, merge the per-unit `.pprof` files,
  and compare `go tool pprof -top -tagfocus=analyzer=<name>` for each changed
  analyzer. Report the changed functions' flat and cumulative time and any
  new entry in the top of the profile.
- **Allocations.** Run the touched analyzer's tests with `-run '^$' -bench .
  -benchmem` (or add a benchmark over its fixtures when none exists), or
  collect `-memprofile` from one, and compare allocs/op and bytes/op. Name
  each new allocation on a per-instruction, per-state, or per-edge path:
  slice and map clones, closures, and boxed values.
- **Un-memoized work.** For every new query on a hot path, state whether its
  result is cached and its key. A query whose answer depends only on stable
  inputs (a block, an edge, a value, a function) but runs once per walk state,
  per path, or per caller multiplies by the number of states that reach it.
  Count the repeats on the adversarial input when the diff cannot bound them.
- **Budget spend.** Compare `gohawk dump budget` for the functions the change
  touches. New work that draws from an existing search budget can exhaust it
  earlier, turning a slowdown into a silent false negative instead.

Put the report in the review as a small table: function, before, after, and
the input it was measured on.

A pathological shape that holds becomes a generator case in the stress suite,
either on the PR or as a follow-up; run `make stress` when the change touches
shared proof, the flow query, or the points-to model.

## 6. Risk of accepting

State the risk in one line, before the verdict:

- **Worst case if wrong.** Silent false negative, new false positive, crash, or
  a fix that changes semantics elsewhere. gohawk treats a silent miss as the
  expensive one.
- **Blast radius.** Which other analyzers or packages consume the changed
  proof, helper, or engine rule.
- **Detection path.** How the regression would surface after merge: a labelled
  corpus position, dogfooding, a user report, or nothing. No signal means
  higher risk.
- **Reversibility.** Clean revert, or persisted state, catalog IDs, or output
  changes that outlive it.
- **Performance.** New work on large inputs, budget spend, allocations, and
  the worst growth found in 5.2.
- **Determinism.** Map iteration order, concurrency, timing.
- **Compatibility.** Supported Go versions, JSON output, diagnostic text.

Template: `Risk: medium — shared lifecycle proof consumed by three analyzers;
failure mode is a silent FN; detected by the next precision round; clean
revert.`

## 7. Adjudicating automated review comments

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

## 8. What to run

```sh
make verify                                                 # local completion
make verify-static                                          # fast static set
go test ./internal/analysis/analyzers/<group>/<name>/...    # focused iteration
make lint
```

A maintainer can run the hunts in sections 5.1 and 5.2 on GitHub by
commenting `/hunt-for-regressions` on the pull request. The
`hunt-counterexample` and `hunt-slowdown` agentic workflows build the merge
base and head, follow those sections, and each post one comment; text after
the command is passed to both as extra instructions. Edit the workflows' `.md` sources and recompile with
`gh aw compile`; never hand-edit the `.lock.yml` files.

Run `-race` and `-cover` only in CI. Do not pass them locally, including
through `GOFLAGS`, and do not spoof CI variables. `make verify` is the local
entry point; report pending coverage and race results separately.

## 9. Reject reasons

Request changes when the PR:

- widens recall without accepted fixtures for the new silent shapes;
- adds a project-name or function-name exemption without a documented general
  contract;
- grows production code with no new accepted fixture and no removed special
  case;
- adds a fixture that also passes without the production change;
- widens a rule without a negative control that must stay silent;
- changes an existing fixture's expected output and does not say so;
- mixes a behavior change with a refactor that could stand alone;
- suggests a suppression comment as the fix;
- hand-edits generated files instead of running `make generate`;
- changes a shared proof or helper for a single analyzer;
- has more than one implementation owner on one proof;
- changes a check without updating the catalog or its docs page;
- leaves the flow query or proof policy changed with no decision record;
- introduces nondeterministic output or an unjustified performance regression;
- adds work whose cost grows worse than linearly, or without bound, on an
  input shape that 5.2 can construct;
- documents behavior the code does not implement;
- omits the AI-usage disclosure or the precision/recall intent.

## 10. Verdict

Start every review with the shape block, then approve or request changes naming
the missing evidence and the section above it violates.

```text
Production: +120/-8   Tests: +240/-0   Docs: +30/-2   Misc: +4/-0 (2 generated)
Intent: precision (false-positive fix)
Scope: general evidence model in the classifier; no name exemptions
Refactor: none
Complexity: +112 production lines buy 6 new accepted fixtures and remove one special case
Tests: 2 regression fixtures (both fail on the parent commit), 3 negative controls
Probe: reacquire, sibling arm, loop re-entry; one regression found (finding 1)
Perf: self +3% wall, +0 peak; lockorder walk +8% CPU, +2 allocs/state; switch arms 1x/4x/16x grow linearly; call-chain depth not tried
Risk: medium — shared lifecycle proof, silent-FN failure mode, corpus-detectable, clean revert
```

"Needs more tests" is not a review. Say which fixture, which boundary, or which
document is missing.
