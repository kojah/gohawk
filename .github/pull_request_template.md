## Summary

<!-- What changed, and why? -->

## Intent

<!-- Tick the one that best describes the change. -->

- [ ] **Precision**: fewer wrong diagnostics (a false-positive fix or narrowed rule)
- [ ] **Recall**: an existing check reports a shape it used to miss
- [ ] **New check**: adds a check ID
- [ ] **Reliability**: crashes, hangs, nondeterminism or budgets, with no boundary change
- [ ] **Performance**: speed or memory, with output unchanged
- [ ] **Refactor**: no behavior change
- [ ] **Documentation**: docs, site or talks only
- [ ] **Tooling**: CI, scripts, workflows or dependencies

## AI assistance

<!-- Required by docs/ai-policy.md: which tools you used and for what, or "None". -->

## Verification

<!-- List the commands you ran and any manual checks you performed. -->

- [ ] Tests cover behavior changes and important regressions.
- [ ] Generated documentation is up to date, if applicable.
- [ ] The change has been tested against realistic Go code, if applicable.

## New or changed analyzers and checks

<!-- Delete this section when it does not apply. -->

- Is this primarily a correctness or reliability issue, or an opinionated
  policy/style preference?
- Does a popular Go linter already provide the same check?
- How much noise or how many false positives is it likely to create in real
  codebases?
- Are there real-world bugs this would have caught? Links are helpful but
  optional.

<!-- Note any changes to diagnostic behavior, check IDs, tags, profiles, or flags. -->
