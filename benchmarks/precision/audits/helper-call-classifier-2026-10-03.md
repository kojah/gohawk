# Helper-call classifier consolidation

Bead `gohawk-dho.44.12`; parent source `ae00ed9`.

## Authoritative evidence

The main helper-call classifier prepared eager bindings and allocated a
standalone recursive search per matched binding/handle. It now uses lazy
binding metadata and one supplied candidate allowance for selection, recursive
effects and exact caller identity. One lazily initialized memo serves the call;
its key retains callee, formal/capture and tracked kind. Caller binding remains
an independent exact check for each supplied value, rather than being cached
as a formal effect. Cleanup-target search uses the same constructor with its
existing supplied allowance, avoiding unused default budget preparation.

A structured helper-call proof owns action and stable reason. Incomplete
selection/effect/binding yields unknown with `helper-call-budget-exhausted`
and evidence phase `helper-call`. Testing-cleanup adapters preserve this cause;
pipe-peer consumers project any effect as unknown participation. A proven exact
join stops before unrelated bindings because later opaque effects cannot weaken
it under the existing action lattice. Owner cleanup and possible bindings never
become joins. Existing rationale links stay with the extracted evidence file.

## Controls

Actual SSA has a completion channel spilled into a capture cell. A helper with
32 source-visible calls requires more visits than a small candidate allowance.
At 32 visits, even restoring a standalone recursive search can cut during exact
caller-cell identity. The 64-visit control isolates recursive allowance: current
code remains unknown, while restoring a standalone search returns a completed
join from work outside the candidate allowance. Fresh allowance proves the exact join. A second actual-SSA
control requires one memo to distinguish an ignored first formal from the
second formal's receive when both are supplied the same channel.

Three compiling counterfactuals fail assertions: standalone recursive search,
missing cutoff projection, and omission of the formal from the memo key.
Existing join-binding, mixed/replaced/owner, helper-kind, recursion and trace
controls pass. Temporary inspection probes are removed; retained logs contain
only the local synthetic SSA investigation.

## Validation and limits

Final `make verify VERIFY_TIMINGS=1` passes generation 2s, modules 1s, vet 2s,
formatting 3s, dogfood 4s, lint 6s, deadcode 7s and ordinary tests 59s. Six
read-only all-check parent/current invocations preserve complete payloads:
128 goroutine/closure-choice fixture findings, zero Openase findings at
`e530faf137e764337d5beaaf68af3be159eb17aa` in internal/infra/hook, and the
one stargz store control at `624678b4e421947534cbf0618f9609853cccee0f`.
All exits are zero with empty stderr. Production checkouts are clean at the
pins and modules are readonly. Reviewed and canonical binaries match:
`947a3c9fb8d2e9b351df4aba5c7b9de956cf04323c7cccc099620f45e2918b02`.
Ignored receipts are under `.build/goal-helper-call-*`.

Graph/index tools were unavailable. Source candidate scans cover 336 production
files and 2,207 declarations, retaining five whole-body and 34 partial groups
with unchanged path/symbol/token signatures. These counts do not establish
absence of differently structured semantic duplication. Default standalone
searches and graph/alias/type/flow internals retain separate cost owners. Pool
comments now state this boundary rather than claiming every transitive operation
is charged. No threshold, schema or budget grows. No full precision replay,
local race run or production FP correction is claimed; parent reconciliation
and the five-site residual queue remain open.
