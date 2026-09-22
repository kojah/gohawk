# Remaining miscellaneous findings and producer experiment

This review covers the 18 remaining non-resource findings outside the lock,
goroutine, process, and cancellation slices: seven `producerlifecycle`, five
`concurrentcapture`, four `deferinloop`, one `evalorder`, and one `channelsafety`.
Their original review evidence comes from the [337-site ledger](followup-337-results.tsv).

## Rejected producer narrowing

The experiment requires a complete ordered worker summary for every producer
contributing to the send count. If any summary is incomplete, the existing
proof returns unknown with reason `producer-effects-incomplete`. This avoids
treating an opaque service call as certain to return and reach its later send.
It uses the existing summary machinery rather than a scheduling engine or
framework-name exemptions.

The representative seven
[agola service sends](https://github.com/agola-io/agola/blob/a5ca7877e3b77d38c6fd24a0f16d7267fc2dea12/cmd/agola/cmd/serve.go#L164-L188)
follow long-lived service calls. Their apparent competing-send count ignores
service-return feasibility and the caller's process-exit lifecycle. That is a
real precision gap; simply keeping the warnings is not evidence they are valid.

However, complete protocol summaries also reject ordinary I/O work, calls that
compute payloads, and branches. Requiring completeness for the whole worker
can therefore suppress genuine abandoned sends whose channel usage is clear.
For example,
[smallstep's two copy workers](https://github.com/smallstep/cli/blob/b33bd263c77d8fbc01fdd003555e084c6c0e4444/internal/sshutil/shell.go#L308-L329)
both eventually send on an unbuffered channel, while the caller receives once.
Returning I/O in the other worker can leave its send permanently blocked.

This is a **rejected experiment, not a landed correction**. The replay measures
all 28 original producer true-positive sites from batches 51–55. The baseline
still detects 26; two pulsar loop findings were already absent before this
experiment. **The tentative narrowing loses all 26 baseline-detected bugs.**
It retains none of these genuine-bug controls while suppressing seven reviewed
false positives. The coordinating review rejected this boundary and restored
all producer source, fixture, and documentation changes. It removes too much
of the demonstrated practical value of the check. This decision does not
endorse retaining the seven noisy findings indefinitely or retire the analyzer.

The [46-site experimental ledger](followup-78-misc.tsv) preserves every original
explanation, pin, check, before/experimental status, scope, and receipt. Both
binaries successfully load all 22 package scopes, so the measured losses are
not unavailable repositories or disabled checks. There are no changed results
among the other eleven false-positive sites. **All 18 findings remain
unresolved in the final implementation.** The seven agola suppressions are not
counted as fixes. The tentative binary remains available for reproduction but
is not the shipping implementation; final producer source is unchanged.

A further replay with the final combined binary verifies the restoration:
all 26 baseline-detected producer controls report again, the two baseline
coverage gaps remain absent, and all 18 miscellaneous false positives remain
reported. All 22 final scopes load successfully. The ledger's final state,
exit code, and receipt columns record that actual replay, not an inference
from the source restoration.

## Other unresolved families

| Analyzer | Sites | Missing evidence; no exemption introduced |
|---|---:|---|
| `concurrentcapture` | 4 | Exact singleton/empty collection cardinality through helpers, with phase joins. |
| `concurrentcapture` | 1 | Exact key-to-mutex identity through `LoadOrStore` and helper summaries. |
| `deferinloop` | 1 | Collective resource ownership across a nested collection loop that can appear to execute zero times. |
| `deferinloop` | 1 | Private per-worker completion gates with no remaining lock participant. |
| `deferinloop` | 2 | Singleton iteration or a monotone guard allowing only one defer registration. |
| `evalorder` | 1 | Exact nonnil map plus object-decoding semantics that preserve the copied map header. |
| `channelsafety` | 1 | Intentionally recovered panic probe, not merely any send beneath a recover. |

These eleven sites retain their reviewed false-positive status and remain
unresolved. No unrelated analyzer implementation changes are proposed here.

## Replay provenance

Final measurements are complete and the experiment is rejected. Input/output
records preserve original verdicts separately from experimental absence or
coverage loss. The final-disposition column records the restored baseline
behavior, not a claim that the tentative binary retained those bugs.

Baseline `.build/gohawk-resource-tightening-v2`:
`03d703b2724c0f9f9a554417e6802ec3861287b39792eb62c8092c5939ff0f0a`.
Tentative `.build/gohawk-followup78-producer-v1`:
`a7f8a02e4a93b0a6314a5cefb9e13f51fc7c7b5b015af5a751e660c828260a59`.
Final combined `.build/gohawk-followup78-goroutines-v5`:
`5516cad4c83bffd8dca28713df53f8d3d1a463b838c23d302da9e10ddc257419`.
Final receipts: `.build/followup78-misc-final/`.
All binaries are immutable; intervening other-analyzer changes are not attributed to
the producer experiment.

The direct CLI uses `-enable-all -gohawk-include-tests -json` on exact package
scopes, after checking repository SHA. Environment: `PROTO_REPORTER=text`,
`CGO_ENABLED=0`, `GOWORK=off`, `GOMAXPROCS=2`, `GOFLAGS=-mod=readonly`,
`GOTOOLCHAIN=local`. At most two external static-analysis processes run for this
slice at a time, within the coordinated four-process budget. No candidate tests,
generators, or applications are executed.
Valid diagnostic JSON with exit 3 is a successful scan; load failures never
count as suppression or lost coverage.

A traced smallstep replay confirms the check remains enabled in the experiment:
its genuine finding at `shell.go:317` is declined specifically as
`producer-effects-incomplete`, outcome `unknown`. Standard output remains valid
JSON, standard error is empty, and exit 3 reflects other diagnostics.
Receipts: `.build/followup78-smallstep-producer-trace.jsonl`, `.stdout`, and
`.stderr`.
