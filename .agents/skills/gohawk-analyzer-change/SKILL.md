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

## 0. Orient on the real IR

Do not mentally compile Go to SSA or simulate the classifier. Dump what the
analyzer actually sees, then reason about that:

- `gohawk ssa -func NAME ./pkg` for the SSA of the function under study.
- `gohawk facts ./pkg` for the exported lifecycle summaries.
- `gohawk -gohawk-trace=ANALYZER ./pkg` for the evidence trace, with SSA text.

Details in [debugging](../gohawk-debugging/SKILL.md).

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

## 2. Apply the failure ladder

When a reviewed precision label fails, respond in this order and stop at the
first step that holds:

1. Widen `unknown` at the classifier.
2. Accept the false negative: delete the fixture and record the gap in the
   fixture file's header comment.
3. Add one structural predicate at an existing decision point, with a fixture
   and a commit-pinned link to the real-world pattern.

Never add a new proof file, a loop-count argument, a name, or a framework
guess. A default diagnostic needs positive structural evidence of both an
obligation and its violation; the absence of a recognized cleanup proves
nothing.

## 3. Implement with the shared vocabulary

Read [gohawk-codebase](../gohawk-codebase/SKILL.md) and the
[shared helpers](../gohawk-codebase/references/shared-helpers.md) before writing
traversal code. Facts are consumed through `lifecyclefacts.LifecycleEvidence`,
never by importing raw facts; see [inferred facts](../../../docs/development/fact-model.md)
for what a fact can prove and the polarity each mask must keep.

## 4. Fixtures

- Both forms for every boundary: the diagnostic case and the accepted case,
  close together in the same testdata package.
- Any name-based heuristic gets an accepted fixture with a misleading but
  plausible name.
- Mark expected diagnostics with `// want "message"`; unmarked code must be
  accepted.
- Put the commit-pinned link in the rationale comment at the decision point,
  once, not in every helper.

## 5. Validate

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

## 6. Working in a shared checkout

Other sessions commit from this same working tree. Never switch branches in
it; never `git add -A`; never revert a file you did not change; stage and
commit only your own files after reading `git status`.
