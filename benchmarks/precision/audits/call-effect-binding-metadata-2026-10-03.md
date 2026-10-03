# Shared call-effect binding metadata

Bead: `gohawk-dho.23.21.7`. Parent: `95d1b2f`.

Value, embedded-field and closure call-effect queries retained eager
`CallBindings` collection outside their request allowance. They now consume
`CallBindingsWithin` directly. One metadata-completion fence invalidates active
value/field memo answers after local or shared-pool cutoff. Discovered effects
remain possible-use evidence; missing bindings cannot establish purity. Exact
address matching, field selection, loaded-pointee separation, asynchronous
exposure and unknown-call policies are unchanged. The authoritative proof maps
shared-pool exhaustion to budget-unknown as well as local exhaustion.

The compiled SSA fixture has forty irrelevant arguments/captures after the
queried value. All three parent controls fail assertions: the parent returns
complete purity under a ten-step budget because it does not charge that census.
The new controls return budget-unknown, then recover complete purity with a
fresh allowance. A captured read discovered before cutoff is preserved. A
second fixture supplies the same address first and last, mutating only the last
parameter; a fresh allowance finds that mutation rather than retaining an
incomplete prefix. Local and shared-pool cutoffs both stay unknown. Full focused
ssaflow, heapmodel and lifecycle tests pass; the additional metadata controls
pass after correcting an omitted generated test function. That initial fixture
construction panic is superseded and supplies no regression evidence.

The change removes three eager collectors and centralizes their metadata fence.
It does not create another pairing or traversal algorithm. Graph/index/coverage
tools were unavailable; source and actual SSA provide scoped evidence. This
bounds binding metadata, not every transitive graph/type/alias cost, and makes
no repository-wide absence claim. No production FP correction, full precision
replay or local race run is credited.

Parent SHA-256:
`d83fe660eb8fc23884c7d7c36ec12fca31b716efe1ea71a6cc8c9561a8e4f347`.
Final frozen binary SHA-256:
`d6084bacb7aa9b06104540fe115dfd1c50feea941768e542575329bc3339b16d`.

Local receipts: `.build/goal-call-effect-metadata-{parent,focused,controls}.log`;
`.build/goal-call-effect-metadata-final-scans/{scans,comparison}.json`;
`.build/goal-call-effect-metadata-final-verify.log`.

Fourteen terminal all-check receipts exit 0 with empty stderr. Complete merged
parent/current diagnostic payloads agree in every affected scope:

| Scope | Findings, unchanged |
| --- | ---: |
| cancel-fixture | 40 |
| lock-fixture | 116 |
| resource-fixture | 318 |
| process-fixture | 41 |
| goroutine-fixture | 128 |
| openase | 0 |
| ferro | 1 |

Openase pin: `e530faf137e764337d5beaaf68af3be159eb17aa`. Ferro pin:
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`. Both checkouts were clean;
module scans are read-only with CGO disabled. Their unchanged silence does not
resolve the reviewed protocol/receiver-state gaps or earn correction credit.

Final `make verify VERIFY_TIMINGS=1` exits 0 with all eight gates passing:
generation, module verification, vet, format, lint, deadcode, self-dogfood
(66 seconds) and ordinary tests (123 seconds). The canonical binary matches
the immutable reviewed binary. Final architecture tests after documentation
updates pass. Unrelated working-tree and staged changes remain separate.
