# Flow location and remaining duplication — 2026-10-02

Beads: `gohawk-dho.45`, child `.45.1`. Parent: `600ef43`.

## Shared mechanics

The earlier whole-function review correctly kept resource and obligation state
separate, but their location serialization was still repeated.
`ssaflow.FlowLocationKeyWithin` now owns function-local block, predecessor,
instruction position and guard rendering. Both walks compose this location
with their own domain state. Missing predecessor retains the -1 sentinel;
entry block zero cannot collide with it. Guard visits retain the same budget
and `WalkStatesWithin` refuses an interrupted key before expansion. Resource
activation, cleanup, uncertainty and contradiction policy remain analyzer-owned;
shared obligation coverage remains its own typed component.

The production-only partial scan also found `matchesAnySymbol` duplicating the
existing `CallMatchesAnySymbol` loop. Library contracts now use the shared
symbol-list query. Ordered first-match and nil handling are unchanged; no
symbol declaration, heuristic, fact encoding or reporting policy changes.

## Bounded candidate dispositions

Graph tools were unavailable. Source fallback inspected all six initial
normalized whole-function groups and the 14 initial partial-block groups.
The normalized scanner covers 315 production files under internal, excludes
tests/generated/fixtures, and compares complete bodies of at least 35 tokens
with identifier names normalized. Before: 2,143 functions/six groups;
after: 2,143 functions/five groups. Receipts:
`.build/goal-storage-projection-duplicate-functions.json` and
`.build/goal-flow-location-duplicates-final.json`.

| Remaining whole-function candidate | Source-backed disposition |
| --- | --- |
| carriedValueProof / capturedBodyResult | Found carry is Proven; found guarded Body cleanup is Unknown. Missing reasons differ. |
| spawnAnalysis.action / resourceAnalysis.action | Typed classifier caches retain distinct action/reason policies and trace emitters; each invokes one authoritative classifier. |
| StoresValueInField / Global / EnclosingScope | Field/global destination and may-alias differ from captured free-variable destination and derivation. They already share provenance queries. |
| Spawn/resource budget adapters | Different candidate limits and observers over the shared Within pool mechanic. |
| provenResourcePresence / provenStoredValue | Branch-presence Boolean and exact stored SSA value have different evidence reasons and payloads. |

The deeper scan uses golangci-lint dupl at 35 tokens, tests disabled, scoped to
`./internal/...`. The final canonical 60-token dupl gate passes. Before: 31
reports/14 connected groups. After removing the symbol-loop copy: 29 reports/
13 groups. Receipts `.build/goal-flow-location-dupl35{,-final}.json`, config
`.build/goal-dupl35.yml`. Initial relative JSON output was rooted at the config
directory; it is copied to the recorded receipt. One concurrent final scan
was rejected by golangci-lint's process lock; the completed retry uses an
absolute output path (`.build/goal-flow-location-dupl35-final-retry.log`).

| Partial-block family | Source boundary and disposition |
| --- | --- |
| Goroutine completion_bindings / obligation | Repeated function signatures for different binding/notification queries; no repeated body engine in the reported ranges. |
| deferinloop lockContracts | Declarative acquire/release pairs for Mutex, RWMutex and read locks; declaration identity is the policy. |
| Carried/captured Body results | Same distinct proof polarities as above. |
| store_projection address/root adapters | Both delegate to projectionUsesPreserveStorage; callback acceptance policies differ. |
| store_regions_cache publication/eviction | Repeated matching-entry deletion/LRU removal; follow-up gohawk-dho.46 must establish dependency and notification requirements before reuse. |
| completion_coverage entry points | Shared signature; public NonNil adapter delegates to the authoritative assumptions/coverage engine. |
| localCallbackAddress / localAggregateRootWithin | Callback addresses allow slices with an eight-step cap; aggregate roots allow field/index paths and share a search budget. Different transparent forms and evidence boundary. |
| store_escape field/global/enclosing scope | Destination and identity policies differ, as above. |
| replicated_workers iteration / noEffects | Replication requires positive workers and permits group-add operations; noEffects requires zero operations and workers. Similar fields encode opposite conditions. |
| callback_facts / element_discharges | Both require a visible package-local body; element proof also requires a valid parameter and proves release, callback inference proves invocation. Small boundary predicates, no duplicated body walk. |
| heap Retained / Stored | Both delegate to heapClaim; may-retention and positive storage/transfer are different claims. |
| call_contracts API declarations | Different registered testing and mock symbols, not repeated resolution mechanics. |
| syntax namedType / NamedType | Final named declaration test is similar; symbol matching unaliases before and after a pointer, while NamedType intentionally checks the supplied named type with one pointer layer. No alias policy is broadened. |

These scans do not prove absence of smaller, differently shaped or excluded
clones. The cache follow-up, broader family inventory, graph/type/alias costs
and production FP queue remain open. No generic proof factory or domain-state
merge is introduced solely to make normalized syntax disappear.

## Validation

Actual compiled SSA controls retain a two-predecessor merge and stable guard.
The work list expands six different locations and deduplicates the repeated
one. A guard-rendering cutoff admits no state. Consumer controls expand three
coverage states and four resource states at one location.
Four temporary counterfactuals fail on assertions when guard identity,
missing-predecessor sentinel, coverage or resource state is dropped:
`.build/goal-flow-location-{guards,predecessor,coverage,resource}-counterfactual.log`.
Focused existing flow/guard tests and all ssaflow contract/symbol fixtures pass.
Receipts `.build/goal-flow-location-focused-final.log` and
`.build/goal-flow-location-symbols-focused.log`.

Final `make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, self-dogfood and ordinary tests including
architecture checks. Receipt `.build/goal-flow-location-verify-final.log`.
The earlier location-only gate also passes (`goal-flow-location-verify.log`).
No full precision replay or local race tests ran.

Final immutable binary `.build/goal-flow-location-reviewed-final`, SHA-256
`e7af861356b14c9029248a6cabeea75a7e725213d16641eeb6a4e61aa4d2f517`.
Pinned controls: XD `b905a14ecfeceaa21a5dde82b52f164d075002ab`,
`./lib/configparser`; goiardi `937cae400a92d8036b88ae2f65d93506271c292e`,
`./datastore ./indexer`. Readonly-module, CGO-disabled, workspace-disabled
all-check vet scans have 180-second limits. Both exit zero with empty stderr
and byte-identical JSON against the parent immutable binary (838 and 1,476
bytes). Receipts `.build/goal-flow-location-{xd,goiardi}-final.json` and `.err`.
Missing releases at configparser.go:115/121 and read-lock writes at
datastore.go:542/file_index.go:708 remain.
Eleven production FP sites plus Rune remain unresolved; no production FP
removal or complete architecture certification is credited.
