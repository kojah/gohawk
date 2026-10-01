# Returned logger: value-copy summary gap

The batch 63 production FP in `urunc-dev/urunc`, revision
`ef1dc96a6bf0c188fc7714200d66d95557ae8af3`, remains reported at
`internal/metrics/metrics.go:61:16`. The [assessment ledger](returned-logger-assessment-2026-10-01.tsv)
records it as unresolved. The original review remains frozen; no analyzer
correction or FP removal is claimed.

[NewZerologMetrics](https://github.com/urunc-dev/urunc/blob/ef1dc96a6bf0c188fc7714200d66d95557ae8af3/internal/metrics/metrics.go#L58-L74)
opens a file and builds `zerolog.New(file).Level(...).With().Timestamp().Logger()`.
It stores the result in a local logger cell, puts the cell's address into the
returned metrics object, and boxes that object as `Writer`. The pinned
[command setup](https://github.com/urunc-dev/urunc/blob/ef1dc96a6bf0c188fc7714200d66d95557ae8af3/cmd/urunc/main.go#L144-L155)
retains the returned metrics writer. This assessment concerns that structural
handoff; it does not infer a cleanup method from logging names.

## Evidence from SSA and imported facts

The real SSA contains five call results in the construction chain, followed by
a whole-value store into the logger cell and a pointer store into the returned
object. There is no unresolved interface dispatch in the construction chain.
The current trace ends with `unowned-return`.

The pinned dependency is zerolog v1.35.1, tag commit
`116c8060e034e8d46855354d22db2acbc8df9e1e`.
[New](https://github.com/rs/zerolog/blob/116c8060e034e8d46855354d22db2acbc8df9e1e/log.go#L246-L255)
conditionally replaces a nil writer and adapts its interface. Its imported
lifecycle fact establishes possible retention, not a parameter guaranteed to
be stored in the result on every return.
[Level](https://github.com/rs/zerolog/blob/116c8060e034e8d46855354d22db2acbc8df9e1e/log.go#L311-L314)
changes a scalar field in a value receiver and returns the copy.
[With](https://github.com/rs/zerolog/blob/116c8060e034e8d46855354d22db2acbc8df9e1e/log.go#L279-L290)
updates context storage and embeds that copy in another value.
[Logger](https://github.com/rs/zerolog/blob/116c8060e034e8d46855354d22db2acbc8df9e1e/context.go#L17-L19)
returns the nested logger. The imported facts for Level and Logger each show
only a fresh result root, without the parameter-relative writer-field edge.
The metrics factory exports no retaining-result guarantee either.

The missing relation reproduces without a logging dependency:

```go
type View struct {
    Out io.Writer
    Level int
}
type Context struct { View View }

func ChangeLevel(v View) View { v.Level = 1; return v }
func ExtractView(c Context) View { return c.View }
```

Each function currently publishes `heap edge R0 -> fresh(#1) must` and no
parameter claim. The ChangeLevel SSA spills the parameter whole into a local
cell, updates Level, then loads and returns the whole value. Out remains the
input's writer, but that field relation is not published. ExtractView similarly
does not publish the writer relation beneath its returned nested value.

This is evidence of missing field relationships, not a claim that every
unmentioned summary field proves absence of retention. Investigation should
also check completeness boundaries for copying untouched symbolic fields.

## Bounded counterfactual and next action

The baseline binary implements the production code committed in `e36fd6a`
(cohort record `cff5669`) and has SHA-256
`935a652ca97dbcdb1c793b49888de8ed71a3eb5812720a11e8950be41e2b1492`.
An isolated build changed only `maxWrapperChain` from four to eight; its hash is
`934928d748e7f6440a93daea1d531503bac480cc4180bb3082598448c1f2b487`.
Both static scans of `./internal/metrics` completed and emitted the same single
diagnostic at the reviewed site. The experimental worktree was removed; the
production bound remains four. Increasing traversal depth alone is insufficient.

The next target is bounded parameter-field/result-field composition in the
existing heap and fact publication layers, including value copies, unrelated
field replacement, and nested result projection. A returned-field relation
must distinguish replaced writer fields from preserved ones, and missing or
truncated relations must remain unknown. The conditional adaptation in New
is another limitation; fixing the copy relation alone is not yet evidence the
full FP will disappear.

No analyzer-local struct-copy traversal, type-name exemption, or logging API
catalog entry was added. Candidate tests, generators, and applications were
not executed. This is a package-scoped diagnostic assessment, not a new audit
or a cumulative precision replay.
