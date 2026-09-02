---
title: Configuration
description: Choose checks, set options, and handle intentional findings.
---

Running gohawk without selection flags uses its conservative default checks:

```sh
gohawk ./...
```

Use `gohawk list` to see available analyzers. Entries marked with `*` are
opt-in and do not run by default.

```sh
gohawk list
gohawk list -checks
```

## Choose what runs

Use analyzer names with `-enable` and `-disable`:

```sh
# Run two opt-in analyzers.
gohawk -enable=wirepolicy,globalstate ./...

# Keep the default set, except oncepolicy.
gohawk -disable=oncepolicy ./...

# Run every analyzer and check, including opt-in checks.
gohawk -enable-all ./...
```

You can also select a whole group:

```sh
# Run the ownership group.
gohawk -enable-groups=ownership ./...

# Run everything except the testing group.
gohawk -enable-all -disable-groups=testing ./...
```

To select one check rather than its whole analyzer, use the stable ID shown by
`gohawk list -checks`:

```sh
# Run one opt-in check.
gohawk -enable-checks=testlifecycle/context-root ./...

# Keep contextpolicy enabled, but omit one of its checks.
gohawk -disable-checks=contextpolicy/context-storage ./...
```

Selections combine. Individual analyzer choices take precedence over group
choices, and disabled checks are removed last. Selecting a check also enables
its analyzer. Unknown or repeated names are reported as errors.

Use `gohawk doc` when you want to inspect an analyzer or check:

```sh
gohawk doc contextpolicy
gohawk doc contextpolicy/nil-context
```

These selection flags also work when gohawk runs through `go vet -vettool`.

### Test files

Findings in `_test.go` files are skipped by default, except from the testing
group, whose analyzers take tests as their subject. Fixture files, table-driven
tests, and intentionally orphaned helper processes are usually true by the
letter of a policy and rarely worth acting on. To report them anyway:

```sh
gohawk -gohawk-include-tests ./...
```

The reviewed precision cohorts replay with this flag so labels inside test
files stay meaningful.

## Set analyzer options

Options are prefixed with the analyzer name:

```sh
gohawk -enable=goroutineownership -goroutineownership.mode=join ./...
gohawk -enable=channelcapacity -channelcapacity.max-unexplained-capacity=4 ./...
```

Each configurable analyzer lists its options in the
[analyzer reference](../analyzers/).

## Preview or apply fixes

Some diagnostics include a safe source edit. Preview edits as a diff:

```sh
gohawk -enable=wirepolicy -fix -diff ./...
```

Apply them by leaving off `-diff`:

```sh
gohawk -enable=wirepolicy -fix ./...
```

Not every finding can be fixed automatically. Review the resulting changes
and run your project's tests afterward.

## Ignore an intentional finding

Put an ignore comment on the flagged line or the line above it:

```go
//gohawk:ignore goroutineownership worker belongs to the process lifecycle
go serveMetrics()
```

An ignore applies to one analyzer. The explanation is optional, but adding one
helps future readers understand the decision.

## Investigate a decision

When a diagnostic is surprising, tracing can show why gohawk reported or
rejected it. Write the trace to a file:

```sh
gohawk -enable=goroutineownership \
  -gohawk-trace=goroutineownership \
  -gohawk-trace-file=trace.jsonl ./...
```

For a large run, narrow the output to a source location or function:

```sh
gohawk -gohawk-trace=all \
  -gohawk-trace-source=path/to/file.go:42 \
  -gohawk-trace-function=serveWorker \
  -gohawk-trace-file=trace.jsonl ./...
```

Tracing is off by default and does not change which diagnostics are reported.
Without `-gohawk-trace-file`, trace records are written to standard error.
