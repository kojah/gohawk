# Worker receive search consolidation

Commit `83a526a` replaces two analyzer-local receive walks with one bounded
search. The old visited sets keyed only the function: after inspecting a
helper through its first formal, a later call through its second formal was
skipped. The shared memo now keys the function and local value and preserves
caller-selected channel and stable receiver-context predicates.

The new `receive_bindings.go` fixture reproduces an analyzer FP under parent
`1c2a67c`: a worker first calls the helper with an already-canceled local
context, then uses the caller's receiver context through the other formal.
The parent source overlay's analyzer test reports an unexpected unjoined
diagnostic at line 16. The corrected suite accepts it and still diagnoses the
nearby local-receiver control. Unit tests cover distinct bindings, diamonds,
unrelated values, opaque capture cells, nested asynchronous may-consumption,
field mutation, missing/recursive bodies, budget exhaustion and retrying an
incomplete query with a fresh budget.

The possible receive bounds ownership; it never proves a join. Memo cutoffs
retain explicit unknown evidence. Queries use the candidate's budget and
observer; exhaustion suppresses reporting with
`worker-receive-budget-exhausted`. A candidate-level test covers that decision.
No fact schema, API-name exception or new lifecycle contract was introduced.

## Validation receipts

The parent binary implements `1c2a67c`, SHA-256
`8762964108e3bcd84c7bb095820e899559b5bc6add1cc103bc314cf3d936668b`.
The final binary implements `83a526a`, built before commit; SHA-256
`91eb53901377a4d8f1bb7244792ea6cd200025c11ff84aa317785adbc7be08b1`.

The static fixture scans use `GO111MODULE=off`, the analyzer's testdata GOPATH,
`-enable=goroutineownership -json`. Both exit 3 because the fixture package
contains expected diagnostic cases; both stderr receipts are empty. The parent
JSON is `.build/goal-receive-search-parent.json`, and the final JSON is
`.build/goal-worker-receive-final-fixture.json`. Their exact finding-key
difference is removal of `receive_bindings.go:16:2`, with no added keys.
The final trace `.build/goal-worker-receive-final.trace.jsonl` records
`receiver-context-lifecycle/unknown` for that candidate. The actual SSA is
`.build/goal-worker-receive-fixture.ssa.txt`.

Production controls use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`,
and `go vet -vettool=<final immutable binary> -enable-all -json`:

| Repository and pin | Scope | Result and receipt |
| --- | --- | --- |
| containerd/stargz-snapshotter, `624678b4e421947534cbf0618f9609853cccee0f` | `./store` | Exit 0; reviewed unjoined-worker TP at `manager.go:193:2` retained. `.build/goal-worker-receive-final-stargz.json`. |
| Debian/dcs, `567a9be49163cbf731f25bf79890f692e04d22d9` | `./internal/sourcebackend` | Exit 0; unresolved original FP keys at `sourcebackend.go:433:3` and `562:3` retained. `.build/goal-worker-receive-final-debian.json`. |

Production stderr receipts are empty; assertions checked the pins and exact
keys. These are scoped controls, not complete repository scans. The synthetic
FP correction does not reduce the [18 unresolved production sites](remaining-fp-assessment-2026-10-01.md)
or change frozen labels. Candidate tests, generators and applications were not
executed.

Final `make verify` passes all gates. The first gate's ordinary tests passed,
but modernize lint required `slices.ContainsFunc`; the correction and final
gate passed. No full precision-regression corpus or local race tests ran.
Beads `gohawk-dho.14` owns this change. The broader consolidation goal remains
active.
