# Completion binding mapping follow-up — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.8`. Parent: `20ec1d8`.

## Change and boundaries

Completion local mapping now consumes `CallBindingsWithin` under the request
allowance and discards the entire mapped-local list if binding metadata or a
mapping query exhausts the child or its parent pool. A mapped prefix cannot
enter coverage as an authoritative binding set.

Captured-value selection, capture matching, derivation, static access paths and
aggregate containment use existing bounded owning-layer queries. The original
capture match is computed once. Deferred stability can replace the later exact
answer with a different stable-value alias question, so its original result
remains separate for the containment boundary. Exact-target and deferred
storage-owner policies, mapping order, and may versus exact identity remain
unchanged at completed queries. No independent alias engine or traversal was
added.

Graph tools were unavailable. Source review covered completion_mapping.go,
completion_search.go, call_bindings.go, value_matching.go, store_alias.go,
store_derivation.go, store_ownership.go and store_cells.go. Graph/alias/type
internals, deferred storage referrer/path censuses, argument strict projections,
static storage-owner paths and receiver matching remain separate review scope.
This does not claim a whole-query bound or complete architecture consolidation.

## Actual SSA and controls

`TestCompletionMappingCutoffDiscardsPrefix` examines both a helper called with
the same resource in two argument slots and a closure with two spilled capture
cells. Every cold cutoff must publish no locals; a sufficient allowance maps
both slots. Existing deferred storage-owner controls still reject sibling
fields, reassignment before and after registration, cleanup mutation and an
opaque cell handoff.

- Parent receipt `.build/goal-completion-mapping-parent.log`: assertion failure
  on two locals published after argument cutoff and no capture cutoff at all.
- Focused receipt `.build/goal-completion-mapping-focused.log`: passed.
- Counterfactual removing only partial-map cutoff guards:
  `.build/goal-completion-mapping-prefix.log`, assertion failures for both
  families, with one partial local published at allowance 1 and 4 respectively.
- SSA dumps `.build/goal-completion-mapping-{arguments,captures}.ssa` confirm
  ordered argument slots and the two captured cells. Both final dumps succeed
  with empty stderr. An initial function filter `map` matched nothing and is
  not a successful dump receipt.
- Affected lifecycle, lockorder, resource, goroutine and process tests pass in
  `.build/goal-completion-mapping-consumers.log`. That invocation also included
  an incorrect cancellation package path and exits 1 for that setup failure;
  the corrected cancellation-only receipt
  `.build/goal-completion-mapping-cancellation.log` passes.

## Production and canonical validation

Immutable binary `.build/goal-completion-mapping-reviewed`, SHA-256
`4573803d5e0bfdca3feed5c9ec5ac1ae1b0031b7b5b80191984b01d3f83d51c8`.
Pinned scans use CGO disabled, readonly modules, workspace disabled, a
180-second limit and `go vet -vettool=/absolute/binary -enable-all -json`.

| Pin and scope | Retained reviewed controls |
| --- | --- |
| majestrate/XD `b905a14ecfeceaa21a5dde82b52f164d075002ab`, `./lib/configparser` | Missing releases at configparser.go:115 and 121. |
| ctdk/goiardi `937cae400a92d8036b88ae2f65d93506271c292e`, `./datastore ./indexer` | Read-lock writes at datastore.go:542 and file_index.go:708. |

Both scans exit zero, with empty stderr and byte-identical JSON to the prior
callee-resolution receipts (838 and 1,476 bytes). Current receipts:
`.build/goal-completion-mapping-{xd,goiardi}.json` and `.err`.

`make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, ordinary tests, architecture checks and local
all-check dogfood. Receipt `.build/goal-completion-mapping-verify.log`.
No full precision-regression replay or local race run was performed.

No production FP correction is credited. Eleven recorded production FP sites
and Rune's publication issue remain unresolved. Broader consolidation remains
active.
