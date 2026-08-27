---
title: Configuration and suppressions
description: Select analyzers, set analyzer flags, and suppress intentional findings.
---

## Select analyzers

Running gohawk without analyzer flags enables every check. Name one or more
analyzers to run only those checks, or set a default analyzer to `false` to
exclude it.

```sh
# Run two analyzers.
gohawk -wirepolicy -globalstate ./...

# Run the defaults except globalstate.
gohawk -globalstate=false ./...
```

## Set analyzer options

Analyzer options use standard `go/analysis` flags and work with both gohawk and
`go vet -vettool=...`. Prefix an option with its analyzer name:

```sh
gohawk -goroutineownership -goroutineownership.mode=join ./...
gohawk -contextpolicy -contextpolicy.prefer-test-context=false ./...
```

Each configurable analyzer lists its options on its own page in the
[analyzer reference](../analyzers/).

## Suppress a finding

Put `//gohawk:ignore <analyzer> [reason]` on the flagged line or the line above:

```go
//gohawk:ignore goroutineownership worker belongs to the process lifecycle
go serveMetrics()
```

Ignores apply to one analyzer. The reason is optional.
