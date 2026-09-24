---
description: Use when changing a gohawk analyzer, fixing a false positive or false negative, responding to a failed precision label, or extending a classifier or proof.
metadata:
    source: project
name: gohawk-analyzer-change
---

# Changing a gohawk analyzer

gohawk values precision over recall: a diagnostic must be actionable, and an
uncertain case is reported as nothing. This procedure keeps a change inside
that contract. For a brand-new analyzer, follow
[How to contribute](../../../docs/contributing.md) instead; this skill covers
the change, fix, and extend loop.

## Implementation ownership

Use one implementation agent per task, including large false-positive batches.
That agent may be the main agent or one delegated fixer, but not both at once.
It owns production changes, shared infrastructure, regression fixtures, and
associated documentation across all affected analyzers. Do not split fixes
among concurrent implementers merely because they touch different analyzers:
shared proof changes can interact even when file ownership does not overlap.

Additional agents may perform read-only investigation, source review, or
precision audits and return evidence or proposed fixes to the implementation
agent. They may write isolated scan artifacts, but must not edit shared source,
tests, or documentation. The main agent coordinates and reviews rather than
becoming a second implementer when a fixer is delegated. Transfer implementation
ownership explicitly and stop the previous writer before the next starts.

Parallel implementation requires explicit user approval; a request to finish
a batch quickly or resolve every finding is not that approval.

## 0. Orient on the real IR

Do not mentally compile Go to SSA or simulate the classifier. Dump what the
analyzer actually sees, then reason about that:

- `gohawk ssa -func NAME ./pkg` for the SSA of the function under study.
- `gohawk facts ./pkg` for the exported lifecycle summaries.
- `gohawk -gohawk-trace=ANALYZER ./pkg` for the evidence trace, with SSA text.

Details in [debugging](../gohawk-debugging/SKILL.md).
When a changed precision boundary needs instrumentation, follow
[gohawk-analyzer-tracing](../gohawk-analyzer-tracing/SKILL.md).

## 1. Locate the decision point

Lifecycle analyzers share one shape. Find which function owns each stage in
the analyzer at hand:

1. **Obligation finder** — what was promised, resolved to exact SSA values.
2. **Classifier** — every later instruction labelled once as `join`,
   `transfer`, `unknown`, or `none`. `unknown` is any consumption the analysis
   cannot see through; it suppresses, it is never a weak join.
3. **One flow query** — honored when exact actions cover every return,
   unknown when only opaque actions do, violated otherwise.

Nearly every change is a stage-2 change. The flow query should not change.

## 2. Reassess the check

Before patching a false-positive class, ask whether the check's question can
be answered reliably with the evidence available. Repeated false positives
trigger a check-level reassessment, not automatically another exception.

- **Expand the model** when the missing evidence expresses a generalizable,
  bounded structural contract. Explain how it distinguishes valid code from
  the defect without project, naming, timing, or framework guesses.
- **Narrow the check** when only a useful subset has reliable evidence.
  State the proven boundary and abstain outside it; accept the coverage loss.
- **Retire the check** when its central distinction depends on unavailable
  intent or runtime behavior, or maintaining precision requires an open-ended
  catalog of special cases and no useful bounded subset remains. Retire the
  affected check, not unrelated focused checks in the same analyzer.

Retirement or demotion is a recommendation until the user explicitly approves
it. Explain the evidence and coverage or default-enablement change, then ask
before removing, disabling, or demoting a check (including moving it to an
experimental or opt-in tier). Approval to investigate or fix false positives
does not authorize these changes.

Record the chosen direction, representative evidence, what is knowable, and
the expected complexity and coverage tradeoff. If evidence is insufficient,
record the unresolved question and investigate before expanding the model.
Do not equate a missing model with a fundamentally unmodelable question, or
treat every false positive as justification for a larger proof engine.

## 3. Apply the failure ladder

For a check retained after reassessment, respond to a failed precision label
in this order and stop at the first step that holds:

1. Widen `unknown` at the classifier.
2. Accept the false negative: delete the fixture and record the gap in the
   fixture file's header comment.
3. Add one structural predicate at an existing decision point, with a fixture
   and a commit-pinned link to the real-world pattern.

Never add a new proof file, a loop-count argument, a name, or a framework
guess. A default diagnostic needs positive structural evidence of both an
obligation and its violation; the absence of a recognized cleanup proves
nothing.

## 4. Implement with the shared vocabulary

Read [gohawk-codebase](../gohawk-codebase/SKILL.md) and the
[shared helpers](../gohawk-codebase/references/shared-helpers.md) before writing
traversal code. Facts are consumed through `lifecyclefacts.LifecycleEvidence`,
never by importing raw facts; see [Inferred facts](../../../docs/development/fact-model.md)
for what a fact can prove and the polarity each mask must keep.

### Extend the lowest appropriate semantic layer

Before implementing analyzer-local reasoning, ask whether it can be composed
from existing shared building blocks. If a building block is insufficient,
consider extending its owning layer rather than reproducing the mechanism
downstream where sibling consumers cannot use it.

Prefer reusable evidence and queries; keep diagnostic policy and check-specific
precision boundaries beside the analyzer. Do not promote code merely because
it could theoretically be reused, or use shared infrastructure to bypass the
failure ladder above.

Place code according to its meaning, not its first caller. A package should
have a coherent vocabulary and responsibility. If new logic does not fit,
refactor it into the appropriate existing package, or introduce a focused
package when there is a genuine semantic boundary. Preserve dependency
direction: shared infrastructure must not depend on its consumers.

Lower is not automatically better: result guarantees do not all belong in
`heapmodel` merely because it supplies identity evidence. Reuse evidence
upstream and expose each guarantee in the layer that owns its semantics.

## 5. Fixtures

- Both forms for every boundary: the diagnostic case and the accepted case,
  close together in the same testdata package.
- Any name-based heuristic gets an accepted fixture with a misleading but
  plausible name.
- Mark expected diagnostics with `// want "message"`; unmarked code must be
  accepted.
- Put the commit-pinned link in the rationale comment at the decision point,
  once, not in every helper.

## 6. Validate

- `make verify` — the local gate. It regenerates the derived documentation
  first, so a helper you added or renamed updates the generated index in
  place instead of failing `generated-check`; commit the regenerated pages
  with your change. Only prose that cites renamed code still fails, and that
  needs a human edit.
- `make lint` — includes `funlen`, `gocognit`, `cyclop`, `lll`, and `dupl` at
  60 tokens.
- `go test ./internal/architecture/` — the enforced invariants.
- `make precision-regression` — the replay runs with every check enabled, so
  noise from an opt-in audit fails the gate like a default check.
- `make test`.

A new diagnostic starts as an opt-in audit. Promote it to a default check only
after its false-positive classes are fixtured; retire an audit whose labels
keep failing rather than refining it.

## 7. Working in a shared checkout

Other sessions commit from this same working tree. Never switch branches in
it; never `git add -A`; never revert a file you did not change; stage and
commit only your own files after reading `git status`.
