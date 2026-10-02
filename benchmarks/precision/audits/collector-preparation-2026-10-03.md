# Shared acyclic collector preparation

Bead: `gohawk-dho.23.21.5`. Parent: `01e17e7`.

Exact-source review found only two callers of `orderedBlocks`: the single
ordered-history collector and the bounded alternative-path collector. Their
identical detached-recovery rejection now belongs to that shared preparation
function. It executes before loop folding and budget spending, preserving the
cutoff block and control-flow-unknown reason. Their distinct join and path
algorithms are unchanged. Select and counted-loop collectors retain recovery
checks at their separate dispatch points.

The reviewed CLI selector candidate remains local scaffolding. Check selection
builds stable IDs from metadata and supports later enable/disable precedence;
analyzer and group selection reject overlap. Groups retain catalog-ordered
choice errors. Existing helpers already own value extraction and check/analyzer
list validation. Map allocation, argv[0] retention and local iteration do not
justify a new callback-driven dispatcher.

Graph tools were unavailable. Source inspection and the normalized AST scan
provide bounded evidence. The refreshed scan covers 329 production files and
2,174 declarations under `internal`, excluding generated/test/fixture/vendor
and dot/underscore trees. It retains five full-body and 35 partial-block
candidate groups. Identifier normalization conflates names and predeclared
identifiers; counts include overlapping windows. These are candidate counts,
not defect counts or proof that semantic duplication is absent. The reviewed
collector, lifecycle-consumption and slot-naming block groups are absent from
this refreshed candidate set; the intentional CLI scaffold remains.

Focused `go test ./internal/passes/concurrencyfacts` passes (4.270 seconds),
including existing detached-recovery and collector behavior controls.
No implementation-mirroring tests were added for this small guard relocation.
`make fmt` passes. Terminal canonical and scoped comparison receipts are
recorded below.

Eight production locations and broader semantic consolidation remain open.
No production FP correction, full precision replay or local race result is
claimed.

## Terminal validation receipts

Canonical `make verify VERIFY_TIMINGS=1` exits 0. All eight gates pass:
generation, module verification, vet, format check, deadcode, lint,
self-dogfood and ordinary tests. The canonical binary matches the reviewed
binary below. Scoped all-check scans produce eight terminal receipts, each
exit 0 with empty stderr. Complete merged parent/current payloads are identical:

| Scope | Findings in each version |
| --- | ---: |
| lockorder + ordercycles + readlockpaths | 117 |
| goroutineownership + closurechoices | 128 |
| Rune `./internal/ide/idepkg` | 1 |
| Skywalking `./pkg/tools/buffer` | 2 |

Rune pin: `3e2165f8983280542c985947378dfa740a397d03`, CGO enabled.
Skywalking pin: `e83d5925500a7e63dd55c080a9b1542d6cedaefb`.

Parent SHA-256:
`24f3e531a5ca7ed81b8c6ab8dbd84f28b6d7a8630e6400626b35df0850a50aec`.
Reviewed/canonical SHA-256:
`f459687bdeab6f8e04735f875880472eed76f07fddb31bd572b5993a0ad69fd5`.
Local scoped receipts: `.build/goal-collector-preparation/{scans,comparison}.json`.
Canonical log: `.build/goal-collector-preparation-verify.log`.

Final `go test ./internal/architecture` after the documentation updates passes.
