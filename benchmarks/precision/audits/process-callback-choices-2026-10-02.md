# Opaque process callback choices

Beads `gohawk-dho.23.3`.

## Demonstrated gap and bounded correction

Actual SSA from the isolated probe (`.build/goal-process-choice-probe/mixed.ssa`)
shows a phi of captured closures supplied to an opaque runner. The parent
binary reports this and a dynamically launched choice. Its literal-only capture
gate misses positive possible retention, although literal opaque callbacks
already leave ownership unknown.

The existing handoff boundary now uses one shared ReachingWalk Any fold and
bounded lifecycle containment for captured leaves. Phi alternatives are
transparent; conversions and loads stay opaque. The dynamic call target and
callback arguments use the same query. Positive capture establishes unknown
ownership, never a unique callee, invocation guarantee or exact Wait.
Known-body droppers still use ordinary completion evidence. Bypassing the
handoff on an earlier return remains diagnostic.

Handoff classification now returns one structured state/reason proof. The
merged Wait receiver and nonreturning spawned-worker cases keep their prior
policies. Normal-return discovery uses `ProveNormalReturnWithin`; its completion
request already shares the same allowance. Capture, normal-return and completion
cutoffs are unknown. Graph/type/alias internals retain their existing costs;
this makes no whole-program cost or fact-guarantee claim.

The focused `handoff.go` file owns possible Wait participation and its
uncertainty. Ordinary ownership and reporting consume that authoritative proof.
The public page already describes the callback boundary; no new option or
message is introduced.

## Validation

Compiled SSA controls cover mixed and unrelated captures, converted functions,
nonreturning Wait workers, ordinary returning workers and workers without Wait.
An exhausted child remains unknown while fresh queries under the same parent
recover. All positive handoff answers remain unknown rather than proven.
Three counterfactuals fail assertions: requiring every phi leaf loses mixed
capture evidence, making conversions transparent violates the selected boundary,
and unbounded normal-return discovery bypasses the child cutoff.

The [ledger](process-callback-choices-2026-10-02.tsv) records eight successful
parent/current all-check scans with empty stderr. Existing process fixtures
retain all 37 diagnostic payloads exactly. New `processchoices` fixtures fall
from five reports to three, removing only mixed opaque argument capture and
mixed dynamic launch while retaining unrelated command/Kill, early bypass
and converted-callable findings. Initial fixture expectations were attached to
the closing braces; they were moved to the reported Start lines before final
validation. A temporary return of the embedded completion proof was also
corrected before focused validation.

Final trace `.build/goal-process-handoff-choices.trace` records the two accepted
choices as unknown ownership, rather than budget-driven silence. Pinned Ferro
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4` `./mcp` keeps its unrelated
finding; its historically budget-silent target remains unresolved. Pinned
rev-dep `8a2fdb0927e2fc9b2a5b178c94f55d1887659152` `./internal/telemetry`
retains its existing pipe-boundary silence. Production checkouts are clean.

Parent binary SHA-256:
`67fef5820e87ebf0692fadc8d41456ddda537e42f6d4d77af4d91eabb734ec23`.
Current binary SHA-256:
`355c5583b064e32b47e853e6abb005410bc258248e2fd76539c2a67c25c1d5cd`.
Receipts: `.build/goal-process-handoff-focused.log`,
`.build/goal-process-handoff-controls-final.log`,
`.build/goal-process-handoff-query-controls-final.log`,
`.build/goal-process-handoff-mutants/results.json`,
`.build/goal-process-handoff-scoped/scans.json`,
`.build/goal-process-handoff-verify.log` and
`.build/goal-process-handoff-architecture.log`.
All eight canonical verify targets and final architecture validation pass.
No full precision replay or local race run was used. Graph tools were unavailable;
exact source and actual SSA supply scoped evidence.

No recorded production FP correction is credited. Seven production sites plus
Rune remain unresolved, and the overall consolidation audit remains open.
