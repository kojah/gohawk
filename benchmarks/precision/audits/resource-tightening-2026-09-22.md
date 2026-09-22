# Resource lifetime: bounded precision tightening

This follow-up narrows `resourcelifetime/missing-release`; it does not retire,
disable, or demote the analyzer. The input is the fixed 210 resource sites from
the [337-site review](followup-337.md), alongside its 72 original resource
true-positive controls. Original verdicts remain separate from replay outcomes.

Implementation: `56a8966` (pushed to `main`).

## Outcome

The [282-site replay ledger](resource-tightening-2026-09-22.tsv) records every
input and control, including original review evidence and exact replay scope.

| Reviewed population | Result |
|---|---:|
| False positives newly absent | 33 |
| False positives already absent before this change | 36 |
| False positives still reported | 140 |
| Previously inconclusive site now absent | 1 |
| True-positive controls still detected | 66 |
| True-positive controls already missing in the baseline | 5 |
| Newly missed true-positive control | 1 |

All 104 package scopes successfully load with both binaries. Seventeen changed
or control scopes additionally pass direct all-enabled CLI replay; the geesefs
logger discrepancy is explicitly excluded from corrections. No new target-scope
diagnostic appears. These counts measure this reviewed population, not overall
precision or recall, and do not imply the remaining 140 reports are acceptable.

## Reassessment and chosen boundaries

Two recurring false-positive families admit conservative, bounded changes:

- **Cleanup capability is not acquisition evidence.** A custom type with
  `Close` can be an unopened lazy handle. Inferring a new resource obligation
  from every constructor returning that type invents obligations. Owned-field
  inference now uses the existing concrete resource vocabulary, not a generic
  `Close` method. This deliberately loses nested custom-owner acquisition
  inference; a recursive constructor-summary engine was not introduced.
- **A previously deferred closure can observe a later acquisition.** A cleanup
  callback registered before the acquisition can read a captured local cell
  populated afterward. Positive possible cleanup through that exact cell makes
  the missing-release proof unknown; it is not exported as guaranteed cleanup.
  Unrelated captured cells and resource-derived data are not sufficient.

The first family is represented by
[postgres-exporter's lazy instance constructor](https://github.com/prometheus-community/postgres_exporter/blob/e7e2095249dc369d943af1cef3d8c615228278ec/collector/instance.go#L31-L47):
the constructor validates a DSN with an already-closed database handle and
returns an instance with no live database. Its `Close` method does not prove
that constructing the instance acquired an obligation.

These are evidence-boundary changes, not project/function-name exemptions.
Direct known-resource owners and missing cleanup without a matching deferred
capture remain diagnostic in regression fixtures. Complexity remains bounded
by the existing summary and local completion queries; no path enumeration or
new recursive summary model was added.

## Replay provenance

The immutable baseline is `.build/gohawk-followup337-final-contained`, SHA-256
`3da9ce166dfd8dc72c3225dc1229298040ae27b04a394c49b2edcf7c3ca772a7`.
The immutable corrected binary is `.build/gohawk-resource-tightening-v2`,
SHA-256 `03d703b2724c0f9f9a554417e6802ec3861287b39792eb62c8092c5939ff0f0a`.
It contains the implementation committed as `56a8966` and was built from the
task's changes on `8dca7d6`; the baseline's source was
`5a56fb4`, with no intervening Go implementation changes before this task.

Both sides use identical pinned checkouts and all-enabled package scopes:

```text
go vet -vettool=<immutable-binary> -enable-all -gohawk-include-tests -json <package>
```

Environment: `PROTO_REPORTER=text`, `CGO_ENABLED=0`, `GOWORK=off`,
`GOMAXPROCS=2`, `GOFLAGS=-mod=readonly`, `GOTOOLCHAIN=local`.
Every scope verifies its checkout SHA against the input pin. This replay uses
at most three concurrent static-analysis processes, alongside at most one
coordinated validation scan. Candidate tests,
generators, and applications are never executed. A failed load is not an
absence or a successful correction.

The earlier isolated-check geesefs absence was profile-dependent; this replay
uses `-enable-all` on both binaries rather than attributing profile changes to
the implementation. The first current-binary run was interrupted when review
caught an overly broad data-dependency match; its receipts are superseded, not
used to claim fixes.

All 104 package scopes load successfully with both immutable binaries. Changed
sites are additionally replayed through the final CLI with the same all-enabled
profile, because the driver and direct `go vet` output can disagree about
reported diagnostics. The per-site record preserves the final authoritative
receipt, original verdict, exact checkout pin, check, and position.

## Coverage tradeoff

One original true-positive control becomes an accepted false negative:
[piko's successful WebSocket test](https://github.com/andydunstall/piko/blob/dd4356dfd307006380794952e4a8ccf9edcfa404/server/proxy/server_test.go#L360-L378).
It reads and writes the newly connected client but never closes it. Closing
the listening sockets does not close that accepted client. The acquisition
wraps a `gorilla/websocket.Conn` in a custom owner, so the narrowed inference
does not establish the wrapper's acquisition duty. This is not safe code and
has not been relabelled as a false positive.

Five wg-portal controls were already absent in the baseline and remain absent;
they are previous coverage losses, not caused by this change. The piko
`server/proxy/server_test.go:406:12` review remains inconclusive even though
the diagnostic disappears: a supposedly unreachable fixed localhost port
does not prove acquisition impossible.

## Validation

- Final `make verify` passes.
- Canonical precision round 58 passes 70 labels (60 false-positive controls,
  ten true-positive controls). Twenty-five labels remain excluded by incomplete
  loading in six repositories, matching the previous baseline; those are not
  counted as passes.
- Focused lifecyclefacts and resourcelifetime race tests pass. An earlier run
  exceeded the wall-clock budget test under concurrent load (16.22s against
  15s); the final rerun passed without loosening that budget.
- Pinned comqtt tracing emits `prior-defer-may-clean-captured-cell` at the
  expected HTTP acquisition. The CLI exits successfully, its standard output
  parses as JSON (`{}`), and standard error is empty. Evidence is recorded in
  `.build/resource-tightening-comqtt-trace.jsonl` and its `.stdout`/`.stderr`
  companions.
- Dependency diagnostics are not used to claim changes in target packages.
  In particular, an apparent Mmx233/BitSrunLoginGo `evalorder` loss in the
  `internal/config` scope is only dependency-output variation: directly
  replaying `pkg/srun` with both binaries still reports `api.go:102:40`.
  Receipts: `.build/resource-tightening-mmx-baseline.json` and
  `.build/resource-tightening-mmx-v2.json`.

The geesefs logger remains unresolved: direct CLI replay reports
`core/cfg/logger.go:37:16` with both binaries despite absence in one `go vet`
receipt. It is not counted as a correction. The discrepancy itself is outside
this bounded analyzer change and is not claimed fixed.
