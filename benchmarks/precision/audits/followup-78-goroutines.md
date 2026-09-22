# Goroutine ownership follow-up: 24 remaining false positives

Implementation: `6e226c1`.

All 24 assigned sites were reassessed by evidence family and replayed at their
original pins. Seven reports are now absent; 17 remain reported. These 17 are
still reviewed false positives, not accepted permanent noise or relabelled bugs.
The changes retain the existing check and its default enablement.

The [96-site ledger](followup-78-goroutines.tsv) separates this batch from its
controls:

| Site group | Baseline | Final |
| --- | ---: | ---: |
| Assigned remaining false positives | 24 reported | 7 absent, 17 reported |
| Previously corrected false positives | 16 absent | 16 absent |
| Reviewed true-positive controls | 56 reported | 56 reported |

The final replay introduces no additional `goroutineownership` diagnostic in
the 66 scoped package outputs. There are no measured true-positive losses in
these controls; this is not a recall measurement for arbitrary programs.

## Implemented boundaries

The common direction is to preserve positive evidence already available to the
existing classifier, or to mark uncertain ownership as unknown. There is no new
protocol engine, summary family, shared-pass change, framework exemption, or
assumption about runtime durations.

| Evidence boundary | Corrected sites | Decision and limit |
| --- | --- | --- |
| Factory-returned aggregate copied into a local | Envoy `ratelimit_test.go:813`, tusd `s3store_part_producer_test.go:34` | The existing bounded `Storage.Content` query preserves the factory origin. It does not establish a fresh, exclusively local join obligation. Unknown, not proof that the companion channel is drained. |
| Completion channel loaded from an aggregate given to a helper | Galene `webclient.go:879` | Existing access-path and helper-use evidence recognizes a possible shutdown handoff. Losing the field identity yields unknown, never an exact join. A helper that only inspects the owner still does not settle it. |
| Exact locally canceled context handed to an opaque helper | cdebug `client.go:101` | Existing lifecycle capture evidence sees the same stable `WithCancel` result passed to the unavailable helper. Its exact cancel must cover every return. Unknown, not a claim that cancellation stops arbitrary I/O. |
| Terminal `WaitGroup.Done` with only completion defers afterward | Murphysec `nuget_cmd_build.go:611` | Only exact `WaitGroup.Done` and builtin `close` defers can remain. The existing caller `Wait` can use that alternative completion handle; real deferred work and arbitrary callbacks remain excluded. |
| Explicit channel stores through a type-asserted owner | goflow `segment_statements_test.go:92`, `strip_trivia_test.go:41` | Existing containment now keeps those stores when the same owner is subsequently handed to `Run`. The assertion is not unwrapped into guessed semantics. Merely constructing or filling the owner, or handing off another channel, remains reportable. |

Each boundary has minimized accepted and diagnostic fixtures. Commit-pinned
source links are kept at the implementation decision points. The original
real-world evidence remains in the per-site ledger.

### Coverage tradeoffs

Factory-created owners may actually be exclusive local owners; their abandoned
workers can now be missed through a by-value copy, consistently with existing
factory-channel uncertainty. An aggregate helper may act on another field, and
an opaque helper may ignore the context it receives. Those cases remain unknown,
not proven safe. Fixture headers and the analyzer documentation state these
limits. No existing diagnostic fixture was silently converted into an accepted
example, and no reviewed true-positive label was changed.

The first deferred-tail attempt was too broad: it also promoted a buffered send
before a deferred close into an additional completion signal. The Resinat/Resin
control at `internal/state/dirtyset_test.go:110` disappeared under that intermediate
v4 binary. Its drainer can spin forever because the caller steals remaining
entries. The final implementation limits the exception to exact `WaitGroup.Done`,
and a new buffered-result/nonblocking-poll regression fixture preserves the
distinction. The real Resinat control reports again under v5. The intermediate
loss is not accepted or hidden in the final totals.

## Remaining families

| Family | Remaining sites | Assessment |
| --- | ---: | --- |
| Transport shutdown | 9 | The worker often sees a buffered reader, stream, or opposite pipe endpoint while cleanup acts on the underlying transport. Preserve the known false-positive verdicts; a reusable owner/view relationship could help, but a `Close` name or a finite deadline is not by itself completion evidence. No new cross-object shutdown model was added. |
| Transitive completion | 3 | Kadeessh's timeout watchdog, Mutagen's error-path endpoint shutdown, and k8s-csi-s3's downstream consumer connect several participants. Exact alternative ownership or helper effects remain unresolved. The factory/aggregate fixes above do not cover these paths. |
| Alternative completion | 1 | Conduit's waiter only waits for the worker group and closes a notification; parent cleanup independently waits for that group. Relating a waited-on dependency to the waiter's own completion needs a separate bounded rule. Arbitrary unrelated waits are not interchangeable. |
| Receiver-stored cancellation | 1 | My-geektime's worker uses a context held on its receiver and a semaphore token. The new locally-created context rule does not establish that identity or the token lifecycle. No receiver or context-field name exemption was added. |
| Process lifetime | 2 | Bitrise's optional input reader and driftctl's immediate process-exit caller need an explicit lifetime boundary. CLI placement, comments, or a `main`-like name do not waive a join obligation automatically. |
| Repeated receiver-field guard | 1 | Ghostferry launches and joins beneath the same receiver-field condition. The present local flag proof does not correlate those field loads across intervening calls. No broad same-field exemption was added. |

These are unresolved modeling decisions, not a request to build one bespoke
model per site. Future work should first look for reusable identity, retention,
or possible-completion evidence at the existing classifier boundaries.

## Reproduction and validation

Both baseline and final runs used the direct CLI profile
`-enable-all -gohawk-include-tests -json .`, in the pinned package directory.
This deliberately avoids treating an explicit-check or raw `go vet` result as
equivalent to the canonical all-check output. Every checkout's `HEAD` was
verified against the ledger pin. Repository tests, applications, and generators
were not run.

- Baseline: `.build/gohawk-resource-tightening-v2`, SHA-256
  `03d703b2724c0f9f9a554417e6802ec3861287b39792eb62c8092c5939ff0f0a`.
- Final: `.build/gohawk-followup78-goroutines-v5`, SHA-256
  `5516cad4c83bffd8dca28713df53f8d3d1a463b838c23d302da9e10ddc257419`.
- Receipts: `.build/followup78-goroutines-baseline/` and
  `.build/followup78-goroutines-final/`; each records the binary hash, exact
  command, repository pin, package, exit status, findings, and per-site presence.
- All 66 scopes parse successfully with exit 0 or diagnostic exit 3 and no
  analyzer-error payloads. This is scoped-package validation, not a claim that
  every module of every repository loads successfully.
- Environment: `PROTO_REPORTER=text`, `CGO_ENABLED=0`, `GOWORK=off`,
  `GOTOOLCHAIN=local`, `GOFLAGS=-mod=readonly -p=2`, `GOMAXPROCS=2`.
  Final replay used two static scan processes, within the parent batch budget.
- Actual SSA dumps were inspected for Murphysec, tusd, cdebug, Galene, Envoy,
  and goflow before relying on the changed evidence. Traced pinned invocations
  confirmed the rejected/unknown boundary, and `-json` remained parseable.
- `go test ./internal/analyzers/concurrency/goroutineownership -count=1`
  passes, including eleven stable decision reason/outcome/candidate assertions.
- Focused golangci-lint reports zero issues. Focused race tests pass in 25.481s.
- The parent-run final combined `make verify` passes; receipt:
  `.build/followup78-final-verify.log`. The parent owns the combined round-58
  replay and repository-wide handoff.

Scratch replay driver: `.build/followup78-goroutines-replay.py`. Test and lint
receipts: `.build/followup78-goroutines-v5-{tests,lint,race}.log`. The immutable
final binary, rather than a subsequently changing shared working tree, is the
authority for the numbers above.
