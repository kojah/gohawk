# Strict-dominator instruction census

Beads `gohawk-dho.23.4`.

## Consolidated mechanics

Three process pre-Start queries duplicated the block-dominance and same-block
prefix loop. External command-store and goroutine pre-spawn queries enumerated
the same structural relation using per-instruction dominance. One shared
`ssaflow.InstructionsStrictlyDominatingWithin` now owns this traversal.
It indexes the pivot once, preserves function block order and excludes the
pivot and later instructions in its block, even in loops. Block checks,
indexing and yielded visits consume the supplied allowance. The iterator
provides structural evidence and deliberately leaves acceptance policy local.

Process ownership materializes one completed census for command cleanup,
wrapper registration/completion and external-store policies. Its child cutoff
discards every prefix and traces unknown ownership before any policy consumes
it. Method sets, wrapper watcher requirements, possible alias and destination
contracts retain their meanings. Goroutine ownership can stop at a positive
witness; an interrupted negative is unknown with reason
`pre-spawn-census-budget-exhausted`. Deferred/testing cleanup, ordinary prior
Wait and transfer rules retain their policies. No unconditional fact changes.

Watcher discovery, binding/heap/type query internals and other pre/post-Start
queries retain separate costs. This is a bounded census consolidation, not a
whole-candidate or whole-program time guarantee. No performance percentage is
claimed from concurrently running validation commands.

## Verification

Actual SSA differential controls compare every pivot against the existing
per-instruction dominance relation across straight-line, diamond and loop CFGs.
Budget controls cover each intermediate prefix, exact completed order and fresh
child recovery. Process controls require cutoff prefixes to be discarded;
goroutine controls require attributed unknown cutoff and fresh recovery.
The large signal-census fixture now stops at the earlier pre-spawn stage.
Its direct signal query still exercises cutoff/recovery and the production
cutoff projection, preserving the separate signal-census trace assertion.

Three counterfactuals fail assertions: including the pivot changes strict
ordering, publishing a shortened process prefix invents a completed census,
and an unbounded goroutine census bypasses its cutoff. These use isolated
source overlays and do not edit production source.

The [ledger](strict-dominator-census-2026-10-02.tsv) records eight terminal
parent/current all-check scans with empty stderr. Process/choice fixtures
retain all 40 reports; goroutine/choice fixtures retain all 128 reports, with
identical complete payloads. Clean pinned Ferro
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4` `./mcp` retains its one unrelated
finding and historically budget-silent unresolved target. Clean pinned Debian
`567a9be49163cbf731f25bf79890f692e04d22d9` `./internal/sourcebackend` remains
silent after the previously credited closure-consumer correction.

A terminal traced all-check goroutine fixture scan has empty stderr and
byte-identical diagnostic JSON to the untraced current scan. Its 3,280 parsed
trace events include accepted, rejected and unknown decisions. The targeted
unit trace asserts the new pre-spawn cutoff's phase, budget reason and candidate
association. Disabled tracing has no new policy path.

Parent binary SHA-256:
`355c5583b064e32b47e853e6abb005410bc258248e2fd76539c2a67c25c1d5cd`.
Current binary SHA-256:
`f4aaa34926a705750e7a908497b3697f715f748a45cd6c99934275677986d940`.
Receipts: `.build/goal-dominating-census-ssa-controls.log`,
`.build/goal-dominating-census-start-controls.log`,
`.build/goal-dominating-census-spawn-controls-final.log`,
`.build/goal-dominating-census-mutants/results.json`,
`.build/goal-dominating-census-scoped/scans.json`,
`.build/goal-dominating-census-fixture-traced.json`,
`.build/goal-dominating-census-fixture.trace.jsonl`,
`.build/goal-dominating-census-verify.log` and
`.build/goal-dominating-census-architecture.log`.
Final canonical validation passes all eight targets. Final architecture passes.
Initial focused validation exposed the new reason spelling assertion and the
changed earlier cutoff stage; focused reason/trace controls were corrected.
No full precision replay or local race run was used. Graph tools were unavailable;
source fallback identified every changed production consumer and actual SSA
verified the traversal's contract. The generated helper reference is updated.

No recorded production FP correction is credited. Seven production sites plus
Rune and the broader consolidation completion audit remain open.
