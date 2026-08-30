---
title: Configuring gohawk
description: Select analyzers, set analyzer flags, and suppress intentional findings.
---

## Tags and profiles

Tags belong to individual checks and explain why a finding matters:

<!-- gohawk:generated-tags:start -->
- <strong id="correctness">Correctness</strong> — Strong evidence that the program can behave incorrectly.
- <strong id="reliability">Reliability</strong> — Code that may work but is vulnerable to meaningful lifecycle, concurrency, or operational failures.
- <strong id="policy">Policy</strong> — A project convention on which reasonable teams may differ.
<!-- gohawk:generated-tags:end -->

Tags are composable: one check can have more than one tag. An analyzer may run
several checks with different tags, but the tags remain properties of those
checks rather than of the analyzer as a whole. Tags describe the nature of
findings; they are not severity levels.

Profiles answer a different question: whether something runs automatically.
Analyzer profiles are the outer gate: a **default** analyzer participates in an
ordinary run, while an **opt-in** analyzer must be selected explicitly. Check
profiles are the inner gate: a **default** check runs whenever its analyzer is
selected, while an **opt-in** check must be named explicitly or included by
`-enable-all`.

These two levels make it possible for a broadly useful analyzer to contain one
more opinionated check without enabling that check for everyone. Profiles do
not indicate severity, and an opt-in analyzer or check is not necessarily a
policy rule.

Use `gohawk list` to see analyzer profiles, or `gohawk list -checks` to see
check profiles.

```sh
gohawk list
gohawk list -checks
```

## Select analyzers

Running gohawk without analyzer flags uses the default analyzer profile and
the default checks within those analyzers. Checks marked "opt-in" in the
analyzer reference are available but do not run automatically.
Use `-enable` to run named analyzers or `-disable` to remove analyzers from the
default profile. Selecting an analyzer runs its default checks. Selecting a
group runs every analyzer in that group, including opt-in analyzers, but still
respects each check's profile. Disabled groups are removed from the selected
profile or group set. Group and individual selections combine, with individual
analyzer selections taking precedence.

```sh
# List every analyzer with its profile and group.
gohawk list

# Inspect an analyzer or one stable check.
gohawk doc contextpolicy
gohawk doc contextpolicy/nil-context

# List stable check IDs and their profiles.
gohawk list -checks

# List only one profile.
gohawk list -defaults
gohawk list -opt-in

# Run two opt-in analyzers.
gohawk -enable=wirepolicy,globalstate ./...

# Run every ownership and testing analyzer with their default checks.
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

# Run one opt-in check by itself.
gohawk -enable-checks=contextpolicy/test-context ./...

# Add an opt-in check to an analyzer's default checks.
gohawk -enable=contextpolicy -enable-checks=contextpolicy/test-context ./...

# Run every analyzer and every check, including opt-in checks.
gohawk -enable-all ./...
```

Analyzer names are values of `-enable` and `-disable`, not Boolean flags. Use
`-disable=wirepolicy` rather than `-wirepolicy=false`. The same selectors are
available when gohawk runs through `go vet -vettool`.

`-enable-checks` runs exactly the named checks when used alone and implicitly
activates their owning analyzers. When combined with an analyzer or group
selection, it adds those checks to the selected analyzers' default checks.
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

## Suppress a finding

Put `//gohawk:ignore <analyzer> [reason]` on the flagged line or the line above:

```go
//gohawk:ignore goroutineownership worker belongs to the process lifecycle
go serveMetrics()
```

Ignores apply to one analyzer. The reason is optional.
