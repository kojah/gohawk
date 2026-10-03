# Possible writer witness allowance

Bead: `gohawk-dho.44.11.5.27.26`. Parent source: `55baa68`.

## Confirmed request gap

Read-lock mutation classification consumed a completed function inventory, but
`possibleWriterAt` repeated its call-list scan and used default dominance and
reachability helpers without the function's observed allowance. A real-SSA
consumer control on the parent answers its writer query without marking a zero
request allowance exhausted. The current control marks cutoff and publishes no
read-lock mutation diagnostic.

Possible writer selection now has one structured proof path. Candidate visits,
call-list entries and temporal queries share the function allowance. A completed
positive means a possible writer guard exists, not that the mutex is definitely
held; the mutation classifier retains its unknown imported-writer verdict.
A completed negative permits the existing mutation proof to continue. Cutoff
is unknown and yields `lock-state-budget-exhausted`, never a missing guard.
The surrounding bounded walk still discards staged diagnostics and edges when
interrupted. Alias/type/heap graph costs retain their separate owners.

Setup selection and temporal writer evidence live in `writer_witnesses.go`.
The source responsibility is uncertainty evidence rather than actual held-state
or ordering inference. This extracted a distinct concern from the growing
read-lock file, reducing it from 412 to 306 lines and keeping its mutation/field
policy together. Existing precision rationale and pinned source comments moved
with the implementation.

## Actual SSA and boundary controls

`TestReadLockWriterWitnessSharesAllowance` reproduces the parent zero-allowance
failure and checks zero, tiny and complete requests without any report from an
interrupted or possible writer. `TestWriterTemporalProofAllowance` logs actual
SSA for a dominating defer, explicit intervening release, defer after the write,
and a branched mutation. It sweeps every allowance through first completion,
requires unknown budget reason at local cuts and checks fresh-child recovery
without exhausting the outer pool. Complete results preserve the old policy:
possible writer for dominating guards, absent for later defers and prior release.
The existing accepted and diagnostic `opaque_writer.go` fixtures also pass.

## Validation and output compatibility

Initial canonical verification passes all eight gates before the cohesive
extraction. Final `make verify VERIFY_TIMINGS=1` passes after extraction:
generate 3s, modules 0s, vet 1s, formatting 2s, deadcode 4s, lint 6s,
dogfood 25s, tests 68s. No gate baseline or threshold was raised.

Ten parent/current read-only all-check scan invocations exit 0 with empty stderr.
Complete diagnostic JSON matches for lock fixtures (116 findings), goroutine
fixtures (128), Rune (0), Skywalking (0), and stargz (1). The retained stargz
finding is the reviewed `store/manager.go:193:2` true positive. Receipts record
clean pinned revisions, package scopes and binary hashes. Rune uses CGO enabled;
other scopes disable it. No external repository tests, applications or generators
were run.

Two traced lock-fixture scans select candidates in `opaque_writer.go`. All 39
records per version preserve complete contents and ordered per-candidate
sequences. Their complete diagnostic JSON matches the corresponding untraced
scans. This targeted comparison establishes the existing writer boundary; it
is not a comparison of every historical trace or every budget event.

Reviewed/canonical binary SHA-256:
`5ea95b0ad5c0b602244b1b8a93dd0d8a751a56bca29e1c240c8d13547aedcf43`.
Parent binary SHA-256:
`2dffcbc4fca6730695d6c2edab8accf244ddc8e9e5a28410e254ce080742c7fb`.
Artifacts use `.build/goal-writer-witness-*`, including parent SSA control,
canonical logs, scan receipts and targeted trace comparisons.

## Limits and remaining work

Graph/index MCP tools were unavailable; this review uses bounded exact source
and actual SSA. It does not claim transitive wall-clock completeness or absence
of semantic duplication outside the writer evidence concern. Lock identity
storage and caller exclusivity remain open source-backed questions in the parent.
The five recorded production FP sites remain unresolved. No full precision
replay, local race run, FP credit or overall goal completion is claimed.
