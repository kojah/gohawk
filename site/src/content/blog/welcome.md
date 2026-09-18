---
title: Bring Your Own SSA
description: Building gohawk taught me that SSA can show what a program does, but not what its operations mean.
date: 2026-09-11
draft: true
---

## Go had already done the hard part. Or so I thought

I came into this project with a comfortable assumption. Go ships [`go/ssa`](https://pkg.go.dev/golang.org/x/tools/go/ssa), which lowers a function into static single-assignment form, and [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis), a framework for writing modular analyzers that plug straight into `go vet`. Between the two you get exact values, an explicit control-flow graph, and a fact mechanism that survives package boundaries. If you have ever tried to do serious analysis on a language that hands you a raw syntax tree and wishes you luck, you know how much that is. Half the war looked won before I had written a line.

So the question that stuck with me early was not "how do I get at the program's structure." It was the opposite. If the infrastructure is this generous, why aren't there more deep, SSA-backed Go tools? There are plenty of good linters. Most of them read the syntax tree. Far fewer pick one specific value and follow it down every path, asking what has to happen to it before the function returns. If the raw material was sitting right there, the shortage seemed strange.

It took building the thing to understand the shortage, and the short version is that SSA gives you the map without telling you what any of the roads are for.

## SSA tells you what happened, not what it meant

If you have not looked at SSA before, the one rule that generates the rest is that every value is defined exactly once. There are no variables that get reassigned over time. Each assignment in the source becomes a fresh value with its own name, and every use points back to the single definition that produced it. The identity of a value is the instruction that made it.

A small subprocess function shows what that buys you:

```go
func run(ctx context.Context, shouldWait bool) error {
  command := exec.CommandContext(ctx, "worker")
  if err := command.Start(); err != nil {
    return err
  }
  if shouldWait {
    return command.Wait()
  }
  return nil
}
```

Lowered, it reads roughly like this:

```text
entry:
  t0 = exec.CommandContext(ctx, "worker")
  t1 = (*exec.Cmd).Start(t0)
  t2 = t1 != nil
  if t2 goto startFailed else startSucceeded

startFailed:
  return t1

startSucceeded:
  if shouldWait goto wait else leave

wait:
  t3 = (*exec.Cmd).Wait(t0)
  return t3

leave:
  return nil
```

The names are synthetic, but nothing else is invented. `t0` is the exact command `CommandContext` handed back. When `Wait` runs, it runs on `t0`, and there is no ambiguity about whether some other variable that happened to be spelled `command` might mean a different object. The branches have become edges between blocks, so "on every return path" stops being a figure of speech and becomes a walk over the graph. When values from different paths meet, SSA drops in a `phi` that names the predecessors it could have come from. Closures make their captures explicit. Calls, deferred calls, and goroutine launches each carry a distinct shape you can read off the instruction.

All of that is real, and it is why I could ask a question like "is `t0` waited for on every path out of this function" and actually get an answer instead of a heuristic.

Here is what SSA would not tell me. It says `Wait(t0)` happens on the `wait` branch and does not happen on `leave`. It does not say that this is a bug. It has no notion that starting a process creates an obligation to reap it, that `Wait` is the thing that discharges the obligation, or that returning through `leave` walks away from a live child. The graph records that an event occurred. The meaning of the event is not in the graph.

That gap turned out to be the whole project.

## The missing layer

Once I framed it that way, the shape of the problem got clearer, and so did the reason those deep tools are rare. Iterating over instructions is the part Go gives you. Deciding what the instructions mean for ownership and lifecycle is the part you bring yourself, and it is most of the work.

To report that subprocess safely, an analyzer has to stand up several separate claims. `Start` succeeded, so a process actually exists to reap. The value that got `Wait` is the same value `CommandContext` returned. Waiting is genuinely required here unless ownership moved somewhere else or the process was explicitly released. And at least one path that can really run does none of that. The first two of those come almost entirely from SSA. The last two are policy, and SSA has no opinion about policy.

The trouble is that real code does far more than call `Wait` on the next line. It defers the wait. It stashes the command in a struct and returns it. It hands the command to a helper. It launches a goroutine whose entire job is to wait. It registers the thing with some framework and never touches it directly again. Some of those genuinely finish the obligation. Some genuinely move it onto someone else. And some are opaque, where something clearly happened to the value but the analyzer cannot prove what.

What I settled on is deliberately small. Every instruction that touches the value after the obligation gets sorted into one of four buckets. A **join** finishes the obligation, like a call to `Wait`. A **transfer** moves it onto another object or goroutine that demonstrably takes over. **none** means the instruction has no bearing on the obligation at all. And **unknown** is anything the analysis cannot see through, an opaque call, a send on a channel, an `append`, a callback handed off to code the analyzer does not model. Then a single flow query asks whether joins and transfers cover every return path.

The load-bearing decision is what happens to `unknown`. It suppresses the diagnostic. If the only thing standing between a started process and a return is an operation the analyzer cannot interpret, gohawk stays quiet rather than turning its own ignorance into a warning. That produces false negatives, and I chose them on purpose. A correctness tool dies the moment people learn they have to re-derive each finding to see whether it is real. I would rather miss a bug at the edge of the model than ship one the model cannot actually back up. The project's guidance puts it bluntly, prioritize precision over recall, and this classifier is where that trade physically lives.

## A borrowed idea, not a borrowed architecture

While I was working out what belonged in that layer, I kept coming back to the [Clang Static Analyzer](https://clang.llvm.org/docs/ClangStaticAnalyzer.html). Most people meet Clang as a compiler frontend. What I cared about was its analyzer, which does path-sensitive, interprocedural work by symbolic execution. As it walks a program it carries a `ProgramState`, an abstract account of the values, the storage, and the constraints that hold at that point on that particular path.

Picture a C function that allocates memory, checks a condition, frees on one branch, and returns on the other. The syntax contains all four operations. The diagnosis lives in the state carried between them, where one branch releases the memory and the other reaches a return with the allocation still live. The analyzer is running a small evolving model of the program and asking whether a bad state can reach a given point.

I want to be careful not to overstate the lineage, because it is easy to dress up an influence as a pedigree. gohawk is not a port of the Clang analyzer. It does not copy its architecture and it does not do general symbolic execution. Go also erases whole classes of C memory bugs before analysis even begins, so the target is different. One idea carried over cleanly, and only one. A low-level representation earns its keep when you pair it with a higher-level state that assigns meaning to the operations flowing through it. For gohawk that state is not a simulation. It is a handful of small claims about exact values. This command started successfully. This path waits for it. This call moves it into an owner. That other call is opaque. Keeping the vocabulary tiny is exactly what makes the reasoning testable, and a bigger model would find more things while deserving less trust.

## A bug hiding in Docker

The abstraction stopped being theoretical the day it followed real code into [Docker](https://github.com/moby/moby/pull/53517). The affected function started an `nft` subprocess, then drove its input and output by hand instead of letting the standard library manage the lifecycle. Stripped down, the flow was:

```go
if err := command.Start(); err != nil {
  return err
}

if err := writeInput(command); err != nil {
  return err
}
if err := readOutput(command); err != nil {
  return err
}

return command.Wait()
```

The last path waited. Several of the earlier error paths did not. Once `Start` succeeds, returning ahead of `Wait` can leave the child unreaped and quietly discard its final status.

This is the kind of defect that reads as obvious once someone points at it and is genuinely awkward to catch with a line-oriented rule. The analyzer has to start from the *successful* `Start`, carry that same command value through the later calls, and check every reachable return against it. It has to throw out the `Start`-failure path, where no child exists to wait for. And it has to keep quiet about all the legitimate shapes that look superficially similar, handing the command to a long-lived owner, waiting inside a launched goroutine, deliberately calling `Process.Release`. Each of those is a join or a transfer, and none of them should draw a warning.

The missing waits also sat inside a bigger process-management problem. The code drained stdout before stderr, so a chatty child could fill the stderr pipe and deadlock while the parent sat blocked on stdout. The fix that merged handed the streams and the whole lifecycle back to `os/exec` through `Cmd.Run`, which handles both output streams and waits. Watching the tool follow a mature codebase far enough to surface something reproducible and fixable upstream was the moment I stopped worrying that it only recognized its own fixtures.

## Getting facts across a package boundary

Every example so far lived inside a single function, and real programs refuse to be that tidy. Hand the command to a helper and the caller's SSA no longer contains a call to `Wait`:

```go
func Reap(command *exec.Cmd) error {
  return command.Wait()
}

func run(ctx context.Context) error {
  command := exec.CommandContext(ctx, "worker")
  if err := command.Start(); err != nil {
    return err
  }
  return Reap(command)
}
```

What the caller sees is a call to `Reap`. If `Reap` lives in another package, its body may not even be loaded when the caller is analyzed. gohawk runs one package at a time under `go vet`, type-checking dependencies from export data rather than pulling the entire program into memory at once, which is what keeps it usable on projects with enormous dependency trees. Treating every helper as opaque would be sound. It would also stop the analysis dead at the first abstraction boundary, and Go code is essentially nothing but abstraction boundaries.

The framework's answer is facts. An analyzer can summarize something it proved about an exported function and attach that summary to the function's object, and an importing package reads the summary later instead of re-analyzing the dependency. For the helper above the summary amounts to one bit, that parameter zero is always waited on. gohawk sets that bit only when waiting is unavoidable on every normal return from `Reap`. A conditional wait would not earn it. When the caller reaches `Reap(command)` it imports the summary, maps the parameter back to the exact command value, and records a join.

The interesting part is how little an imported fact is permitted to say, because the limits are what make it safe. A fact describes one parameter at a time and never how two values relate. It records only what holds on every normal return, so a maybe collapses into the same clear bit as a never. It is computed once per function rather than once per call site. And it exists only for a callee the analyzer can name directly, whose declaration is exported. A method reached through an interface gets no imported fact. Neither does a package-internal helper.

One rule sits underneath all of that and I lean on it everywhere now. A missing fact means *unknown*, never *false*. Maybe the callee was unexported. Maybe it was reached through dynamic dispatch. Maybe its package was not analyzed by the driver. None of those is evidence that cleanup failed to happen, so absence suppresses the diagnostic rather than proving one. Evidence used to raise a finding has to be exact. Evidence used only to hold one back is allowed to guess on the cautious side, because an extra suppressing bit can hide a real bug but can never manufacture a false one.

## What it changed about how I think

The lock-order analyzer later found the same shape somewhere completely different, a [lock inversion in Caddy](https://github.com/caddyserver/caddy/pull/7968) where one error path took a per-entry lock and then a pool lock, while another path took the two in the opposite order. Different analyzer, different proof, and the same underlying move. Reduce the control flow to a small set of facts, then ask whether those facts admit an unsafe path. Worth noting that the two locks there are compared by their field declarations rather than by object identity, because nothing in the code settles whether one method's receiver is the object another method locks, and pretending otherwise would have meant guessing.

I started this thinking SSA was the advanced part and the rest would follow. It was the opposite. SSA is the shared vocabulary everyone begins from, and the deep checks do not come from chasing ever more elaborate syntax patterns on top of it. They come from choosing a small semantic vocabulary, propagating its facts with some discipline about what a fact is allowed to claim, and being willing to answer "unknown" the moment the evidence thins out. Go hands analyzer authors a genuinely excellent starting position. It just leaves you to bring your own meaning to its SSA, and I no longer think that is a gap in the tooling so much as an honest description of where the real work was all along.

If you want the mechanics rather than the story, the docs cover [how a lifecycle analyzer is shaped](/architecture/#how-a-lifecycle-analyzer-is-shaped), [how to read the SSA the analyzers see](/development/understanding-ssa/), and [what the cross-package facts can and cannot express](/development/fact-model/); the source is [on GitHub](https://github.com/kojah/gohawk).
