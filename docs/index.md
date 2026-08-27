---
title: gohawk
description: High-signal static analysis for Go.
template: splash
hero:
  tagline: Find ownership, API, and reliability bugs without teaching your team to ignore the linter.
  image:
    file: ../assets/gohawk-logo.png
    alt: A hawk sheltering the Go gopher
  actions:
    - text: Browse analyzers
      link: analyzers/
      icon: right-arrow
      variant: primary
    - text: View on GitHub
      link: https://github.com/kojah/gohawk
      icon: external
      variant: minimal
---

## Start with one command

```sh
go install github.com/kojah/gohawk/cmd/gohawk@latest
gohawk ./...
```

gohawk runs as a standalone CLI or through `go vet`. Start with every check,
then tune the few settings that represent real project-level choices.

<div class="signal-grid">
  <article>
    <p class="signal-number">01</p>
    <h3>API and data contracts</h3>
    <p>Catch parameter shapes, context misuse, weak closed domains, and brittle wire formats.</p>
  </article>
  <article>
    <p class="signal-number">02</p>
    <h3>Ownership and lifecycle</h3>
    <p>Track cancellation, channels, goroutines, processes, and resources across return paths.</p>
  </article>
  <article>
    <p class="signal-number">03</p>
    <h3>Reliability and safety</h3>
    <p>Find nondeterminism, confused error ownership, global state, lock-order conflicts, and unsafe data flow.</p>
  </article>
  <article>
    <p class="signal-number">04</p>
    <h3>Testing</h3>
    <p>Keep blocking waits cancellable and make helper lifecycle ownership explicit.</p>
  </article>
</div>

## Precision first

gohawk would rather miss an uncertain case than report a dubious one. A finding
should be specific, actionable, and backed by strong evidence. When the tool
does not understand a common ownership or cleanup pattern, that is an analyzer
problem to fix—not a reason to pile up suppressions.

## Fits the Go toolchain

```sh
# Run through go vet.
go vet -vettool="$(command -v gohawk)" ./...

# Preview safe fixes.
gohawk -fix -diff ./...

# Run selected checks.
gohawk -wirepolicy -globalstate ./...
```
