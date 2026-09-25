---
title: FAQ
description: What gohawk checks, how it reasons about Go programs, and how it fits alongside other analysis tools.
---

## What is gohawk?

gohawk is a set of static analyzers for Go. It looks for bugs in the parts of
your code that have to be started, stopped, or cleaned up: goroutines,
channels, locks, subprocesses, and open resources. It finds them by reading
your code, so nothing has to run.

It's meant to sit alongside tools like `go vet` and Staticcheck, not replace
them.

## Has gohawk found bugs in large-scale projects?

Yes, gohawk found an unreaped process in
[Docker/Moby](https://github.com/moby/moby/pull/53517) and a lock inversion in
[Caddy](https://github.com/caddyserver/caddy/pull/7968), and both projects
merged the fixes.

## What kinds of projects should use gohawk?

Almost any Go project can. It helps most in code that starts goroutines, runs
subprocesses, or holds locks, which covers most servers, command-line tools,
and background workers.

## What are some advanced bugs that gohawk can catch?

gohawk follows your code down every path it can take, so it spots problems a
line-by-line check would miss. Here are three.

A process that starts, but isn't waited for on one path:

```go
func run(ctx context.Context, wait bool) error {
  command := exec.CommandContext(ctx, "worker")
  if err := command.Start(); err != nil {
    return err
  }
  if wait {
    return command.Wait()
  }
  return nil // command is still running
}
```

Two functions that take the same locks in opposite orders, which can deadlock:

```go
func forward() {
  first.Lock()
  defer first.Unlock()
  second.Lock()
  defer second.Unlock()
}

func reverse() {
  second.Lock()
  defer second.Unlock()
  first.Lock() // opposite order can deadlock
  defer first.Unlock()
}
```

This lock-order check runs by default.

A goroutine that should be waited for, but one path returns early:

```go
func refresh(skipWait bool) {
  var group sync.WaitGroup
  group.Add(1)
  go func() {
    defer group.Done()
    updateCache()
  }()
  if skipWait {
    return // goroutine is not joined
  }
  group.Wait()
}
```

The [analyzer reference](/analyzers/) lists every check, with examples of code
that gets flagged and code that doesn't.

## How does gohawk compare with other Go static-analysis tools?

gohawk is meant to work alongside your other analyzers, not replace them. Each
one covers different ground:

- [`go vet`](https://pkg.go.dev/cmd/vet) ships with Go and catches a small set
  of suspicious code. gohawk can run through `go vet` too.
- [Staticcheck](https://staticcheck.dev/) is a large, mature set of checks.
  gohawk goes deeper on one narrower topic: who owns a value and who cleans it
  up.
- [NilAway](https://github.com/uber-go/nilaway) finds possible nil panics,
  which gohawk doesn't try to do.
- [gosec](https://github.com/securego/gosec) looks for security problems, while
  gohawk looks for correctness bugs.
- [go-critic](https://github.com/go-critic/go-critic) has many quick checks for
  style and code quality. gohawk follows how values move through your program.
- [golangci-lint](https://golangci-lint.run/) isn't an analyzer itself. It runs
  many linters from one config, and gohawk can be one of them.

Using a few of these together catches more than any one alone.

## Does gohawk integrate with golangci-lint?

Yes. You can build gohawk into a custom golangci-lint binary with its
module-plugin system. The [golangci-lint guide](/golangci-lint/) walks you
through it.

## Will gohawk create a lot of noise if I add it to my project?

We work hard to keep it quiet. gohawk reports a problem only when it can see
both that something needs cleaning up and that it doesn't get cleaned up. When
it can't tell, it says nothing.

Checks come in two tiers: core and experimental. Only core checks run by
default, because they're the ones we trust most. See
[Configuration](/configuration/) to turn on the experimental ones.

No analyzer is perfect, though. If gohawk flags something that isn't a real
problem, please [open an issue](https://github.com/kojah/gohawk/issues) with
the check name, your gohawk version, and a small example if you can. It really
helps.

## What does gohawk deliberately not report?

gohawk doesn't treat "I can't see what happens here" as a bug. If a value
disappears into code it can't look inside, like a callback or a framework, it
counts that as unknown and leaves it alone.

So yes, it will miss some real bugs on purpose. We'd rather show you a few
findings you can trust than a pile you have to double-check. Checks that rely
on educated guesses stay in the experimental tier, which is off by default.

## I already use `go test -race`. Are gohawk's concurrency checks redundant?

No. They catch different things, and they work well together.

The race detector watches your tests run. It finds races that actually happen
during those runs, so a race on a path your tests never take can slip by.

gohawk reads the code instead, so it covers paths your tests might never reach.
It also finds concurrency bugs that aren't data races at all, like locks taken
in opposite orders or goroutines nobody waits for.

## What is SSA?

SSA, short for static single-assignment form, is a way of writing out a program
so that every value is set exactly once. It makes branches, loops, and the
origin of each value easy to follow.

gohawk uses it to answer questions like "is this file closed on every way out
of the function?" and "was this value handed off to someone else?"

[Understanding SSA](/understanding-ssa/) explains it visually, with
Go examples.

## What is gohawk's reasoning model?

For lifecycle checks, gohawk works in three steps:

1. Find something that has to happen, like a file that must be closed or a
   goroutine that must be waited for.
2. Follow that exact value and sort what happens to it: it was cleaned up, it
   was handed to a new owner, it went somewhere gohawk can't see, or it
   doesn't matter.
3. Check that every way out of the function is covered.

When a value goes somewhere gohawk can't see, it stays quiet rather than guess.

The [architecture guide](/architecture/#how-a-lifecycle-analyzer-is-shaped)
goes into more detail.

## How does gohawk see through function calls?

gohawk writes a summary of each function: what it needs from its arguments
(its precondition) and what it has done to them by the time it returns (its
postcondition), including where each value ends up, such as a field, a global,
or a returned object. A caller uses the summary instead of reading the helper
again, even across packages. That is how gohawk tells a file handed to a new
owner from one that is simply lost.

The idea comes from Meta's [Infer](https://fbinfer.com/) and its
[Pulse](https://fbinfer.com/docs/checker-pulse) engine.

## How do I contribute to gohawk?

We'd love your help, whether it's a bug report, a false positive, or a pull
request.

The [contributing guide](/contributing/) shows how analyzers are built, tested,
and documented.
