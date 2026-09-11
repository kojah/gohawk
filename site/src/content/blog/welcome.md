---
title: Bring Your Own SSA
description: What building gohawk taught me about SSA, fact propagation, and precise static analysis in Go.
date: 2026-09-11
draft: true
---

When I started building gohawk, I assumed the difficult part would be gaining access to the compiler's view of a Go program. Go already provides an analysis framework and an SSA representation, so surely most of the machinery for deep static analysis would already exist.

What I found was more complicated. SSA gives an analyzer the structure of a program, but not the meaning it needs to reason reliably about ownership, resource lifetimes, and concurrency. Building that missing reasoning layer became one of the central challenges—and most interesting parts—of gohawk.

This is the story of how I arrived there: what Go gives analyzer authors out of the box, what I learned from the existing static-analysis landscape, and why gohawk ended up needing a fact model of its own.

## A brief overview of SSA

Before getting to that reasoning layer, it helps to understand what SSA gives us.

<!--
Introduce only the concepts the rest of the post needs:

- basic blocks and control-flow edges;
- values that are assigned once;
- calls, branches, and returns;
- phi nodes where values from different paths meet; and
- closures or captured values, if the later example depends on them.

Keep this section short and use the same small Go example again in "The nature
of SSA-backed analysis." The goal is to give readers a working mental model,
not to teach SSA comprehensively.
-->

## Surveying the landscape

Once I understood what Go exposed, I started looking for the layer above it. How were other analyzers turning syntax and SSA into useful conclusions about real programs?

The standard library analyzers were an obvious starting point. Beyond those, the projects I kept returning to included:

- [Staticcheck](https://staticcheck.dev/), a broad suite of correctness and quality checks;
- [NilAway](https://github.com/uber-go/nilaway), which focuses on nil-safety; and
- [gosec](https://github.com/securego/gosec), which looks for security problems in Go code.

These tools made it clear that sophisticated analysis in Go was possible. They also helped clarify the problem I actually wanted to solve. I was not looking for another general lint rule or a model dedicated to one issue class. I wanted reusable reasoning about ownership, cleanup, and concurrency—the relationships that connect an operation in one part of a program to an obligation somewhere else.

<!--
Describe what was useful about studying each project without implying that all
three use SSA in the same way. The important observation is not that alternatives
do not exist; it is that their reasoning models solve different problems and are
not a ready-made foundation for gohawk's checks.
-->

## The nature of SSA-backed analysis

The first surprise was that obtaining SSA was only the beginning. SSA can show that a function allocates a resource, passes it to a helper, branches, and eventually returns. It does not automatically tell us who owns that resource, whether the helper assumes responsibility for it, or which cleanup action would discharge the obligation.

Consider a small example:

<!-- Show a real Go example from one of gohawk's analyzers. -->

<!-- Show a simplified, annotated SSA deconstruction of the same example. -->

At the SSA level, identify exactly what is visible: the values, calls, control-flow edges, and return paths. Then identify what is missing: the semantic relationship between acquiring something, transferring it, and eventually releasing or joining it.

That gap is where a fact model becomes necessary. The analyzer needs to infer a small number of defensible facts from the program and combine them into a reasoning chain. The hard part is not collecting as many facts as possible; it is deciding which facts are strong enough to support a diagnostic without creating noise.

## Prior inspiration

While trying to work out what belonged above SSA, I kept returning to the Clang Static Analyzer.

Apple's investment in Clang and LLVM was driven in part by a need for compiler infrastructure that could support better developer tooling. The result was not only a compiler frontend, but an ecosystem in which tools could reason about C and C++ programs using the compiler's own representation of the code.

<!--
Keep the history concise and link to authoritative sources for claims about
Apple, GCC, Clang, and Xcode.
-->

The Clang Static Analyzer is especially interesting because C and C++ leave a large gap between programs the compiler accepts and programs that are safe to execute. Its analyzer narrows that gap by tracking program state and building a chain of reasoning about what must be true at each point along a path.

<!--
Show one compact Clang example, followed by a simplified representation of the
state or facts involved. Choose an example that prepares the reader for gohawk's
model rather than becoming a detour into C++ memory safety.
-->

What I found compelling was not one particular check. It was the architecture: a low-level program representation becomes useful when it is paired with a higher-level model that can carry meaning through the program.

## Back to Go

Go eliminates many of the memory-safety problems that C and C++ developers have to contend with, but it still has obligations the type system does not enforce. A goroutine may need to terminate before its owner returns. A resource may need to be closed on every feasible path. Locks may be acquired in an order that can deadlock only under a particular interleaving.

Those are not merely properties of individual syntax nodes. They depend on relationships between values, calls, paths, and sometimes functions or packages.

<!--
Use an actual gohawk finding here. A resource- or goroutine-ownership example
will connect most directly to the post's thesis. Show:

1. the original Go code;
2. the obligation gohawk discovers;
3. the evidence it follows through SSA;
4. the fact or summary it records; and
5. the point at which it can safely report—or must conservatively stop.

Prefer a minimized version of a real Docker or Caddy finding so the abstraction
ends in a concrete result. Do not use the transaction example unless gohawk
actually implements that contract.
-->

This became the core design problem in gohawk: how do you turn the mechanics exposed by SSA into a small, trustworthy vocabulary for reasoning about resource and concurrency bugs?

## Fact propagation

Inferring a fact inside one function is useful, but many interesting obligations cross a function boundary. A helper may acquire a resource on behalf of its caller. A goroutine may signal completion through a channel captured by a closure. Cleanup may be delegated to another function or exported as part of a package's behavior.

<!--
Walk one fact through its complete journey:

- where it is inferred;
- what information is retained in the summary;
- how it crosses a call or package boundary;
- how a caller combines it with local SSA; and
- which uncertainty causes gohawk to stop rather than guess.

Emphasize that precision comes from propagating the minimum evidence required
for a proof, not from constructing a perfect model of the whole program.
-->

This is also where the engineering tradeoff becomes visible. A more aggressive analyzer can infer more, but every unsupported assumption risks turning a useful signal into a false alert. gohawk deliberately treats uncertainty as a reason to withhold a diagnostic.

## Conclusion

Go's analysis APIs made gohawk possible, but they did not make it automatic. SSA provided the map: values, instructions, and paths through a program. The harder work was deciding what those pieces meant for ownership, resource lifetimes, and concurrency—and encoding only the conclusions the analyzer could defend.

That journey changed how I think about static analysis. The deepest checks do not come from searching for increasingly elaborate syntax patterns. They come from building a small semantic model, propagating its facts carefully, and knowing when the available evidence is not enough.

<!--
Close with links to:

- the gohawk documentation or reasoning-model page;
- one or two analyzers discussed in the post;
- the merged Docker and Caddy fixes, if used above; and
- the repository for readers who want to explore or contribute.

Optionally add one forward-looking sentence about a later post that will unpack
an individual analyzer or the fact model in more technical detail.
-->
