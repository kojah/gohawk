---
title: Configuration
description: Choose checks, set options, and handle intentional findings.
---

Running gohawk without selection flags runs its core checks:

```sh
gohawk ./...
```

Every check carries a tier that records how much trust it has earned:

| Tier | Runs by default | Meaning |
| --- | --- | --- |
| core | yes | precision demonstrated on the repository audit and guarded by the precision replay |
| extended | no | stable checks that encode a house rule a team may reasonably decline |
| experimental | no | heuristic audits that may change or be retired |

Use `gohawk list` to see every analyzer with its tier, and `gohawk list
-checks` for the checks themselves. An analyzer's tier is the most trusted
tier among its checks; it runs whenever one of its checks is selected.

```sh
gohawk list
gohawk list -checks
```

Raise the tier ceiling to run every check at or below a tier:

```sh
# Run core and extended checks.
gohawk -tier=extended ./...

# Run everything, including experimental audits.
gohawk -tier=experimental ./...
```

## Choose what runs

Use analyzer names with `-enable` and `-disable`:

```sh
# Run two extended analyzers.
gohawk -enable=wirepolicy,globalstate ./...

# Keep the default set, except oncepolicy.
gohawk -disable=oncepolicy ./...

# Run every analyzer and check at every tier.
gohawk -enable-all ./...
```

Naming an analyzer admits its core and extended checks. Its experimental
checks join only under `-tier=experimental` or when named by check ID.

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
# Run one extended check, whatever the tier ceiling.
gohawk -enable-checks=testlifecycle/context-root ./...

# Run one experimental audit alongside the core checks.
gohawk -enable-checks=goroutineownership/detached ./...

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

Evidence records carry the SSA text of the instruction they judged. To see the
surrounding SSA form, which is what the analyzers reason over, print it for a
package or one function:

```sh
gohawk ssa -func serveWorker ./internal/server
```

The dump uses the same builder as the analyzers, includes the function
literals created inside the selected function, and accepts `-tests` to load a
package's test variant. It is a debugging aid; SSA text changes across Go
versions, so do not pin it in fixtures.

Cross-package lifecycle evidence comes from summaries of exported functions.
To see what a package's functions are proven to do with their parameters, and
which imported summaries its calls resolve to, print the facts:

```sh
gohawk facts -func CloseAll ./internal/server
```

Each line names a parameter and the masks proven for it on every return, such
as `Closed` or `Invoked`. A summarized function always appears, even with no
proven mask; a function missing from the dump was never summarized, and
consumers treat calls to it as unknown.
