# Remaining process and cancellation findings

Implementation: `4bf1a39`.

This is the process/cancellation slice of the remaining 78 non-resource
findings from the [337-site review](followup-337.md). All six process sites and
five cancellation sites were reassessed. The original false-positive labels
are preserved; an unresolved model gap does not become a valid diagnostic.

## Outcome

- **Two corrected:** sonar's fallback commands share a merged `Cmd.Wait`
  receiver. Both original reports disappear in a successful pinned replay.
- **Nine still unresolved:** four process and five cancellation findings.
- **Seven known-bug controls retained**, with no newly lost control.
- All **13 package scopes load successfully** with both binaries. The
  [18-site ledger](followup-78-process-cancel.tsv) records every input, control,
  pin, original explanation, outcome, and direct CLI receipt.

## Chosen narrowing

The existing process action classifier now returns uncertainty for an exact
`os/exec.Cmd.Wait` call whose SSA receiver is a merge that may contain the
started command. The ordinary flow still rejects successful return paths that
bypass that wait. It does not turn possible identity into a guaranteed wait,
and waits on a distinct command remain insufficient.

The representative
[sonar fallback](https://github.com/raskrebs/sonar/blob/9c963b8447d6ca08dd4a3c0bc6c0bf27527cd793/internal/runs/runs_test.go#L117-L130)
replaces the first command only after `Start` fails. Its SSA merges both
commands into one receiver, losing the correlation between acquisition success
and receiver identity. Narrowing the report is cheaper and safer than adding
an analyzer-specific path-correlation engine.

**Accepted coverage loss:** a merge can also choose the wrong command after a
successful start. Such a wait remains unknown and can hide a real missing
wait. This is documented in the fixture header, not labelled safe. The scoped
real-world controls show no additional losses, but cannot establish recall.

The action result preserves uncertainty for tracing: accepted, rejected, and
unknown decisions come from the same flow/classification path. The new reason
is `ambiguous-wait-ownership`; a possible handoff is not described as proven
cleanup. No shared SSA helper or fact vocabulary was changed.

## Remaining evidence gaps

| Family | Sites | Why retained as unresolved |
|---|---:|---|
| Parent-exit/daemon handoff | 1 | Requires a process-boundary handoff contract, not a CLI or daemon name. |
| Pre-start command wrapped by another owner | 1 | Mutagen's `NewStream` exports both `ReturnedOwner` and `ReturnedView`; post-start failure cleanup and success return need owner-aware path evidence. Mere pre-start wrapping does not prove later reaping. |
| Build-selected no-op error helper | 1 | Sonar's Linux helper returns nil, but the current flow does not prove that callee-result branch infeasible. No GOOS/name exemption was added. |
| Correlated nullable command guard | 1 | Gonc's final nil guard is correlated with successful launch; a may-alias wait alone does not rule out the bypass return. |
| Selected child-context Done branch | 1 | Gleam's `select` branch needs exact selected-channel and branch evidence. Seeing a Done case does not release obligations on every other case. |
| Command-lifetime signal registration | 3 | Rekor's final command lifecycle is not a reusable local proof of signal unregistration. No main/helper-name exemption was added. |
| Timer expiry before return | 1 | Sponge needs temporal evidence; a duration or sleep heuristic is not introduced. |

Cancellation production behavior is unchanged. Its documentation now states
these limits explicitly rather than implying elapsed time or process intent is
already modeled. No check was retired, disabled, or demoted.

## Provenance and validation

Baseline: `.build/gohawk-resource-tightening-v2`, source implementation
`56a8966`, SHA-256
`03d703b2724c0f9f9a554417e6802ec3861287b39792eb62c8092c5939ff0f0a`.
Corrected immutable binary: `.build/gohawk-followup78-process-v2`, SHA-256
`d3710e8c2d22f2462cf7208b333c50d491f2a101c52f982df707e117c9b84146`.
The corrected binary was built from the uncommitted, frozen process changes;
concurrent other-analyzer work is not attributed to this slice.

Both sides use the direct CLI, not an isolated-check profile:

```text
<immutable-binary> -enable-all -gohawk-include-tests -json <package>
```

Every checkout SHA is verified. Environment: `PROTO_REPORTER=text`,
`CGO_ENABLED=0`, `GOWORK=off`, `GOMAXPROCS=2`, `GOFLAGS=-mod=readonly`,
`GOTOOLCHAIN=local`. One external static-analysis process at a time; candidate
tests, generators, and applications are not executed. Exit 3 with valid
diagnostic JSON is a completed scan, not a loading failure.

Focused process and cancellation tests pass. Final process race tests pass
(13.847s); unchanged cancellation race tests pass (5.215s). Fixtures cover
fallback acquisition, unrelated wait receivers, and returns bypassing the
merged wait. The fallback fixture failed at both original acquisition sites
before the narrowing. Trace tests assert all three outcomes and associate
the unknown event with the merged-wait fixture.

The pinned sonar trace also emits `ambiguous-wait-ownership` with outcome
`unknown` for the acquisition at `runs_test.go:120`. Its standard output is
valid JSON (`{}`), standard error is empty, and the command exits successfully.
Receipts: `.build/followup78-sonar-trace.jsonl`, `.stdout`, and `.stderr`.
