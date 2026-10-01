# Definite nilness and interface boxing

Beads `gohawk-dho.18` owns this correction. Shared-engine review found that
`DefinitelyNil` peeled interface boxing and called a boxed typed nil pointer
nil, while `ValueOutcome` correctly called the same interface nonnil.
The static probe's actual SSA contains `MakeInterface` for both pointer and
error interfaces (`.build/goal-nil-boxing-probe.log`). Candidate applications,
tests and generators were not executed; analyzer regression fixtures were.

The correction preserves boxing and delegates definite nilness to
`ReachingWalk.Every`. Nil pointer conversions and actual nil interfaces remain
recognized. Shared phi alternatives are evaluated independently, while mixed,
empty and cyclic evidence does not establish nilness. Unit tests cover these
boundaries; the parent fails the three boxing and two shared-phi cases.

## Minimized analyzer corrections

The parent reports unexpected resource-lifetime diagnostics at
`compression_error_boxing.go:15:12` and `:21:12`: a compression writer returning
a typed nil error, and one aborting its output pipe with that error. The
existing abandonment classifier now labels both paths unknown with reason
`compression-output-may-be-abandoned`. This is uncertain abandonment, not
proven finalization. The unfinished writer returning an actual nil error at
`:27:12` remains reported.

Immutable parent and corrected fixture scans use `-enable=resourcelifetime
-json`, `GO111MODULE=off`, and the analyzer's fixture GOPATH. Both exit 3 because
the package contains expected diagnostic fixtures, with empty stderr. Their
diagnostic-key difference is exactly the two removals above, with no additions.

| Executable | SHA-256 |
| --- | --- |
| Parent through `055f0c6` | `3babd82c8363cb4dc3fabbfb4ab0c78f5a1332a9a90a22d99e6ea5d587eb5093` |
| Corrected, also used by the canonical verification build | `b99e3aead2fa73e2cd62ac21ac6ccf49b912907ada2378ad37afaa04ebaac76f` |

Receipts are `.build/goal-nil-boxing-{parent,current}.json`,
`.build/goal-nil-boxing-current.trace.jsonl`, and
`.build/goal-nil-boxing-compressor.ssa.txt`. Focused SSA and resource-lifetime
tests pass, and `make verify` passes every local gate; ordinary tests take
76 seconds (`.build/goal-nil-boxing-verify.log`). No full precision-regression
corpus or local race run was performed.

## Scoped production controls

Both pinned package scans use `go vet -enable-all -json`, the corrected
executable, `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, and `GOWORK=off`. They exit
zero with empty stderr:

- containerd/stargz-snapshotter at `624678b4e421947534cbf0618f9609853cccee0f`,
  `./store`: the known `goroutineownership/unjoined` TP at `manager.go:193:2`
  remains (`.build/goal-nil-boxing-stargz.json`).
- PacificStudio/openase at `e530faf137e764337d5beaaf68af3be159eb17aa`,
  `./internal/infra/hook`: the unresolved findings at
  `remote_shell_executor.go:138:2` and `:142:2` remain
  (`.build/goal-nil-boxing-openase.json`).

These are two new minimized FP corrections, not removals from the frozen
production ledger. The assessed production queue still has 18 unresolved
locations; audit labels and historical precision totals remain unchanged.
