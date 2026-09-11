---
title: Bring Your Own SSA
description: What building gohawk taught me about SSA, cross-package facts, and why precise Go analysis needs a conservative semantic model on top of the compiler's view.
date: 2026-09-11
draft: true
---

When I started building gohawk, I expected the hard part to be getting at the compiler's view of a program. It isn't. Go hands you [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis), a framework for modular static analyzers, and [`go/ssa`](https://pkg.go.dev/golang.org/x/tools/go/ssa), which lowers Go into static single-assignment form. Between them you get exact values, an explicit control-flow graph, and a fact mechanism that survives package boundaries. That is an unusually generous starting position. Many languages make you build some version of this yourself before you can ask a single interesting question.

So here is the puzzle that kept nagging at me. If the infrastructure is this good, why are there comparatively few deep, SSA-backed Go tools? Plenty of linters read the syntax tree. Far fewer follow a specific value along every path and reason about what must happen to it.

The reason, I came to believe, is that SSA is necessary but not sufficient. It gives you the *structure* of a program without the *meaning* you need to reason about ownership, resource lifetimes, and concurrency. SSA can tell you a value flowed into a call. It cannot tell you whether that call discharged an obligation, handed the obligation to someone else, or simply made the answer unknowable. Supplying that missing layer — a small, deliberately conservative model of what operations *mean* — turned out to be the actual project.

## A brief overview of SSA

Static single-assignment form rewrites a function so every value is defined exactly once. You stop tracking a source variable as it mutates over time; instead you track the specific instruction that produced each value, and every use names that one definition.

A small example makes the shape concrete:

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

The names are synthetic; the structure is real. `t0` is the exact command that `exec.CommandContext` returned. `Start` produces its own value. The two `if` statements have become edges between basic blocks, and every return is spelled out.

That is already far friendlier than source text. Nothing has to guess whether some other variable named `command` refers to the same object — every use points back to `t0`. Nothing has to approximate the branches, because the control-flow graph is sitting right there. When values from different branches meet, SSA inserts a `phi` instruction naming the possible predecessors. When a closure captures a variable, the capture is explicit. Calls, deferred calls, and goroutine launches each carry a distinct form. All of this makes a question like "is this exact resource released on every return path?" answerable in principle.

In principle. Not for free.

## Surveying the landscape

Once I understood what Go exposed, I went looking for the layer above it. How do existing analyzers turn syntax, types, and control flow into trustworthy conclusions about real code?

The analyzers bundled with Go were the obvious first stop. Past those, I kept circling back to [Staticcheck](https://staticcheck.dev/), [NilAway](https://github.com/uber-go/nilaway), and [gosec](https://github.com/securego/gosec).

These tools occupy very different territory, and — this matters — they do not all reason the same way. Staticcheck is a broad suite of correctness and style checks; some of its analyses use SSA, others work closer to the syntax. NilAway builds a dedicated model for one property, nil-safety, and propagates it interprocedurally. gosec leans toward pattern- and taint-style detection of security issues. I learned something distinct from each. None of them handed me a ready-made model for the specific question I wanted gohawk to answer: who owns a value, what must eventually happen to it, and whether that obligation is met on every feasible path.

That gap clarified something. The interesting part of an analyzer is rarely the API that lets you iterate over instructions. It is the model that decides what the instructions *mean*. I had filed SSA under "the advanced part." In practice SSA was the shared vocabulary — the thing everyone starts from. The real work began after I could read it fluently.

## The nature of SSA-backed analysis

Go back to the subprocess. The SSA tells me `Wait(t0)` happens on one branch and not the other. It does not tell me why that is a bug.

To report it *safely*, an analyzer has to establish several separate claims:

1. `Start` succeeded, so a process actually exists to reap.
2. The command returned by `CommandContext` is the same value that receives `Wait`.
3. Waiting is required unless ownership is transferred or the process is explicitly released.
4. At least one feasible return path does none of those things.

The first two claims come almost entirely from SSA — exact values and the branch structure give them to you. The last two need a policy and an evidence model that SSA has no opinion about.

This is exactly where precision gets hard, because real programs do far more than call `Wait` inline. They defer cleanup. They stash the value in a struct and return it. They hand it to a helper. They launch a goroutine whose whole job is to wait. They register callbacks with a framework. Some of those operations plainly discharge the obligation. Some plainly move it elsewhere. And some are opaque: *something* happened to the value, but the analyzer cannot prove what.

gohawk's lifecycle analyzers sort every operation after the obligation into a small, fixed vocabulary:

- **join** — the obligation is fulfilled, e.g. a call to `Wait`;
- **transfer** — another object or goroutine demonstrably takes over ownership;
- **unknown** — the value enters code the analysis cannot see through; and
- **none** — the instruction has no bearing on the obligation.

Then one flow query asks whether a join or a transfer covers every relevant return. The rule I care most about lives in how `unknown` is treated: it hides the diagnostic. If an opaque operation is the only thing standing between the acquisition and a return, gohawk does not convert missing knowledge into a warning. It stays quiet.

That produces false negatives, and I made my peace with them on purpose. A correctness tool loses its value the moment people have to reverse-engineer each finding to learn whether it is real. I would rather miss a bug at the edge of the model than emit one the model cannot back up. The project's guidance says it plainly: prioritize high precision over high recall. This is where that trade lives.

## Prior inspiration

While working out what belonged above SSA, I kept returning to the [Clang Static Analyzer](https://clang.llvm.org/docs/ClangStaticAnalyzer.html).

Most developers meet Clang as a compiler frontend, part of the toolchain behind Xcode. What drew me in was its *analyzer* architecture. It does path-sensitive, interprocedural analysis by symbolic execution: as it walks a program it carries a `ProgramState`, an abstract account of the values, storage, and constraints known at that point on that path.

Picture a C function that allocates memory, checks a condition, frees the allocation on one branch, and returns on the other. The syntax tree holds all four operations flatly. The diagnosis lives in the state carried between them:

```text
after allocation:  pointer p owns live memory
true branch:       memory released
false branch:      memory still live
return:            one feasible state retains the obligation
```

The analyzer is not just hunting for a missing `free`. It is evolving a model of the program and asking whether a bad state can reach a given point.

I want to be careful here, because it is easy to overclaim a lineage. gohawk is not a port of the Clang Static Analyzer. It does not copy its architecture, it does not do general symbolic execution, and its model is far narrower — Go also erases whole categories of C and C++ memory-safety bugs before analysis even starts. What transferred was one idea, cleanly: a low-level representation earns its keep when you pair it with a higher-level state that assigns meaning to operations.

For gohawk that state is not a simulation of the program. It is a handful of small proofs about exact values — *this command started successfully; this path waits for it; this call moves it into an owner; this other call is opaque*. Keeping the vocabulary tiny is what makes the reasoning testable and the diagnostics trustworthy. A bigger model would find more, and I would trust it less.

## Back to Go

Go's memory safety does not retire lifecycle bugs; it just changes which ones remain. A goroutine may have to finish before its owner returns. A file or a response body has to be closed. A subprocess has to be waited for. Locks can be taken in an order that only deadlocks under some interleaving.

The subprocess rule found its concrete example in [Docker/Moby](https://github.com/moby/moby/pull/53517). The affected code started an `nft` subprocess, then wrote its input and read its output by hand. Roughly:

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

The last path waited. Several earlier error paths did not. Once `Start` succeeds, returning before `Wait` can leave the child unreaped and throw away its final status.

This is the kind of defect that looks obvious in hindsight and is genuinely awkward to catch with a line-oriented rule. The analyzer has to start at the *successful* `Start`, follow that same command through the later calls, and check every reachable return. It has to exclude the `Start`-failure path, where no child exists. And it has to keep quiet about the legitimate alternatives — handing the command to a long-lived owner, waiting inside a launched goroutine, deliberately calling `Process.Release`. Each of those is a `transfer` or a `join`, not a violation.

In Moby's case the missing waits sat inside a larger process-management problem: the code drained stdout before stderr, so a noisy child could fill the stderr pipe and deadlock while the parent blocked on stdout. The merged fix handed the streams and the lifecycle to `os/exec` via `Cmd.Run`, which drains both and waits. Watching gohawk follow a real, mature codebase far enough to surface something reproducible, reviewable, and fixable upstream told me the model was doing more than recognizing a fixture.

It wasn't the only one. The lock-order analysis later found a [lock inversion in Caddy](https://github.com/caddyserver/caddy/pull/7968): one error path took a per-entry lock and then the pool lock, while another path took the same two in the opposite order. That fix merged too. Different analyzer, different proof, same underlying move — reduce the control flow to a small set of facts, then ask whether those facts admit an unsafe path. It's worth noting the two locks are compared by their field declarations, not by object identity, because nothing in the code settles whether one method's receiver is the object another method locks.

## Fact propagation

Every example so far stayed inside one function. Real code does not.

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

The caller's SSA contains a call to `Reap`, not a call to `Wait`. If `Reap` lives in another package, its body may not even be present when the caller is analyzed — gohawk runs one package at a time through `go vet`, type-checking dependencies from export data rather than loading the whole program into memory. Treating every helper as opaque would be sound, but it would stop the analysis dead at the first abstraction boundary, and Go code is nothing but abstraction boundaries.

Go's framework has a clean answer: facts. An analyzer can summarize something it proved about an *exported* function and attach that summary to the function's object. When an importing package is analyzed later, it reads the summary instead of re-analyzing the dependency, and the result rides along in Go's normal package cache.

For the helper above, gohawk's summary is conceptually just:

```text
Reap(command *exec.Cmd)
  parameter 0: Waited
```

That bit is set only if waiting is unavoidable on *every* normal return from `Reap`. A conditional call to `Wait` would not earn it. Neither would the mere absence of anything contradicting it. When gohawk reaches `Reap(command)`, it imports the summary, maps parameter 0 back to the exact command value, and classifies the call as a join.

The fact model says deliberately little, and its limits are the point. It describes one parameter at a time, never how two values relate. It records only what holds on every normal return — a maybe collapses to the same clear bit as never. It is computed once per function, not once per call site. It can carry discharge verbs like `Closed`, `Waited`, or `Stopped`, plus ownership transfers such as `ReturnedOwner`. It cannot describe conditional cleanup, arbitrary relationships between two values, or anything about a dynamically dispatched interface call.

Two constraints matter most, and both come straight from how facts flow. First, an imported lifecycle fact exists only for a function the analyzer can name directly and whose declaration is exported — package internals and interface calls get no imported fact. Second, and this is the load-bearing rule: **a missing fact means *unknown*, not *false***. Maybe the function was unexported. Maybe the callee was reached through an interface. Maybe the package wasn't available. None of those is evidence that cleanup didn't happen, so absence suppresses the diagnostic rather than proving one.

This asymmetry became a principle I lean on everywhere in gohawk. Evidence used to *prove* a diagnostic has to be exact. Evidence used only to *suppress* one is allowed to be conservative — to guess on the cautious side. If the analyzer suspects a framework retains a value but can't prove the transfer, classifying the call as `unknown` might hide a real bug. It cannot manufacture a false one. Fact propagation is what lets the model cross package lines, but the discipline around what a fact is permitted to claim mattered more than the transport ever did.

## Conclusion

Go's analysis APIs made gohawk possible; they did not make it automatic. SSA handed me the map — exact values, instructions, explicit paths through a function. The harder, longer work was deciding what those pieces *mean* for ownership, resource lifetimes, and concurrency, and being willing to say "unknown" whenever the evidence ran out.

The project changed how I think about static analysis. The deepest checks don't come from chasing ever-more-elaborate syntax patterns. They come from choosing a small semantic vocabulary, propagating its facts carefully, and knowing precisely when the available evidence is not enough to speak. Go gives analyzer authors a genuinely excellent starting point — and then leaves you to bring your own meaning to its SSA.

If you want the model in more detail, the docs cover [how a lifecycle analyzer is shaped](/architecture/#how-a-lifecycle-analyzer-is-shaped), [how to read the SSA the analyzers see](/development/understanding-ssa/), and [what the cross-package facts can and cannot express](/development/fact-model/). The source is on [GitHub](https://github.com/kojah/gohawk).
