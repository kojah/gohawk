---
title: Bring Your Own SSA
description: Notes from building gohawk — Go gives you SSA and a cross-package fact framework, but the meaning of a program's operations is the part you bring yourself.
date: 2026-09-11
draft: true
---

When I set out to build gohawk a couple of months ago, I was surprised to find the lack of viable alternatives.

Go is a language that's designed from the ground up for static analysis — only perhaps C# steals the throne from it.

The official tooling doesn't just hand you a syntax tree. Through [`x/tools/go/ssa`](https://pkg.go.dev/golang.org/x/tools/go/ssa) it will lower a function into static single-assignment form. Through [`go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis) it gives you a framework for writing modular analyzers that plug straight into `go vet`, with a fact mechanism that carries results across package boundaries.

There's nothing "native-plugin" about any of this; these are ordinary Go packages you import. The raw material for deep flow analysis is sitting right there.

## A brief overview of SSA

If you haven't run into SSA before, one rule generates everything else: every value is defined exactly once.

There are no variables that get reassigned over their lifetime. Each assignment in the source becomes a fresh value with its own name — usually written `tN` in a dump — and every use points back to the single definition that produced it.

The identity of a value is just the instruction that created it.

Control flow becomes explicit at the same time. A function is a list of basic blocks, each a straight run of instructions with one way in and one way out. `if`, `for`, `&&`, and `select` all turn into edges between blocks.

Where two branches assign what used to be the same variable, the block they merge into opens with a `phi` that names which predecessor each value came from.

That's the whole form, more or less. What it buys you is that a question like "is this specific value handled on every path that returns" stops being a matter of matching source patterns and becomes a walk over a graph.

## Surveying the landscape

So why, then, is it so uncommon for people to write their own SSA-backed analyzers in Go?

Aside from the analyzers in the Go standard library, there are only a few tools that reach past the syntax tree to do detailed flow analysis. I can count the ones I know on one hand:

- staticcheck
- nilaway
- gosec

Even that short list is less uniform than it looks. These tools use different mixtures of syntax, types, flow analysis, and interprocedural facts to answer different questions.

Staticcheck is a broad suite of checks. NilAway builds a dedicated model for nil-safety. gosec focuses on security problems, including taint-style analysis.

"SSA-backed" isn't one property these three share in exactly the same way.

Which leaves the obvious question:

If the SSA is right there in the toolchain, why doesn't everyone reach for it?

It took building gohawk to see the answer, and it wasn't the one I expected.

## The nature of SSA-backed analysis

As it turns out, it's not enough to just have SSA APIs for performing deeper inference. You'll often need to construct building blocks on top of those. Consider this small function:

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

`t0` is the exact command `CommandContext` returned. When `Wait` runs, it runs on `t0` — there's no ambiguity about whether some other variable that happened to be spelled `command` might be a different object.

The branches are edges now, so "on every return path" is a walk from the start block to each `return`.

Here's what the SSA will not tell you.

It records that `Wait(t0)` happens on the `do.wait` branch and doesn't happen on `skip`. It has no idea that this is a problem.

Nothing in the graph knows that starting a process creates an obligation to reap it, that `Wait` is the thing that discharges that obligation, or that returning through `skip` walks away from a live child.

The graph tells you what happened; it's silent on what any of it meant.

That gap is where the fact model goes.

To report the missing wait safely, gohawk builds a small semantic layer over the SSA. It finds the obligation — a `Start` that succeeded, tied back to the exact command value — and then labels every later instruction that touches that value as one of four things:

- A **join** discharges the obligation, like the call to `Wait`.
- A **transfer** hands it to some other owner.
- **none** is for instructions with no bearing on it.
- **unknown** is anything it can't see through: an opaque call into another package, a send on a channel, an `append`, or a callback passed to code it doesn't model.

Then one flow query asks whether joins and transfers cover every return.

The choice that matters is what `unknown` does: it suppresses the diagnostic.

If the only thing standing between a started process and a return is an operation gohawk can't interpret, it stays quiet rather than guess. That produces false negatives on purpose.

gohawk's own guidance is to value precision over recall, and this classifier is where that trade physically lives. A correctness tool stops being useful the moment people learn they have to re-check each finding by hand.

I'd rather miss a case at the edge of the model than raise one the model can't stand behind.

## Prior inspiration

Let's take a moment for something that might be completely off your radar. Have you ever used Xcode?

In the 2000s Apple had been using GCC to power the language features in its editor and found it slow and awkward to work with. So they sponsored Clang and LLVM as an alternative, which would go on to reshape a good part of the compiler world.

Part of that effort was the Clang Static Analyzer, and it was — arguably still is — remarkable for its time.

C++ has a huge gap between the set of programs the compiler will let you write and the set that are actually safe. There's enormous room for an analyzer that catches would-be runtime errors before they happen.

CSA has had a lot of time to mature. It works by symbolic execution: it walks the program path by path, carrying a `ProgramState` that records what it knows about each value, each region of storage, and the constraints in force at that point on that path. Take a C function that allocates, checks a condition, frees on one branch, and returns on the other:

```c
char *buf = malloc(n);
if (should_abort()) {
    return -1;   // buf leaks here
}
process(buf);
free(buf);
return 0;
```

The syntax has all four operations in it. The bug lives in the state carried between them — on the error branch the analyzer reaches a `return` with the allocation still recorded as live. Roughly, its state runs the pointer through a small machine:

```text
after malloc:   buf -> allocated
error return:   buf -> allocated   (reported: leak)
after free:     buf -> released
```

A checker fires when a path ends with a symbol still in the `allocated` state rather than passing through `released`.

I don't want to oversell the connection, because it's easy to dress an influence up as a pedigree.

gohawk is not a port of CSA. It doesn't copy its architecture, and it does no general symbolic execution. Go also erases whole categories of C memory bugs before analysis even begins, so the target is different.

One idea carried over, and only one: a low-level representation earns its keep when you pair it with a higher-level state that gives the operations flowing through it a meaning.

That's the entire reason the classifier above exists.

## Back to Go

While Go does not have the same memory safety issues that C++ developers have to worry about, there are still many issue classes that require an initial pass over the SSA graph for fact inference.

In gohawk, for example, you might want to make sure that a process you `Start` gets a matching `Wait`. That's a real check — `processownership` — and it's exactly the `run` function from earlier.

A command started with `os/exec` has to be waited on, or handed to something that will wait, before every successful return.

In the local proof, that comes out as a handful of small claims about the one command value:

- This command started successfully, so a child exists to reap.
- This path calls `Wait` on it: a join.
- That path returns having done nothing to it: a violation.

Cross-package summaries can later record a `Waited` bit for a helper parameter, but the local analyzer first has to prove that the call covers every normal return.

The vocabulary stays deliberately small.

### A real bug in Docker

The check stopped feeling academic the first time it followed real code into [Docker](https://github.com/moby/moby/pull/53517).

The function it flagged started an `nft` subprocess and then drove the process by hand instead of letting `os/exec` manage it. The final path waited. Several of the earlier error paths returned first.

Once `Start` has succeeded, returning ahead of `Wait` can leave the child unreaped and throw away its exit status.

It's the kind of bug that's obvious once someone points at it and genuinely awkward to catch with a line-oriented rule. Catching it means starting from the *successful* start, carrying that exact value through the later calls, and checking every reachable return against it.

At the same time, the analyzer has to stay quiet about shapes that only look similar: handing the command to a long-lived owner, waiting inside a launched goroutine, or deliberately releasing it. Each of those is a join or a transfer and shouldn't draw a warning.

The result was a reproducible bug and an upstream fix, rather than another fixture written for the analyzer itself.

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

All the caller sees is a call to `processutil.Reap`.

If `Reap` lives in another package, its body might not even be loaded when the caller is analyzed. gohawk runs one package at a time under `go vet`, type-checking dependencies from export data rather than pulling the whole program into memory. That's what keeps it usable on projects with huge dependency trees.

Treating every helper as opaque would be sound. It would also stop the analysis dead at the first function call, and Go code is mostly function calls.

The framework's answer is facts.

An analyzer can summarize something it proved about an exported function and attach that summary to the function's object. An importing package reads the summary later instead of re-analyzing the callee.

For `Reap`, the summary is essentially one bit: parameter zero is always waited on. gohawk sets it only when the wait is unavoidable on every normal return. A wait tucked behind an `if` doesn't earn it.

When the caller reaches `processutil.Reap(command)`, it imports that bit, maps the parameter back to the exact command value, and records a join.

What's striking is how little the fact is permitted to say. That's the part that keeps it safe.

It describes one parameter at a time, never how two values relate. It records only what holds on every normal return, so a maybe collapses into the same bit as a never. It's computed once per function, not once per call site.

And an imported fact exists only for a callee the analyzer can name directly and that is exported. A method reached through an interface gets no imported fact, and neither does an unexported helper.

Underneath all of it sits one rule I now lean on everywhere:

> A missing fact means *unknown*, not *false*.

Maybe the callee was unexported. Maybe it was reached through dynamic dispatch. Maybe its package wasn't analyzed by the driver.

None of that is evidence that the cleanup didn't happen, so absence suppresses the diagnostic rather than proving one.

Evidence used to raise a finding has to be exact. Evidence used only to hold one back is allowed to be cautious, because an extra suppressing bit can hide a real bug but can never invent a false one.

## Conclusion

I came into this thinking SSA was the hard, advanced part and the rest would fall out of it.

It was closer to the reverse.

SSA is the shared starting point — the vocabulary every one of these tools begins from. The depth doesn't come from chasing ever more elaborate patterns on top of it.

It comes from picking a small semantic vocabulary, propagating its facts with some discipline about what a fact is allowed to claim, and being willing to answer "unknown" the moment the evidence thins out.

The shape turned out to be reusable in places I didn't expect.

The lock-order analyzer runs the same play — reduce the control flow to a few facts, then ask whether they admit an unsafe path. It turned up a [lock inversion in Caddy](https://github.com/caddyserver/caddy/pull/7968): two locks taken in one order on one path and the opposite order on another.

There the locks are compared by the fields they're declared in rather than by object identity. Nothing in the code settles whether one method's receiver is the object another method locks, and pretending otherwise would have meant guessing.

That's the same instinct as `unknown`, in a different corner.

Go hands analyzer authors a genuinely good starting position. It just leaves you to bring your own meaning to its SSA, and after a couple of months of this I'm no longer sure that's a gap in the tooling so much as an honest description of where the work was always going to be.

If you'd rather read the mechanics than the story: [how a lifecycle analyzer is shaped](/architecture/#how-a-lifecycle-analyzer-is-shaped), [how to read the SSA the analyzers see](/development/understanding-ssa/), and [what the cross-package facts can and can't express](/development/fact-model/).
