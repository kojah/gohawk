---
title: Bring Your Own SSA
description: Notes from building gohawk — Go gives you SSA and a cross-package fact framework, but the meaning of a program's operations is the part you bring yourself.
date: 2026-09-11
draft: true
---

When I set out to build gohawk a couple of months ago, I was surprised by the lack of viable alternatives.

Go is designed from the ground up for static analysis — only perhaps C# steals the throne from it. [`x/tools/go/ssa`](https://pkg.go.dev/golang.org/x/tools/go/ssa) lowers functions into static single-assignment form, while [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis) provides a framework for analyzers that plug into `go vet` and carry facts across package boundaries.

The raw material for deep flow analysis is sitting right there.

## A brief overview of SSA

If you haven't run into SSA before, one rule generates everything else: every value is defined exactly once. Each source assignment becomes a fresh value, and each use points back to the instruction that produced it.

Control flow is explicit too. A function becomes basic blocks joined by edges; where branches merge, a `phi` identifies the value arriving from each predecessor.

That turns a question like “is this value handled on every return path?” from source-pattern matching into a graph walk.

## Surveying the landscape

So why, then, is it so uncommon for people to write their own SSA-backed analyzers in Go?

Aside from the Go standard library, I can count the tools I know that reach past syntax into detailed flow analysis on one hand:

- staticcheck
- nilaway
- gosec

Even this list is less uniform than it looks. Staticcheck is a broad suite, NilAway models nil-safety, and gosec focuses on security. Each uses a different mixture of syntax, types, flow analysis, and interprocedural facts.

Which leaves the question:

If the SSA is right there in the toolchain, why doesn't everyone reach for it?

It took building gohawk to see the answer.

## The nature of SSA-backed analysis

As it turns out, SSA APIs aren't enough. You still have to build the semantic layer on top. Consider this function:

```go
func run(ctx context.Context, wait bool) error {
  command := exec.CommandContext(ctx, "worker")
  if err := command.Start(); err != nil {
    return err
  }
  if wait {
    return command.Wait()
  }
  return nil
}
```

Lowered to SSA it reads roughly like this — the block names are synthetic, but nothing else is invented:

```text
entry:
  t0 = exec.CommandContext(ctx, "worker")
  t1 = (*exec.Cmd).Start(t0)
  t2 = t1 != nil
  if t2 goto start.failed else start.ok

start.failed:
  return t1

start.ok:
  if wait goto do.wait else skip

do.wait:
  t3 = (*exec.Cmd).Wait(t0)
  return t3

skip:
  return nil
```

`t0` is the exact command returned by `CommandContext`, and the branches are now edges. Checking every return path is a graph walk.

But the graph doesn't know that `skip` is a problem. It doesn't know that `Start` creates an obligation, that `Wait` discharges it, or that returning first abandons a live child.

SSA tells you what happened, not what it meant.

That gap is where the fact model goes.

gohawk fills that gap with a small semantic layer. It finds a successful `Start`, ties it to the exact command value, and classifies each later use:

- A **join** discharges the obligation, like the call to `Wait`.
- A **transfer** hands it to some other owner.
- **none** is for instructions with no bearing on it.
- **unknown** is anything it can't see through: an opaque call into another package, a send on a channel, an `append`, or a callback passed to code it doesn't model.

One flow query then asks whether joins and transfers cover every return.

The important choice is that `unknown` suppresses the diagnostic. When gohawk can't interpret an operation, it stays quiet rather than guess.

That creates false negatives on purpose. I'd rather miss a case at the edge of the model than raise one the model can't stand behind.

## Prior inspiration

Have you ever used Xcode?

In the 2000s, Apple found GCC slow and awkward for editor tooling and sponsored Clang and LLVM as an alternative. That effort also produced the Clang Static Analyzer (CSA).

CSA uses symbolic execution. As it walks each path, it carries a `ProgramState` describing values, storage, and constraints. Consider this allocation:

```c
char *buf = malloc(n);
if (should_abort()) {
    return -1;   // buf leaks here
}
process(buf);
free(buf);
return 0;
```

The syntax contains every operation, but the bug lives in the state between them. On the error branch, the function returns while the allocation is still live:

```text
after malloc:   buf -> allocated
error return:   buf -> allocated   (reported: leak)
after free:     buf -> released
```

A checker reports any path that ends in `allocated` rather than `released`.

gohawk isn't a port of CSA and does no general symbolic execution. One idea carried over: a low-level representation becomes useful when a higher-level state gives its operations meaning.

That's why the classifier exists.

## Back to Go

Go avoids many of C++'s memory-safety problems, but it still has obligations that require flow analysis. gohawk's `processownership` check, for example, ensures that an `os/exec` command is waited on—or handed to something that will wait—before every successful return.

Locally, the proof is only a few claims:

- This command started successfully, so a child exists to reap.
- This path calls `Wait` on it: a join.
- That path returns having done nothing to it: a violation.

The vocabulary stays deliberately small. Cross-package summaries can later record the same proof for helper parameters.

### A real bug in Docker

The check stopped feeling academic when it found a bug in [Docker](https://github.com/moby/moby/pull/53517).

The flagged function started an `nft` subprocess and drove it by hand. Its final path waited, but several earlier error paths returned first—potentially leaving the child unreaped and discarding its exit status.

A line-oriented rule can't see that. The analyzer has to follow the exact command from a successful `Start` to every reachable return. It also has to accept legitimate transfers, such as handing the command to a long-lived owner or waiting in a goroutine.

The result was a reproducible bug and an upstream fix.

## Fact propagation

Everything so far happened inside one function, and real programs won't cooperate. Move the wait into a helper and the caller's SSA no longer contains a call to `Wait` at all:

```go
// package processutil
func Reap(command *exec.Cmd) error {
    return command.Wait()
}

func run(ctx context.Context) error {
    command := exec.CommandContext(ctx, "worker")
    if err := command.Start(); err != nil {
        return err
    }
    return processutil.Reap(command)
}
```

The caller sees only `processutil.Reap`. If that helper lives in another package, its body may not be loaded: gohawk analyzes one package at a time under `go vet` rather than loading the whole dependency graph.

Treating every helper as opaque would stop the analysis at the first function call. The framework's answer is facts.

An analyzer can attach a proven summary to an exported function. Here, the summary is essentially one bit: parameter zero is always waited on. gohawk sets it only if every normal return includes the wait; a conditional wait doesn't count.

The caller imports that bit, maps the parameter back to its command value, and records a join.

These facts are deliberately narrow. They describe one parameter at a time, record only what holds on every normal return, and exist only for exported callees the analyzer can name directly. Interface dispatch and unexported helpers get no imported fact.

Underneath all of it sits one rule I now lean on everywhere:

> A missing fact means *unknown*, not *false*.

An absent fact could mean an unexported callee, dynamic dispatch, or a package the driver didn't analyze. None proves that cleanup failed, so absence suppresses the diagnostic.

Evidence used to raise a finding must be exact. Evidence used to hold one back can be cautious.

## Conclusion

I came into this thinking SSA was the hard part. It was closer to the reverse.

SSA is the shared starting point. The depth comes from choosing a small semantic vocabulary, propagating it carefully, and answering “unknown” when the evidence runs out.

The same shape later found a [lock inversion in Caddy](https://github.com/caddyserver/caddy/pull/7968): two locks acquired in opposite orders on different paths. There too, the analyzer had to stop where identity became uncertain rather than guess.

Go gives analyzer authors an excellent starting position. It just leaves us to bring our own meaning to its SSA—which may be an honest description of where the hard work was always going to be.

If you'd rather read the mechanics than the story: [how a lifecycle analyzer is shaped](/architecture/#how-a-lifecycle-analyzer-is-shaped), [how to read the SSA the analyzers see](/development/understanding-ssa/), and [what the cross-package facts can and can't express](/development/fact-model/).
