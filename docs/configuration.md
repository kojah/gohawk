---
title: Configuring gohawk
description: Select analyzers, set analyzer flags, and suppress intentional findings.
---

## Select analyzers

Running gohawk without analyzer flags uses the default profile. Checks marked
"Opt-in" in the analyzer reference are available but do not run automatically.
Use `-enable` to run named analyzers or `-disable` to remove analyzers from the
default profile. Selecting a group runs every analyzer in that group,
including opt-in analyzers. Disabled groups are removed from the selected
profile or group set. Group and individual selections combine, with individual
analyzer selections taking precedence. See [Tags and profiles](../tags-and-profiles/)
for the meaning of each tag and how tags differ from profiles.

```sh
# List every analyzer with its profile and group.
gohawk list

# Inspect an analyzer or one stable check.
gohawk doc contextpolicy
gohawk doc contextpolicy/nil-context

# List the stable IDs accepted by -disable-checks.
gohawk list -checks

# List only one profile.
gohawk list -defaults
gohawk list -opt-in

# Run two opt-in analyzers.
gohawk -enable=wirepolicy,globalstate ./...

# Run every ownership and testing analyzer, including opt-in checks.
gohawk -enable-groups=ownership,testing ./...

# Add one analyzer from another group, or exclude one from a selected group.
gohawk -enable-groups=ownership -enable=wirepolicy -disable=channelpolicy ./...

# Run the default profile without the reliability and testing groups.
gohawk -disable-groups=reliability,testing ./...

# Run everything except testing analyzers.
gohawk -enable-all -disable-groups=testing ./...

# Run the default profile except oncepolicy.
gohawk -disable=oncepolicy ./...

# Keep contextpolicy enabled, but disable its context-storage check.
gohawk -disable-checks=contextpolicy/context-storage ./...

# Run every analyzer, including opt-in checks.
gohawk -enable-all ./...
```

Analyzer names are values of `-enable` and `-disable`, not Boolean flags. Use
`-disable=wirepolicy` rather than `-wirepolicy=false`. The same selectors are
available when gohawk runs through `go vet -vettool`.

`-disable-checks` removes individual checks after analyzers have been selected.
It accepts comma-separated stable check IDs shown by `gohawk list -checks`. If
all of an analyzer's checks are disabled, gohawk skips that analyzer's analysis.
Unknown, repeated, and empty check IDs are errors rather than silent typos.

## Set analyzer options

Analyzer options use standard `go/analysis` flags and work with both gohawk and
`go vet -vettool=...`. Prefix an option with its analyzer name:

```sh
gohawk -enable=goroutineownership -goroutineownership.mode=join ./...
gohawk -enable=contextpolicy -contextpolicy.prefer-test-context=false ./...
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
