# Kind enum boundaries in authored Go

Bead: `gohawk-dho.23.31`. Parent: `73b459e`.

The user identified a raw `kind string` parameter. Source verification found
`metadataEffectProof` dispatching value/field/closure queries by raw strings in
an SSA regression test. Documentation example regions also used flagged/OK
strings as their internal enum, including a named result from marker parsing.
Both domains now use defined numeric enums, with strings restricted to test
names, marker syntax and error presentation.

The existing architecture reason rule scanned production source and reason
roles only. `TestNoRawKindEnums` now uses a test-inclusive view of the shared
source inventory. It rejects raw kind fields, parameters, named results and
literal assignments. The same walker retains fixture/generated exclusions and
stable deduplication; existing production-only callers retain their scope.
Before source corrections, the new gate failed at six real locations, including
the reported helper and documentation fields/results. Positive/negative matcher
controls and an inventory inclusion/exclusion test pin the new boundary.

CLI error wording is a noun, and the lock identity test stores an expected
output prefix; those fields now name their textual roles. Neither dispatches
production proof policy by its value. The documentation tests assert the typed
marker classification and unchanged exact unclosed-marker wording. Existing
metadata cutoff/mutation controls continue to execute the same query families.

Six parent/current CLI outcomes are byte-identical: two successful catalog lists
and four invalid/repeated selection errors. Focused architecture, SSA, example,
CLI and lock tests pass; final canonical verification passes all eight gates.
Receipts are `.build/goal-kind-enum-parent-gate.log`,
`.build/goal-kind-enum-cli/comparison.json`, and
`.build/goal-kind-enum-verify-final.log`. Generated documentation has no output
changes. No analyzer proof/reporting behavior or production FP is changed.

The syntax rule does not resolve inferred string types through aliases or
arbitrary expressions. Remaining named string-backed closed domains are tracked
in `gohawk-dho.23.32`: catalog check kind/tier, evidence provenance and trace
outcome. Their numeric migration must preserve option and output text. Stable
analyzer/group/check identifiers are a separate vocabulary. This task does not
claim all enum migration or the wider consolidation goal is complete. No full
precision replay or local race run was performed.
