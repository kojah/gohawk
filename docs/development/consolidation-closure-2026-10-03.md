# Consolidation completion

The original objective was to consolidate the architecture and duplicated
mechanics, address the remaining easy false positives, keep iteration scoped,
and track work in Beads. This closure checks that objective against production
source at `da87150d` and the final queue reconciliation in `f6e8c953`.
Subsequent closure edits change development documentation only.

## Requirement verification

| Requirement | Inspected authoritative evidence | Disposition |
| --- | --- | --- |
| Clean responsibility and dependency boundaries | The [source-owner reconciliation](consolidation-completion-audit.md#current-source-owner-coverage-reconciliation) maps all 359 authored production paths across all 33 source directories. Current AST inventories retain exactly those paths and 2,290 functions. Shared traversal, storage, completion, facts, broker, reporting and adapters have distinct owners; architecture tests enforce the specified boundaries. | Verified within authored production source. The latest conditional-copy and family changes remain in those existing owners. |
| Consolidated duplicated mechanics | The maintained [semantic family reviews](consolidation-completion-audit.md) cover analyzer proofs, shared queries, inference/publication and execution adapters, including differently written traversals. The current zero-threshold whole-body and partial scans retain 55 and 33 source-backed candidate dispositions, with no additions after the final follow-ups. | Identified unnecessary mechanics are consolidated. Retained domain adapters and different observation/polarity contracts are deliberate. Counts alone are not the proof. |
| One authoritative decision per check | The eight-analyzer/ten-check reconciliation maps collection, classifiers, flow proofs, tracing and reporting. Shared completion/transfer and report filtering replace the previously duplicated decision paths. Final resource and process reporting consume their structured proof results. | Verified by source-family ownership and behavioral controls; display and filtering do not independently infer lifecycle policy. |
| Numeric closed classifications | Kind, tier, reason, phase, outcome, action, provenance and lock-mode migrations use domain-owned numeric representations. Completion review also found and fixed resource family selection in `da87150d`. Syntax/type controls cover raw roles and indirect aliases; removing the final family guard fails six assertions. | Verified for the reviewed classification domains and declared guard roles. IDs, API names, input parsing and serialized output remain text. |
| Remaining easy FPs addressed | The [final 55-site ledger](../../benchmarks/precision/audits/final-production-fp-refresh-2026-10-03.md) has 29 successful original scopes and preserves all frozen keys/pins/verdicts. The rediscovered urunc FP is repaired at the shared heap boundary, with returned/discarded/replacement controls. Separate Rune controls and both original Rune issue closures are verified. | All identified bounded easy corrections are implemented. Five sites need the larger models described below and receive no completed-fix credit. |
| Precision preserved | Complete package comparison across the 29 scopes finds only the intended urunc removal and no additions. The final [family migration](../../benchmarks/precision/audits/resource-family-enum-2026-10-03.md) preserves ten diagnostics and 65,436 trace records in four affected scopes. Focused accepted/diagnostic, mutation, replacement, cutoff and counterfactual controls accompany the owning changes. | Verified at the stated fixture and pinned scopes. The full historical corpus is not claimed to have been rerun. |
| Tight validation cycle | The final conditional-copy and family receipts each pass generation, modules, formatting, vet, dead-code, lint, self-analysis and ordinary tests. Makefile inspection confirms `make verify` runs the canonical ordinary suite and gates. Final architecture checks pass after documentation. No code/build changes follow the final family receipt. | Verified; unaffected receipts are reused. There is no full precision replay per iteration or local race run. |
| Work tracked and own changes published | Beads records the corrections, source reviews and final queue, including `.4.9.1`, `.23.41` and `.4.9`. Commits `116376a8`, `da87150d` and `f6e8c953` are pushed. The unrelated staged blog-plan deletion and untracked artifacts are preserved. | Publication is verified. This closure is committed/pushed before final parent and goal completion. |

Current machine reconciliation is `.build/goal-closure-evidence.json`.
Its inputs include `.build/goal-final-owner-da871.json`,
`.build/goal-final-blocks-da871.json`, both immutable binary hashes, the final
canonical logs, all terminal scan metadata, complete diagnostic comparisons,
family controls and original/final ledger keys. The source inventories use the
same test/generated/fixture/vendor/hidden exclusions as the architecture
inventory. Graph tools were unavailable; evidence uses exact source rather
than claiming a clean graph index.

## Larger-model follow-ups

These are the remaining five sites, explicitly distinguished from easy existing
helper or classifier repairs. Their unresolved behavior is unchanged and is
not hidden by altered labels, suppressions, budgets, names or check retirement.

| Sites | Missing reusable evidence | Follow-up |
| --- | --- | --- |
| Goiardi, `shovey/sql_funcs.go:460:13` and `492:13`, pin `937cae400a92d8036b88ae2f65d93506271c292e` | A caller's global backend OR predicate must remain related to later callee reloads across intervening calls. Current binding proves literal/fixed snapshots but cannot establish mutation stability for the global relation. Three no-cutoff SSA probes distinguish those cases; both original diagnostics remain. | `gohawk-xmz`: stable caller predicates across mutable global reloads. |
| Ferro, `mcp/stdio.go:154:12`, pin `d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4` | Constructor-bound implicit zero state, scalar publication and receiver-conditioned results must compose to exclude the impossible transport Start failure. Twelve no-cutoff local probes isolate those distinct missing guarantees. Current package silence has command-use budget uncertainty and is not a correction. | `gohawk-zta`: receiver-conditioned scalar results through constructors. |
| Openase, `remote_shell_executor.go:138:2` and `142:2`, pin `e530faf137e764337d5beaaf68af3be159eb17aa` | Exact session/returned-reader provenance and EOF/completion across the application interface are missing. Pinned x/crypto v0.50.0 Close delegates channel close; Wait consumes exit and copy completion. A Close name or an additional call target does not prove a worker join. Both sites remain unknown. | `gohawk-278`: transport reader lifetime and completion contracts. |

The [source assessments and current probes](../../benchmarks/precision/audits/final-production-fp-refresh-2026-10-03.md#current-implementation-scope)
establish why these require additional evidence models rather than one safe
predicate at an existing decision point. Each follow-up records a bounded
design question and required accepted/diagnostic controls. They remain open
future model work; they are not described as fixed or removed.

The final ledger has 52 absences and three reported original keys. Of those
three, two are the goiardi assessments above and one is Promu's reviewed real
leak. Of the 52 absences, three are the Openase/Ferro unresolved silences above.
Twelve target traces contain cutoff events and receive no new correction credit.
All other dispositions retain their original structural correction or policy
evidence beside the current package observation.

## Completion boundary

All requirements of the original consolidation and remaining-easy-FP objective
are verified at the source and receipt scopes above. The three larger-model
features remain separately tracked. This does not assert that arbitrary future
code has no false positives, that unknown evidence proves cleanup, or that every
external dependency, build configuration and historical audit has been checked.
Those limits preserve the original precision-first model; they do not replace
uninvestigated easy work with a narrower completion claim.
