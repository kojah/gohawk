# Techniques and related work

gohawk is a compositional, summary-based typestate analysis over SSA, backed by
per-function points-to and escape summaries and footprint preconditions. It is
tuned to report only what it can prove, not to find every possible violation.
This page names each technique, points at where it lives in the code, and says
where gohawk departs from the work it borrows from.

## Compositional summaries

Each function is analyzed once, callees before callers, into a summary that a
caller applies instead of reanalyzing the body. This is the functional approach
to interprocedural analysis (Sharir and Pnueli, "Two approaches to
interprocedural data flow analysis", 1981), and the idea Meta's Infer is built
around.

- Summaries are computed by the prerequisite passes in `internal/passes`:
  `lifecyclefacts` for ownership and cleanup, `resultfacts` for what results
  guarantee, `concurrencyfacts` for ordered synchronization effects.
- They cross package boundaries as go/analysis object facts, so a caller in one
  package reads the summary of a callee in another without its body. See the
  [fact model](fact-model.md).
- A summary is one postcondition joined over every path to a normal return.
  There are no per-path disjuncts: a claim is exported only when it holds on
  every such path, and anything weaker is unknown to the caller.

## Pre- and postconditions

A summary is a contract: what the function needs from what it is handed (its
precondition) and what it has done to it by the time it returns (its
postcondition).

- Postconditions are the heap projection below, the discharges (which cleanup
  method is called on which parameter, at which path beneath it), and the
  result guarantees.
- Preconditions follow Pulse, Infer's current engine, rather than the
  bi-abduction engine before it. Bi-abduction guesses the missing facts that
  would make each heap access safe and solves for the untouched frame at every
  call. Pulse, and gohawk, record the footprint a function actually read and
  the facts that held about it on every path. Applying a summary matches the
  caller's state against that footprint; a clear contradiction is a finding,
  and anything else is unknown. See [preconditions](preconditions.md).

References: the [Infer documentation](https://fbinfer.com/), its
[bi-abduction](https://fbinfer.com/docs/separation-logic-and-bi-abduction/) and
[Pulse](https://fbinfer.com/docs/checker-pulse) pages.

## Heap model: points-to and escape summaries

Each function gets a flow-sensitive points-to graph over its SSA, with may and
must edges, in `internal/heapmodel` (the region graph). Its projection onto
what a caller can name, meaning parameters, results, and globals, becomes the
exported `Fact.Heap`: where each slot may point on return, how each object
escaped, what was released, and where the projection was cut. A caller's graph
applies a callee's projection by substitution.

The closest published relative is Whaley and Rinard, "Compositional pointer
and escape analysis for Java programs" (OOPSLA 1999), which builds per-method
points-to and escape graphs and merges them at call sites. gohawk's graph is
narrower: it answers identity, containment, and storage questions for the
ownership checks, not general alias queries. See the
[points-to model](points-to-model.md) and [storage model](storage-model.md).

The transfer and retention claims a consumer reads, such as `ReturnedOwner`,
`Stored`, and `Retained`, are queries over this projection rather than
separately computed facts.

## Typestate over SSA

The analyzers are typestate checks (Strom and Yemini, "Typestate: a
programming language concept for enhancing software reliability", 1986). A
file, lock, goroutine, cancel function, or process moves between states such
as acquired, released, handed over, and unknown, and every path to a return
must leave it in an allowed state.

The shape every lifecycle analyzer shares is a path-sensitive dataflow problem
over SSA (`golang.org/x/tools/go/ssa`):

1. An obligation finder ties what must happen to exact SSA values.
2. A classifier labels each later instruction as join, transfer, unknown, or
   none.
3. One flow query, `ssaflow.EvaluateObligation`, decides the outcome: honored
   when exact actions cover every return, unknown when only opaque ones do,
   violated otherwise.

Paths are merged as in abstract interpretation, with unknown as the safe top
element. Branch guards are remembered per path (`ssaflow.PathGuards`), so a
later test of the same condition is related to the earlier one.

## Where gohawk departs from these

- **Precision over recall.** Infer and Pulse aim to find bugs with few false
  positives; gohawk goes further and treats any opaque handoff as unknown
  rather than guessing, accepting missed bugs to keep every report actionable.
- **No separation logic.** gohawk does not reason about heap shape with
  separation-logic entailment. Its questions are about ownership, and a
  may/must points-to graph answers them.
- **Bounded work.** Every interprocedural question spends a search budget, and
  running out of budget is unknown, never a finding.
