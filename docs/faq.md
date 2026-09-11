---
title: Frequently asked questions
description: What gohawk checks, how it reasons about Go programs, and how it fits alongside other analysis tools.
---

## What is gohawk?

gohawk is a static-analysis suite for Go focused on concurrency and resource
management. It uses control-flow and data-flow analysis to find bugs involving
resources, goroutines, channels, locks, and other lifecycle-sensitive values
without executing the program.

gohawk is designed to complement tools such as `go vet` and Staticcheck rather
than replace them.

## Has gohawk found bugs in large-scale projects?

Yes. gohawk has been run against large open-source Go projects as part of its
precision testing. Findings have led to fixes that were reviewed and merged
upstream:

- [Docker/Moby: reap the `nft` process on errors](https://github.com/moby/moby/pull/53517)
  was found with `processownership`. The affected error paths could leave a
  child process unreaped, and the surrounding pipe handling could deadlock
  when the child produced enough error output.
- [Caddy: avoid a lock inversion after constructor failure](https://github.com/caddyserver/caddy/pull/7968)
  was found with `lockorder`. Two concurrent operations could acquire the same
  locks in opposite orders and deadlock on a rarely exercised failure path.

These projects have merged fixes originating from gohawk findings; this does
not necessarily mean that they run gohawk continuously in their own CI.

## What kinds of projects should use gohawk?

Most Go projects can use gohawk as an additional layer of safety. It is
especially useful for programs that:

- open files, HTTP response bodies, database handles, or compressors;
- start subprocesses;
- create timers or cancellation functions;
- launch goroutines or channel-based producers; or
- coordinate shared state with mutexes and other synchronization primitives.

This includes servers, command-line tools, infrastructure software, database
clients, background workers, and concurrent libraries.

## What are some advanced bugs that gohawk can catch?

gohawk looks for bugs that require more context than a simple syntax check can
provide. Examples include:

- a resource released on most return paths but leaked on one feasible error
  path;
- a subprocess started successfully but never waited on after a later
  operation fails;
- two locks acquired in contradictory orders in different parts of a package;
- a goroutine with a recognizable join mechanism that is not honored before
  every return;
- a producer goroutine that can remain blocked after its receiver stops
  listening;
- a send that remains reachable after the same channel has been closed;
- a derived cancellation function that is lost without being called or
  transferred to another owner; and
- a local variable mutated by goroutines launched repeatedly from a loop.

See the [analyzer reference](/analyzers/) for the complete catalog and examples
of flagged and accepted code.

## How does gohawk compare with other Go static-analysis tools?

**gohawk is meant to complement, not replace, your current suite of analyzers.**
Each of the following tools has a different role:

- [`go vet`](https://pkg.go.dev/cmd/vet) ships with Go and checks for a focused
  set of suspicious constructs. gohawk can run through `go vet` while adding
  deeper resource-lifecycle and concurrency checks.
- [Staticcheck](https://staticcheck.dev/) is a broad, mature collection of
  correctness, performance, simplification, and style checks. gohawk focuses
  more narrowly on ownership, feasible paths, and lifecycle-sensitive bugs.
- [NilAway](https://github.com/uber-go/nilaway) specializes in finding
  potential nil panics. That concern is largely separate from gohawk's
  resource and concurrency analysis.
- [gosec](https://github.com/securego/gosec) looks for security vulnerabilities
  in Go code, including security rules and taint-related analysis. gohawk is
  primarily concerned with correctness rather than security classification.
- [go-critic](https://github.com/go-critic/go-critic) provides a broad
  collection of code-quality, performance, and style diagnostics. Many of its
  checks identify local patterns, while gohawk concentrates on program flow
  and lifecycle evidence.
- [golangci-lint](https://golangci-lint.run/) is not a competing analyzer. It
  is a runner that provides a common interface for configuring and executing
  many Go linters, including gohawk through its module-plugin system.

Using several of these tools together provides broader coverage than choosing
only one.

## Does gohawk integrate with golangci-lint?

Yes. gohawk can be included in a custom golangci-lint binary using
golangci-lint's module-plugin system.

See the [golangci-lint integration guide](/golangci-lint/) for installation
and configuration instructions.

## Will gohawk create a lot of noise if I add it to my project?

gohawk is deliberately conservative about what it reports. Core diagnostics
require positive evidence of both an obligation and a violation; when
ownership or lifecycle behavior cannot be determined safely, gohawk generally
does not report a finding.

Checks are divided into core, extended, and experimental tiers. Core checks
have the strongest precision expectations and run by default, while less
established checks require explicit selection. See [Configuration](/configuration/)
for details.

No static analyzer is perfect. If gohawk reports something that is not
actionable, please [open a GitHub issue](https://github.com/kojah/gohawk/issues)
with the check name, gohawk version, and a minimized example when possible.

## What does gohawk deliberately not report?

gohawk does not turn missing information into evidence of a bug. Opaque
callbacks, registries, framework handoffs, interface calls, or ownership
transfers that the analysis cannot see through are treated as unknown. An
unknown result suppresses a default diagnostic rather than weakening the
standard of proof.

This means gohawk intentionally accepts some false negatives. Its core checks
favor a smaller set of actionable findings over broader coverage that depends
on project names, function names, or guesses about lifecycle behavior.
Heuristic audits remain opt-in at the experimental tier.

## I already use `go test -race`. Are gohawk's concurrency checks redundant?

No. The race detector and gohawk observe different kinds of evidence.

`go test -race` executes instrumented code and reports memory races that
actually occur during the tested execution. A race in an untested path, or one
that depends on timing that did not occur during the test, may remain hidden.

gohawk examines possible program paths without executing them. It can also
detect concurrency defects that are not memory races, such as contradictory
lock ordering, joins that are not honored, abandoned producer goroutines, and
sends after a channel has been closed.

The two approaches are complementary, and using both provides better coverage.

## What is SSA?

Static single-assignment form, or SSA, is an intermediate representation of a
program in which each computed value has a single definition. It makes control
flow, value provenance, branches, loops, and merges explicit.

gohawk uses SSA to answer questions such as whether a resource is released on
every feasible return path, whether a value was transferred to another owner,
and whether one operation can occur after another.

See [Understanding SSA](/development/understanding-ssa/) for a visual
introduction and examples from Go code.

## What is gohawk's reasoning model?

For lifecycle checks, gohawk follows a conservative three-stage model:

1. Find a concrete obligation, such as a resource that must be closed or a
   goroutine that promises to signal completion.
2. Classify what happens to the exact value as a join, ownership transfer,
   unknown operation, or unrelated operation.
3. Ask whether a valid action covers every relevant return path.

An unknown callback, registry, framework handoff, or opaque function call does
not become evidence of a bug. Instead, uncertainty suppresses the default
diagnostic. This intentionally favors fewer findings over findings that users
cannot trust.

The [architecture guide](/architecture/#how-a-lifecycle-analyzer-is-shaped)
describes this model in more detail.

## How do I contribute to gohawk?

Contributions are welcome, including bug reports, false-positive reports,
documentation improvements, analyzer ideas, fixtures, and implementation
changes.

Start with the [contributing guide](/contributing/), which explains how
analyzers are organized, tested, documented, and validated.
