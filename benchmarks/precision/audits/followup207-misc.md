# Miscellaneous follow-up: map snapshots and conditional retention

The original 18 miscellaneous false-positive labels remain in the frozen input.
This checkpoint fixes one report and corrects the review of another; it does
not claim that all remaining sites have a solution.

## Nanobot: JSON map snapshot uncertainty

- Pin: `obot-platform/nanobot@f809ba0d358d402f19b8911782374d6f875a449d`.
- Original false positive: `pkg/config/load.go:192:38`,
  `evalorder/operand-mutation`.
- Implementation: `8855530`. A short-declared fresh plain map, used only as
  an earlier whole-map operand and the exact JSON destination, does not prove
  a stale header. Object decoding updates shared entries. The classifier
  declines the report as unknown; it does not infer the payload's shape.
- Deliberate coverage loss: JSON `null` can reset that map. Nil maps, custom
  decoders, explicit replacements, earlier entry reads, and other uses of the
  destination remain checked. This is one bounded predicate at the existing
  operand decision point, not a JSON data-flow model. The existing AST analyzer
  remains one file because these checks share its source-object and operand
  vocabulary; its size was reviewed rather than introducing a second proof path.
- Actual SSA: `.build/followup207-nanobot-toMap.ssa`. Canonical all-check/test
  output: `.build/followup207-nanobot-evalorder-v1.json`; trace explicitly says
  `map-header-mutation-unknown`. Against the previous pinned canonical output,
  the only evalorder delta is removal of line 192. The line-230 struct decoder
  diagnostic remains.
- Binary: `.build/gohawk-followup207-evalorder-v1`, SHA-256
  `d2baf648454162fe99ceeb747c9674f5b77d1f20fc10628497531198be28962a`.
- Validation: paired fixtures, structured trace test, race test, lint,
  architecture, and `make verify` pass. The cumulative evalorder-only precision
  replay preserves all seven true-positive labels and all six prior false-positive
  absences. Its overall command still fails on the pre-existing round-10 label
  count mismatch, documented in `followup-337-historical-validation.md`.
  Agent-beacon has an unrelated incompletely loadable embedded-assets package;
  its labelled evalorder package is successfully recovered and checked. No
  historical labels or expected counts were changed to force the gate to pass.

## Sloth: original false-positive judgment corrected

- Pin: `slok/sloth@8a3be4fab79defa4448d09d91b48422615980b05`.
- Site: `cmd/sloth/commands/generate.go:204:4`,
  `deferinloop/cleanup-lifetime`.
- Original judgment: output handles enter `genTargets` for later generation,
  so deferring Close to function return is intentional collection ownership.
- Revised judgment: **true-positive hazard on the empty-input path**. Discovery
  in `helpers.go:21-60` checks extensions and path filters, not file contents.
  `SplitYAML` in `pkg/common/utils/data/data.go:33-50` removes empty/comment-only
  segments and can return an empty slice. For each such input, `os.Create`
  still opens the output, but the append loop never runs. That handle has no
  later generation use and remains open across the outer loop. Repeating empty
  YAML files can therefore accumulate unnecessary descriptors.
- The useful nonempty-input path is not being disputed. A repair could defer
  output creation until at least one segment exists, or promptly close unused
  outputs; blindly closing every output before generation would be wrong.
- Actual SSA: `.build/followup207-sloth-run.ssa`, block 53 registers Close,
  then block 54 can go directly to the outer loop without entering block 55's
  append. The minimized `conditionallyRetainedOutputs` fixture preserves this
  diagnostic alongside accepted unconditional retention fixtures. Focused
  tests pass. No analyzer behavior was changed for this review correction.

Candidate repositories were only statically analyzed. Their tests, generators,
and applications were not executed. Historical labels are preserved; this
review correction is distinct from a code fix or a verified diagnostic absence.
