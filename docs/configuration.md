---
title: Configuring gohawk
description: Select analyzers, set analyzer flags, and suppress intentional findings.
---

## Select analyzers

Running gohawk without analyzer flags uses the default profile. Checks marked
"Opt-in" in the analyzer reference are available but do not run automatically.
Name one or more analyzers to run only those checks, or set a default analyzer
to `false` to exclude it. See [Tags and profiles](../tags-and-profiles/)
for the meaning of each tag and how tags differ from profiles.

```sh
# List every analyzer with its profile and tags.
gohawk list

# List only one profile.
gohawk list -defaults
gohawk list -opt-in

# Run two opt-in analyzers.
gohawk -wirepolicy -globalstate ./...

# Run the default profile except determinism.
gohawk -determinism=false ./...

# Run every analyzer, including opt-in checks.
gohawk -enable-all ./...
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
