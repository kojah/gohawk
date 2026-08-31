---
title: Configuration
description: Select analyzers, set analyzer flags, and suppress intentional findings.
---

This page explains how to choose which analyzers and checks run, configure
analyzer-specific options, and document intentional exceptions.

## Default and opt-in checks

An ordinary run uses gohawk's conservative default set. The analyzer catalog
marks anything outside that set with `*`. Opt-in analyzers can be selected by
name, as part of a group, or with `-enable-all`. An individually opt-in check
can be selected by its stable ID or with `-enable-all`.

Use `gohawk list` to see analyzers, or `gohawk list -checks` to see stable check
IDs. Default entries have no marker.

```sh
gohawk list
gohawk list -checks
```

## Select analyzers

Running gohawk without selection flags uses the default set. Entries marked
`*` in the analyzer reference are available but do not run automatically.
Use `-enable` to run named analyzers or `-disable` to remove analyzers from the
ordinary run. Selecting an analyzer runs its ordinary checks. Selecting a
group runs every analyzer in that group, including opt-in analyzers, but still
excludes checks that require individual selection. Disabled groups are removed
from the selected set. Group and individual selections combine, with individual
analyzer selections taking precedence.

```sh
# List every analyzer with its group; * means opt-in.
gohawk list

# Inspect an analyzer or one stable check.
gohawk doc contextpolicy
gohawk doc contextpolicy/nil-context

# List stable check IDs; * means opt-in.
gohawk list -checks

# List only defaults or only opt-in entries.
gohawk list -defaults
gohawk list -opt-in

# Run two opt-in analyzers.
gohawk -enable=wirepolicy,globalstate ./...

# Run every ownership and testing analyzer with their ordinary checks.
gohawk -enable-groups=ownership,testing ./...

# Add one analyzer from another group, or exclude one from a selected group.
gohawk -enable-groups=ownership -enable=wirepolicy -disable=channelpolicy ./...

# Run the ordinary set without the reliability and testing groups.
gohawk -disable-groups=reliability,testing ./...

# Run everything except testing analyzers.
gohawk -enable-all -disable-groups=testing ./...

# Run the ordinary set except oncepolicy.
gohawk -disable=oncepolicy ./...

# Keep contextpolicy enabled, but disable its context-storage check.
gohawk -disable-checks=contextpolicy/context-storage ./...

# Run one opt-in check by itself.
gohawk -enable-checks=contextpolicy/test-context ./...

# Add an opt-in check to an analyzer's ordinary checks.
gohawk -enable=contextpolicy -enable-checks=contextpolicy/test-context ./...

# Run every analyzer and every check, including opt-in checks.
gohawk -enable-all ./...
```

Analyzer names are values of `-enable` and `-disable`, not Boolean flags. Use
`-disable=wirepolicy` rather than `-wirepolicy=false`. The same selectors are
available when gohawk runs through `go vet -vettool`.

`-enable-checks` runs exactly the named checks when used alone and implicitly
activates their owning analyzers. When combined with an analyzer or group
selection, it adds those checks to the selected analyzers' ordinary checks.
`-disable-checks` removes individual checks afterward. Both flags accept
comma-separated stable IDs shown by `gohawk list -checks`. If all of an
analyzer's checks are disabled, gohawk skips that analyzer's analysis. Unknown,
repeated, and empty check IDs are errors rather than silent typos.

## Set analyzer options

Analyzer options use standard `go/analysis` flags and work with both gohawk and
`go vet -vettool=...`. Prefix an option with its analyzer name:

```sh
gohawk -enable=goroutineownership -goroutineownership.mode=join ./...
gohawk -enable=contextpolicy -contextpolicy.prefer-test-context=false ./...
```

Each configurable analyzer lists its options on its own page in the
[analyzer reference](../analyzers/).

## Preview and apply suggested fixes

Some diagnostics include a source edit that gohawk can apply automatically.
Use `gohawk doc <analyzer>` to check the `Suggested fixes` field for an
analyzer:

```sh
gohawk doc wirepolicy
```

Preview the edits as a unified diff without changing any files:

```sh
gohawk -enable=wirepolicy -fix -diff ./...
```

Apply those edits by omitting `-diff`:

```sh
gohawk -enable=wirepolicy -fix ./...
```

Analyzer, group, check, and option flags select diagnostics in the same way as
an ordinary run. A fix-capable analyzer may still omit a fix when it cannot
produce one safely for a particular finding. Review the resulting diff and run
the project's tests after applying changes:

```sh
git diff
go test ./...
```

## Suppress a finding

Put `//gohawk:ignore <analyzer> [reason]` on the flagged line or the line above:

```go
//gohawk:ignore goroutineownership worker belongs to the process lifecycle
go serveMetrics()
```

Ignores apply to one analyzer. The reason is optional.
