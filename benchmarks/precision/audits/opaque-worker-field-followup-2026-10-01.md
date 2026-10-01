# Cleanup of captured worker owner fields

Correction `65f3a0a` removes Lynx's reviewed goroutine false positive from the
[22-site scoped replay](pending-production-fp-replay-2026-10-01.md). Together
with the two previous storage/handoff corrections, three sites have verified
corrections and 19 remain unresolved. Original batch verdicts and replay
snapshots are preserved.

The [pinned worker](https://github.com/lynxbase/lynxdb/blob/7c4bf0432b0cef2807f0ddcd2cd2000ce7ffb8c1/pkg/ingest/receiver/otlpgrpc/server.go#L106-L121)
passes `r.server` to an imported method and then closes its completion signal.
The normal select arm receives completion; the timeout arm cleans up the same
captured field. The existing retained-owner classifier now maps that field
with shared `CallBindings` and `ProveIdentity`, requiring stable capture cells.
Only a nonblocking completion tail qualifies. The cleanup path is unknown,
never a guaranteed join or an inferred gRPC shutdown contract.

Different fields and owners, reassigned captures, visible no-op methods,
blocking sends and receives, and returns bypassing cleanup retain diagnostics.
The exact-field accepted fixture reports before the change and passes after;
trace assertions require the unknown classifier label and final decision.
A broader completion-mapper experiment was rejected because it suppressed the
existing ignored-wrapper control. The retained rule preserves that control
and does not infer ownership from constructor arguments. Hidden field mutation
and worker scheduling remain outside this bounded unknown-ownership model;
some genuine missing joins through opaque operations can consequently be missed.

## Scoped replay

The parent binary implements `c422971`, SHA-256
`cc850d926409bd91ccf2ef317319c6ef8968e4d64f34747a6cfe983314cf3ed9`.
The corrected binary was built before commit and implements `65f3a0a`, SHA-256
`e0d9a9b99e4d4455fd21f96582ffcbb0fd3d225ac0805a84dbc35529cf095432`. It matches the binary rebuilt by final `make verify`.
All scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, and
`go vet -vettool=<immutable binary> -enable-all -json`.
The [ledger](opaque-worker-field-followup-2026-10-01.tsv) records pins and receipts.

| Repository | Scope | Result |
| --- | --- | --- |
| lynxbase/lynxdb | `./pkg/ingest/receiver/otlpgrpc`, parent | Exit 0; reviewed `server.go:108:2` present. |
| lynxbase/lynxdb | Same scope, corrected with tracing | Exit 0; only finding difference is removal of that FP. |
| containerd/stargz-snapshotter | `./store`, corrected | Exit 0; reviewed abandoned-worker control `manager.go:193:2` retained. |

Lynx stderr is empty. Control stderr contains dependency-download messages
only. The Lynx trace `.build/goal-lynx-owner-fields.trace.jsonl` records
`closes-retained-owner`, phase `label`, outcome `unknown`, at the timeout
`Stop` call (`server.go:116:16`), then `opaque-ownership-transfer/unknown` at
the worker candidate. The normal completion arm remains a join.
These are successful affected-package scans, not complete repository scans.
Candidate tests, applications and generators were not executed. No full
precision-regression corpus ran. Final `make verify` and architecture tests pass.
Beads `gohawk-dho.8` owns the correction; the overall consolidation goal remains
active.
