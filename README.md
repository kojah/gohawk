# GoHawk

GoHawk is a focused collection of static analyzers for Go. It currently
ships sixteen framework-neutral checks for API shape, concurrency and
resource ownership, determinism, serialization, tests, and error handling.

## Install

```sh
go install github.com/kojah/gohawk/cmd/gohawk@latest
```

Run GoHawk directly against package patterns:

```sh
gohawk ./...
```

It can also run as a `go vet` tool:

```sh
go vet -vettool="$(command -v gohawk)" ./...
```

## Analyzer utilities

The public [`analysisutil`](https://pkg.go.dev/github.com/kojah/gohawk/analysisutil)
package provides the shared AST, type, SSA, alias, control-flow, and ownership
operations used by GoHawk's analyzers. Other analyzer modules can import it to
build additional policy checks without maintaining copies of GoHawk's analysis
machinery.

```go
import "github.com/kojah/gohawk/analysisutil"
```

## Analyzers

| Analyzer | Policy |
| --- | --- |
| `apishape` | Flags exported APIs with error-prone parameter or receiver shapes. |
| `contextpolicy` | Checks context placement, storage, nil use, and test ownership. |
| `globalstate` | Flags mutable package-level state. |
| `wirepolicy` | Checks serialized structs and their composite literals. |
| `testpolicy` | Checks lifecycle ownership in test helpers. |
| `blockingtest` | Checks cancellation ownership for blocking test channels. |
| `goroutineownership` | Requires explicit goroutines to have a recognizable join owner. |
| `errorownership` | Detects double-handled errors and error-text classification. |
| `channelpolicy` | Checks channel capacity and closing ownership. |
| `processownership` | Requires started commands to have a wait owner. |
| `closedomain` | Finds builtin strings used as closed semantic domains. |
| `taintpolicy` | Checks untrusted environment and argument data reaching sensitive sinks. |
| `lockorder` | Detects contradictory mutex acquisition order. |
| `resourcelifetime` | Checks that owned resources are released on every path. |
| `determinism` | Detects unsorted map iteration reaching ordered output. |
| `cancellationownership` | Checks that derived context cancellation functions are called. |

These are intentionally opinionated policy checks. A deliberate error-text
match can be suppressed immediately above the call with a comment containing
`gohawk:error-text-match` and a rationale.

## License

Licensed under either the Apache License, Version 2.0 or the MIT License, at
your option.
