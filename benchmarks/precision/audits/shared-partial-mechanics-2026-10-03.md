# Shared partial evidence mechanics

Beads: `gohawk-dho.23.21.1`, `.21.2`, `.21.3`. Parent: `520ae75`.

## Bounded source review

A fresh source scan visits 329 non-generated production Go files and 2170
function declarations under internal, with tests and fixture/vendor/dot trees
excluded. Five identifier-normalized whole-function groups and 43 adjacent
three-to-six-statement block groups are candidates, not equivalence evidence.
Identifiers include type/field names and nil/Boolean names, so normalization
hides material policy distinctions. The maintained
[completion audit](../../../docs/development/consolidation-completion-audit.md)
records all candidate indices, exact source review dispositions, three current
consolidations and pending candidates. Graph/index/coverage tools are unavailable;
this review uses bounded source fallback and makes no repository-wide absence
claim. Local scanner sources and before/after JSON live under
`.build/goal-duplicate-*`.

## Consolidations and controls

Concurrency Engine.fixedLoad and callerLoad share the bounded first field-load
census and exact declared-field/access-path match. The canonical cache's
write-once evidence, recursive sentinel and cutoff invalidation stay in
fixedLoad; caller binding retains its subsequent fixedLoad query. Actual SSA
controls distinguish two roots and sibling fields, select the first matching
load, enumerate cold cutoffs and recover with fresh allowance. Existing captured
field and read-time identity controls remain green. Removing the root match in
an overlay fails both caller and canonical assertions.

Aggregate derivation shares the selected-address load/debug/recursive-selection
classifier. Whole-root stores remain an explicit exception in the enclosing
aggregate query. Selected stores, unknown calls and escaping addresses remain
opaque. Actual SSA nested array/struct controls distinguish read-only selection,
selected-field replacement and escape at every cutoff through completion.
Accepting selected stores in an overlay fails the replacement control.
Neither refactor changes visited costs, candidate order or observation time.

Heap transfer's ChangeInterface, ChangeType, Convert and MakeInterface operands
use ssaflow.UnwrapTransparentValue with exactly those forms. SliceToArrayPointer
remains an explicit heap-specific case and TypeAssert remains in the existing
transfer handling. No new transparent form, load traversal, phi expansion or
budget charge is introduced. Existing converted storage/projection controls and
heap/lifecycle/domain tests pass; no implementation-mirroring wrapper test was
added for this mechanical bridge.

After these changes the same source scan finds 2172 declarations, five
whole-function groups and 38 partial-block groups. Overlapping candidate counts
are not a defect metric or proof of no further duplication. Lifecycle argument
consumption, heap slot naming, collector preparation and CLI parser scaffolding
remain review candidates.

## Scoped equality

Fourteen parent/current go-vet receipts use all checks, terminal exit 0 and
empty stderr. Five complete fixture scopes use modules disabled, fixture GOPATH
and CGO disabled; pinned repos use readonly modules, clean full pins and GOWORK
off. Rune uses CGO enabled. Every complete diagnostic payload is identical:

| Scope | Findings in each payload |
| --- | ---: |
| lockorder, ordercycles, readlockpaths | 117 |
| privateread | 6 |
| resourcelifetime | 318 |
| processownership, processchoices | 41 |
| goroutineownership, closurechoices | 128 |
| Skywalking `./pkg/tools/buffer` at `e83d5925500a7e63dd55c080a9b1542d6cedaefb` | 2 |
| Rune `./internal/ide/idepkg` at `3e2165f8983280542c985947378dfa740a397d03` | 1 |

Receipts and full payload comparisons:
`.build/goal-shared-partial-mechanics/scans.json` and `comparison.json`.
Frozen parent SHA256:
`cb52d49e7685fb8fe7c5b9905063c369d61d2f78a81d7e28a0a7032d20b80db5`.
Frozen current `.build/goal-shared-partial-mechanics-reviewed` SHA256:
`9329802541bddbe68a19a11a29754f33a0352dfc7a90e38cb83177096d5760a2`.
The canonical dogfood binary has the same current hash.

No diagnostic or fact-schema change is intended, and no production FP is
credited. Eight recorded production locations and broader semantic/partial
review remain unresolved. No full precision replay or local race run is used.

## Local completion gate

Focused shared/domain tests and `make verify VERIFY_TIMINGS=1` pass. The latter
runs generation, formatter, module verification, vet, dead code, lint, ordinary
tests and self-dogfood. Ordinary tests took 92 seconds, dogfood 54 seconds.
Both counterfactual overlays fail assertions, rather than compilation. Audit
logs use the `goal-shared-partial-mechanics`, `goal-field-load-discovery` and
`goal-address-use-sharing` prefixes under `.build/`.
