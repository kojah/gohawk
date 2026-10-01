# Callback invocation engine consolidation

Commit `17cdac8` removes the separate Boolean callback search. Ordinary
callback invocation now projects `ProveCompletion` with `InvokeTarget`;
examining a spawned wrapper uses the same callee-body search independently
of the outer launch. Both share exact identity, callback bindings, coverage,
recursion and budget rules. No launch mode, invented SSA or fact field was
added. A rejected recursive body visit produces unknown evidence in the
shared engine rather than a negative completion claim.

The shared invocation tests cover direct/forwarded/captured/deferred calls,
conditional, wrong, replaced and mixed targets, bound invokers, dropped
callbacks, nested asynchronous invocation, unavailable bodies, recursion and
budget exhaustion. The spawned-body tests separately require ordinary
completion to reject each outer asynchronous launch. Analyzer fixtures pair
joined and partially joined bound workers with dropped and asynchronous
invokers. The first fixture expected a report for a never-observed signal;
actual SSA and trace identified the existing `signal-never-observed` boundary.
The final diagnostic fixture observes the signal on one branch and leaves
another return unjoined. No reporting policy was broadened for that fixture.

## Focused production controls

The immutable binary implements `17cdac8`, built before commit; SHA-256
`dbd387ebbd5b6c84aed32fce2228eeee723af3c53e9be89d0ce0143c1594b358`.
Both scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, and
`go vet -vettool=<immutable binary> -enable-all -json`. Candidate tests,
generators and applications were not executed.

| Repository and pin | Scope | Result and receipt |
| --- | --- | --- |
| containerd/stargz-snapshotter, `624678b4e421947534cbf0618f9609853cccee0f` | `./store` | Exit 0; reviewed `goroutineownership/unjoined` at `store/manager.go:193:2` retained. `.build/goal-callback-engine-stargz.json`. |
| lynxbase/lynxdb, `7c4bf0432b0cef2807f0ddcd2cd2000ce7ffb8c1` | `./pkg/ingest/receiver/otlpgrpc` | Exit 0; scoped JSON remains empty, including absence of the previously corrected `server.go:108:2` FP. `.build/goal-callback-engine-lynx.json`. |

Both stderr receipts are empty. Assertions checked exact diagnostic keys,
scoped JSON and pinned checkout revisions. These are affected-package controls,
not complete repository scans or a new precision batch. No additional FP
removal is credited; the [18-site assessment](remaining-fp-assessment-2026-10-01.md)
and frozen labels remain unchanged.

Final `make verify` passes all gates, including ordinary tests, formatting,
vet, lint, deadcode and local dogfood. The first gate failed only the incorrect
fixture expectation described above; the corrected fixture passes the focused
analyzer tests and final gate. No full precision-regression corpus or local
race tests ran. Beads `gohawk-dho.12` owns this consolidation; the broader
architecture and remaining easy-FP goal is still active.
