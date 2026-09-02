---
title: The fact model
description: What cross-package lifecycle facts can and cannot express, and the polarity each mask must keep.
sidebar:
  order: 2
---

`internal/passes/lifecyclefacts` lets an analyzer see through a call into
another package without re-analyzing the callee. It exports, for each exported
function, a compact summary of what the function does to its parameters. This
page is the contract for reading and extending that summary.

## The shape of a fact

A fact is a per-function summary of **unary predicates over parameter
positions**, each **proven on every normal return path**, computed
**context-insensitively**, and attached to a **static callee**. Every
capability and every limit below follows from one of those clauses.

The declaration below is regenerated from
`internal/passes/lifecyclefacts/fact.go` by `go generate ./...`, so the mask
list is always the one the code has; do not edit it by hand.

<!-- gohawk:generated-fact-fields:start -->
```go
// Fact is the compact cross-package ownership summary exported for a
// function. Each bit identifies an SSA parameter position. This package is
// internal analysis infrastructure, not a public extension API.
type Fact struct {
	Invoked		ParameterMask
	Closed		ParameterMask
	Finalized	ParameterMask
	Released	ParameterMask
	Shutdown	ParameterMask
	Stopped		ParameterMask
	Waited		ParameterMask
	Committed	ParameterMask
	RolledBack	ParameterMask
	ReturnedOwner	ParameterMask
	// ReturnedView narrows ReturnedOwner: the parameter is stored in the
	// returned struct, but no method of that type releases the field, so the
	// caller keeps the obligation. See fields.go.
	ReturnedView	ParameterMask
	// Retained marks parameters the callee may keep beyond the call; see
	// retention.go for the over-approximation it deliberately makes.
	Retained	ParameterMask
	// Stored is the strict form of Retained: positive structural evidence that
	// the callee keeps the parameter, safe to treat as an ownership transfer.
	Stored	ParameterMask
	// OwnedFields and ReleasedFields are indexed by struct field, not
	// parameter; see fields.go for the constructor and method summaries.
	OwnedFields	ParameterMask
	ReleasedFields	ParameterMask
	ReceiverStore	ParameterMask
}
```
<!-- gohawk:generated-fact-fields:end -->

A `ParameterMask` is a bitset over the first 64 parameter positions; the
receiver is position 0. `OwnedFields` and `ReleasedFields` are indexed by
struct field instead and describe constructors and methods of a type.

## Three answers to "what happened to my value?"

| mask | quantification | meaning for the caller |
|---|---|---|
| a discharge verb (`Closed`, `Waited`, …) | must | the obligation is finished — a join |
| `ReturnedOwner` | must | the obligation moved onto the result; keep tracking it |
| `ReturnedView` | must | the result is a window onto the parameter; the caller still owns it |
| `Stored` | must | positive evidence the callee keeps it; safe to treat as a transfer |
| `Retained` | may | the callee might keep it somewhere; widen to `unknown` |

`ReturnedOwner` and `ReturnedView` look identical in SSA — both store the
parameter in a returned struct. They differ only in whether a method of the
returned type releases that field, which is what `OwnedFields` and
`ReleasedFields` record. That is why the view fact exists alongside the owner
fact rather than being inferred from it.

## Polarity is not optional

Each mask's quantification matches how it is consumed, and that is what keeps
the model sound:

- A mask used to **continue a proof** — `ReturnedOwner`, `Stored`, every
  discharge verb — must be exact. A may-version would let a downstream proof
  draw a false conclusion.
- A mask used only to **suppress** — `Retained` — may over-approximate. An
  extra bit can hide a diagnostic but can never create one. Consumers use it
  to widen `unknown`, never to prove ownership.

When you add a mask, decide its polarity first and document which direction
consumers may use it in.

## Three-valued consumption

Consumers never read a fact as a Boolean. `LifecycleEvidence.Prove` returns
one of three states:

- **proven** — the function was summarized and the bit is set.
- **disproven** — the function was summarized and the bit is clear.
- **unknown** — no fact exists (not exported, too many parameters, not
  analyzed, or reached through dynamic dispatch).

Absence is never disproof. A callee with no fact is opaque, and an opaque
consumption suppresses the diagnostic. Any tool that surfaces facts —
including `gohawk facts` — must keep the summarized-versus-absent distinction
visible for the same reason.

## What the model can express

- A lifecycle action guaranteed on every normal return of the callee.
- Ownership transfer to the result or to an escaping receiver, and its
  opposite, a returned view.
- Invocation of a func parameter (`Invoked`).
- Transitive discharge through exported callees, because summaries consult
  imported facts.
- Marker facts on other object kinds: `closedomain` attaches a fact to a
  struct field, so the mechanism is not limited to functions.
- A new discharge verb, cheaply: one row in the mask table in `fact.go`
  (name, method, field).

## What it cannot express, and which clause forbids it

- **Relations between values** ("param A flows to result B", "which lock does
  this unlock") — *unary*. This is the ceiling; relational questions belong to
  a different tool, not to more bits.
- **Conditional or partial discharge** ("closes only on the error path") —
  *every path*. It collapses to bit-clear, indistinguishable from never.
- **Per-call specialization** — *context-insensitive*. One fact per function.
- **Panic and abnormal exits** — *normal return*. A callee that closes on
  every `return` but leaks through `panic` is still `Closed`.
- **Interface dispatch** — *static callee*. A call through an interface has no
  fact and is `unknown`, unless the method name matches a documented cleanup
  contract such as `Close`.
- **Unexported functions and more than 64 parameters** — *exported*. Package
  internals are the local proof's job.
- **Counts and ordering** ("adds two", "closes after waiting") — *bits*.

Most of these are the price of a bounded, cacheable, precision-first summary
and are chosen deliberately. Extend the model only for a reusable structural
contract with exact value provenance and feasible-path semantics; a single
dogfood false positive is not sufficient reason.

## Where facts live

Facts are imported and exported (`analysis.Pass.ImportObjectFact` and
`analysis.Pass.ExportObjectFact`) only inside the package that defines the fact
type. Consumers use `lifecyclefacts.LifecycleEvidence`, which consults local
evidence first and imported facts second along one decision path. A second
fact family with a different evidence model — `closedomain`'s field marker —
gets its own package for the same reason: one vocabulary per family.
`TestObjectFactsStayInTheirDefiningPackage` enforces the boundary.
