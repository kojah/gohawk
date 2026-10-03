# Conditional aggregate return correction

Beads `gohawk-dho.4.9.1` repairs the returned-logger FP rediscovered by the
[fixed production queue refresh](consolidated-production-fp-refresh-2026-10-03.md).
This follow-up replays the affected urunc package and the reviewed Promu leak
control. It does not replace that earlier 55-site receipt with a claim that
every site was replayed against this change.

## Evidence and change

At urunc pin `ef1dc96a6bf0c188fc7714200d66d95557ae8af3`,
`internal/metrics/metrics.go:61:16` acquires the file returned inside the logger
wrapper. Actual SSA follows `New`, `Level`, `With`, `Timestamp`, and `Logger`,
then stores the logger cell in the returned metrics object. `Timestamp` calls
[Logger.Hook](https://github.com/rs/zerolog/blob/116c8060e034e8d46855354d22db2acbc8df9e1e/log.go#L328-L337).
Hook returns the entry value when no hooks are supplied, or a modified copy
with a new hook slice. Both exits preserve the writer.

The old heap result projection required one aggregate identity across exits.
Hook's differing exit identities therefore lost every result field. The
caller then overwrote its nested logger with a multi-object aggregate result,
and Timestamp published unknown writer contents. Actual dumps are
`.build/goal-urunc-hook.heap` and `.build/goal-urunc-timestamp.heap`.

The existing heap-copy projection now names one by-value result and joins
bounded reference-field contents once per exit. Agreement can produce a must
field edge; replacement, stale contents and opaque writes cannot. Existing
field, slot and content-resolution bounds apply. Pointer results keep their
object-identity policy. No wrapper-depth increase, logging contract, fact-schema
change or analyzer-local traversal is introduced.

The heap regression fails on the original source because the conditional
result lacks `R0/field:0 -> P0/field:0 must`. With the repair, projection and
caller substitution preserve the writer identity; a conditional replacement
does not. Resource fixtures accept the returned conditional wrapper and retain
diagnostics for discarded and replaced writers.

## Pinned replay

The immutable `.build/goal-conditional-copy-reviewed` binary has SHA-256
`f526db1dec9897b3846ea7f92c7ed58e499359944eda27179e1ed46d21900c7f`.
It was built from `e890feda` plus this conditional-copy implementation before
the accompanying documentation edits. Each checkout matched its original pin
and was clean before scanning. Both invocations use
`go vet -vettool=<binary> -enable-all -json`, resource tracing,
`CGO_ENABLED=0`, `GOWORK=off`, and `GOFLAGS=-mod=readonly`.
Candidate tests, generators and applications are not executed.

| Repository and pin | Package | Complete findings | Original target |
| --- | --- | ---: | --- |
| urunc-dev/urunc, `ef1dc96a6bf0c188fc7714200d66d95557ae8af3` | `./internal/metrics` | 0 | `metrics.go:61:16` absent |
| prometheus/promu, `304b60c9fb862b9fa5d897e93901740da729c13b` | `./cmd` | 1 | `release.go:191:14`, `resourcelifetime/missing-release`, retained |

Both scopes terminate with exit zero and empty stderr. The urunc target has
151 trace records and ends accepted, with no target cutoff. Promu retains
`unowned-return` and its original diagnostic. The generic accepted trace label
is `release-proven`; it is not a claim that this factory explicitly closes the
file. The stronger mechanism evidence is the refreshed imported Timestamp
summary: its writer field now has
`R0/field:0/field:0 -> P0/field:0/field:0 must`, while the hook slice retains
may/unknown alternatives.

Complete JSON, traces, clean-pin checks, terminal receipts and refreshed facts
are under `.build/goal-conditional-copy-pinned/`. The other fifty-three queue
sites retain their earlier e395 observations, including the five assessed
larger-model sites. They receive no new correction or replay credit here.

## Validation

- Focused heap and resource suites pass in
  `.build/goal-conditional-copy-focused.log`.
- `make verify VERIFY_TIMINGS=1` passes all eight gates in
  `.build/goal-conditional-copy-verify.log`: generation, module verification,
  formatting, vet, dead-code, lint, self-analysis and ordinary tests.
- Final architecture verification uses
  `.build/goal-conditional-copy-final-architecture.log` after the development
  documentation edits.

The full precision-regression corpus is not rerun during this iteration.
