---
title: Bring Your Own SSA
description: What building gohawk taught me about SSA, fact propagation, and precise static analysis in Go.
date: 2026-09-11
draft: true
---

When I started building gohawk, I assumed the difficult part would be gaining access to the compiler's view of a Go program. Go already provides [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis), a framework for writing modular static analyzers, and [`go/ssa`](https://pkg.go.dev/golang.org/x/tools/go/ssa), which turns Go programs into static single-assignment form. Surely most of the machinery for deep analysis was already there.

What I found was more complicated. SSA gives an analyzer the structure of a program, but not the meaning it needs to reason reliably about ownership, resource lifetimes, and concurrency. It can tell you that a value flowed into a call. It cannot tell you whether that call fulfilled an obligation, transferred the obligation to someone else, or made the answer unknowable.

Building that missing reasoning layer became one of the central challenges—and most interesting parts—of gohawk.

## A brief overview of SSA

Static single-assignment form rewrites a function so that every value is defined exactly once. Instead of following a source variable as it changes over time, an analyzer follows the specific instruction that produced each value.

Consider a deliberately small example:

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

A simplified version of its SSA looks like this:

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

The names are synthetic, but the important structure is real. `t0` is the exact command returned by `exec.CommandContext`. The result of `Start` is another value. The two `if` statements become edges between basic blocks, and every return is explicit.

This is already much easier to analyze than source text. There is no need to guess whether another variable named `command` is the same command. Every use refers back to `t0`. There is also no need to approximate the function's branches: the control-flow graph is right there.

When assignments from different branches meet, SSA uses a `phi` instruction to represent the possible incoming values. When a closure captures a variable, the capture is explicit. Calls, deferred calls, and goroutine launches all have distinct forms. These details make questions such as “does this exact resource get released on every return path?” tractable.

But not automatic.

## Surveying the landscape

Once I understood what Go exposed, I started looking for the layer above it. How were other analyzers turning syntax, types, and control flow into useful conclusions about real programs?

The analyzers included with Go were an obvious starting point. Beyond those, I kept returning to projects such as [Staticcheck](https://staticcheck.dev/), [NilAway](https://github.com/uber-go/nilaway), and [gosec](https://github.com/securego/gosec).

These tools cover very different territory. Staticcheck is a broad suite of correctness and quality checks. NilAway builds a model specifically for nil-safety. gosec looks for security problems. I learned something from each of them, but none provided a ready-made model for the question I wanted gohawk to answer: who owns a value, what must eventually happen to it, and whether that obligation is fulfilled along every feasible path.

That distinction matters. The interesting part of a static analyzer is rarely the API call that lets it iterate over instructions. It is the model that decides what those instructions mean.

I had initially thought of SSA as the advanced part. In practice, SSA was the common language. The real work began after I could read it.

## The nature of SSA-backed analysis

Return to the subprocess example. The SSA tells us that `Wait(t0)` occurs on one branch and not the other. It still does not tell us why this is a bug.

To report that safely, an analyzer needs to establish several separate claims:

1. `Start` succeeded, so a process lifecycle actually began.
2. The returned command is the same value as the receiver of `Wait`.
3. Waiting is required unless ownership is transferred or the process is explicitly released.
4. At least one feasible return path lacks all of those actions.

The first two claims come mostly from SSA. The latter two require a policy and an evidence model.

This is where precision becomes difficult. Real programs do more than call `Wait` directly. They defer cleanup, wrap values in structs, return owners, launch a goroutine whose job is to wait, pass resources through helpers, and register callbacks with frameworks. Some of those operations clearly discharge an obligation. Some clearly transfer it. Others are opaque: something happened to the value, but the analyzer cannot prove what.

gohawk's lifecycle analyzers classify those possibilities into a deliberately small vocabulary:

- **join**: the obligation is fulfilled, such as calling `Wait`;
- **transfer**: another object or goroutine demonstrably assumes ownership;
- **unknown**: the value enters code the analysis cannot see through; and
- **none**: the instruction has no bearing on ownership.

Then a single flow query asks whether a join or transfer covers every relevant return. If an opaque operation is the only thing standing between the acquisition and a return, gohawk does not turn missing knowledge into a diagnostic. It says nothing.

That choice creates false negatives. It is also one of the most important design decisions in the project. A correctness tool loses its value quickly if users have to reverse-engineer every finding to discover whether it is real. I would rather miss a bug at the boundary of the model than report one without enough evidence.

## Prior inspiration

While trying to work out what belonged above SSA, I kept returning to the [Clang Static Analyzer](https://clang.llvm.org/docs/ClangStaticAnalyzer.html).

Clang is familiar to many developers as a compiler frontend and as part of the toolchain behind Xcode. What interested me here was the architecture of its analyzer. It performs path-sensitive, interprocedural analysis using symbolic execution. As it explores a program, it carries a `ProgramState`: an abstract description of values, storage, and constraints that are known at that point on that path.

Imagine a C function that allocates memory, checks a condition, frees the allocation on one branch, and returns on another. The syntax tree contains all four operations. The useful diagnosis comes from the state carried between them:

```text
after allocation:  pointer p owns live memory
true branch:       memory released
false branch:      memory still live
return:            one feasible state retains the obligation
```

The analyzer is not merely searching for a missing call to `free`. It is evolving a model of the program and asking whether a bad state can reach a particular point.

gohawk is not a port of the Clang Static Analyzer, and its model is much narrower. Go also removes entire categories of C and C++ memory-safety errors. But the architectural lesson transferred cleanly: a low-level representation becomes powerful when it is paired with a higher-level state that gives operations meaning.

For gohawk, that state is not a general simulation of the program. It is a collection of small proofs about exact values: this command started successfully; this path waits for it; this call transfers it into an owner; this other call is opaque. Keeping the vocabulary narrow makes the reasoning easier to test and the resulting diagnostics easier to trust.

## Back to Go

Go's memory safety does not eliminate lifecycle bugs. A goroutine may need to terminate before its owner returns. A file or response body may need to be closed. A subprocess may need to be waited for. Locks may be acquired in an order that deadlocks only under a particular interleaving.

The subprocess rule eventually found a concrete example in [Docker/Moby](https://github.com/moby/moby/pull/53517). The affected code started an `nft` subprocess and then manually wrote its input and read its output streams. The shape was roughly this:

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

The final path waited. Several earlier error paths did not. Once `Start` succeeds, returning before `Wait` can leave the child unreaped and lose its final status.

This is exactly the kind of defect that looks local in hindsight but is awkward to find with a line-oriented rule. The analyzer has to begin at the successful `Start`, follow the same command through subsequent calls, and check every reachable return. It must exclude the `Start` failure path because no child exists there. It must also recognize valid alternatives such as handing the command to a long-lived owner, waiting in a launched goroutine, or deliberately calling `Process.Release`.

In Docker's case, the missing waits were part of a larger process-management problem. The code drained stdout before stderr, so a sufficiently noisy child could fill the stderr pipe and deadlock while the parent waited for stdout to close. The merged fix let `os/exec` manage the streams and lifecycle through `Cmd.Run`, which both drains the streams appropriately and waits for the child.

That result was an important milestone for me. gohawk had moved beyond recognizing a pattern in a fixture: it had followed enough of a real, mature codebase to find a bug that could be reproduced, tested, reviewed, and fixed upstream.

It was not the only one. The lock-order analysis later found a [lock inversion in Caddy](https://github.com/caddyserver/caddy/pull/7968): an error path acquired a per-entry lock and then the pool lock, while another operation acquired the same locks in the opposite order. That fix was merged too. Different analyzer, different proof, same basic idea—turn the program's control flow into a small set of facts, then ask whether those facts admit an unsafe path.

## Fact propagation

So far, the examples have stayed inside one function. Real code rarely does.

Suppose the subprocess is handed to a helper:

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

The caller's SSA contains a call to `Reap`, not a call to `Wait`. If `Reap` lives in another package, its body may not even be present in the caller's SSA. Treating every helper as opaque would avoid false positives, but it would also stop the analysis at the first abstraction boundary.

Go's analysis framework has an elegant mechanism for this: facts. An analyzer can summarize something it proves about an exported function and attach that summary to the function's object. When an importing package is analyzed later, it can read the fact without loading and re-analyzing the dependency's body.

For the helper above, gohawk's conceptual summary is:

```text
Reap(command *exec.Cmd)
  parameter 0: Waited
```

That bit is set only if waiting is unavoidable on every normal return from `Reap`. A conditional call to `Wait` would not earn the fact. Neither would the mere absence of a contradictory operation.

When gohawk reaches `Reap(command)` in the caller, it can import the summary, map parameter 0 back to the exact command value, and treat the call as a join. The caller remains small, the dependency does not need to be loaded into one whole-program graph, and the result participates in Go's normal package cache.

The fact model intentionally says very little. It records one parameter at a time. It can express lifecycle actions such as `Closed`, `Waited`, or `Stopped`, as well as specific ownership transfers. It cannot describe arbitrary relationships between two values, conditional cleanup, or facts about a dynamically dispatched interface call.

Just as importantly, a missing fact means **unknown**, not false. Perhaps the function was unexported, the callee was reached through an interface, or the package was unavailable to the analysis. None of those is evidence that cleanup did not happen.

This asymmetry became another recurring principle in gohawk. Evidence used to prove a diagnostic must be exact. Evidence used only to suppress a diagnostic may safely be conservative. If the analyzer suspects that a framework retains a value but cannot prove the transfer, classifying the call as unknown may hide a real bug; it cannot invent one.

Fact propagation let the model cross package boundaries, but the discipline around facts mattered more than the transport. The goal was never to build a perfect account of the whole program. It was to preserve the minimum evidence needed for a trustworthy proof.

## Conclusion

Go's analysis APIs made gohawk possible, but they did not make it automatic. SSA provided the map: exact values, instructions, and paths through a function. The harder work was deciding what those pieces meant for ownership, resource lifetimes, and concurrency.

That journey changed how I think about static analysis. The deepest checks do not come from searching for increasingly elaborate syntax patterns. They come from choosing a small semantic vocabulary, propagating its facts carefully, and knowing when the available evidence is not enough.

In that sense, Go gives analyzer authors an excellent starting point—and then asks them to bring their own meaning to SSA.

If you want to see the resulting model in more detail, the documentation covers [how gohawk's lifecycle analyzers reason](/architecture/#how-a-lifecycle-analyzer-is-shaped), [how to read its SSA output](/development/understanding-ssa/), and [what its cross-package facts can express](/development/fact-model/). The source is available on [GitHub](https://github.com/kojah/gohawk).
