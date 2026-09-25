# Analyzers have no options

Decided 2026-09-25.

## Decision

gohawk analyzers declare no flags. Users choose what runs with the selection
flags (`-enable`, `-disable`, `-enable-checks`, `-disable-checks`, groups, and
`-tier`), and every selected check behaves the same way in every project.
A test in `analyzers/analyzers_test.go` enforces this.

## Why

Three options existed:

- `goroutineownership.mode` (`context`, `lifecycle`, `join`) ran the proof in
  three ways. The two strict modes reported goroutines the default proof
  treats as owned or unknown, and seven branches across four files kept the
  modes apart.
- `resourcelifetime.contracts` switched off whole resource families.
- `resourcelifetime.require-memory-writer-close` reported compression writers
  over in-memory buffers.

The precision replay runs the defaults only, so every non-default mode was an
unaudited second decision path for the same check, against the one-proof rule
in `CLAUDE.md`. Turning a resource family off is suppression by
configuration: a family that misreports needs its pattern fixed. The
golangci-lint plugin never exposed any of them.

## What it rules out

A new behavior is a new check with its own ID, tier, fixtures, and audit
record, not a flag on an existing one. The gaps the strict goroutine modes
covered are recorded in the headers of the goroutineownership fixtures
`relay_dependencies.go`, `retained_context.go`, `opaque_context.go`, and
`retained_transport.go`.
