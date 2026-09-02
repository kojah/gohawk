---
title: Inferred facts
description: What cross-package lifecycle facts can and cannot express, and the polarity each mask must keep.
sidebar:
  order: 3
---

`internal/passes/lifecyclefacts` lets an analyzer see through a call into
another package without re-analyzing the callee. For each exported function it
records a small summary of what that function does to its parameters. This page
explains how to read that summary and how to extend it.

## The shape of a fact

A fact summarizes what a function does to each of its parameters. In the model
as it stands today, four things hold for every fact, and everything below —
what a fact can say and what it cannot — follows from them:

- It describes **one parameter at a time**, never how two values relate.
- It records only what happens on **every normal return**; a maybe does not
  count.
- It is computed **once per function**, not once per call site.
- It is attached to a callee the analyzer can **name directly**, not one
  reached through an interface.

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

| mask | guarantee | what it means for the caller |
|---|---|---|
| a discharge verb (`Closed`, `Waited`, …) | always | the obligation is finished — a join |
| `ReturnedOwner` | always | the obligation moved onto the result; keep tracking it |
| `ReturnedView` | always | the result is a window onto the parameter; the caller still owns it |
| `Stored` | always | firm evidence the callee keeps it; safe to treat as a transfer |
| `Retained` | maybe | the callee might keep it somewhere; fall back to `unknown` |

`ReturnedOwner` and `ReturnedView` look identical in SSA — both store the
parameter in a returned struct. They differ only in whether a method of the
returned type releases that field, which is what `OwnedFields` and
`ReleasedFields` record. That is why the view fact exists alongside the owner
fact rather than being inferred from it.

## Some masks must be exact; others may guess

How sure a mask has to be depends on what reads it, and keeping that straight
is what keeps the model sound:

- A mask that **carries a proof forward** — `ReturnedOwner`, `Stored`, and
  every discharge verb — has to be exact. If it were a guess, a later step
  could build a wrong conclusion on top of it.
- A mask that can only **hide a diagnostic** — `Retained` — is allowed to
  guess on the cautious side. An extra bit can suppress a warning but can never
  invent one. Consumers use it to fall back to `unknown`, never to prove
  ownership.

When you add a mask, decide which of these it is first, and write down which
way consumers are allowed to lean on it.

## Proven, disproven, or unknown

A consumer never reads a fact as a plain yes/no. `LifecycleEvidence.Prove`
returns one of three answers:

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
- Cleanup that happens deeper in a chain of exported calls, because one
  summary is allowed to read the summaries of the functions it calls.
- Facts on things other than function parameters: `closedomain` attaches a
  fact to a struct field, so the mechanism is not limited to functions.
- A new discharge verb, cheaply: one row in the mask table in `fact.go`
  (name, method, field).

## What it can't express, and why

Each limit traces straight back to one of the four things above.

- **Relationships between values** ("param A flows to result B", "which lock
  this unlocks") — *one parameter at a time*. Questions about how two values
  relate belong to a different tool today, not to more bits.
- **Conditional or partial cleanup** ("closes only on the error path") —
  *every return*. It collapses to a clear bit, which looks the same as never.
- **Per-call answers** — *once per function*. There is one fact, however
  differently two call sites use the function.
- **Panics and other abnormal exits** — *normal returns only*. A callee that
  closes on every `return` but leaks when it panics still counts as `Closed`.
- **Interface calls** — *named callee only*. A call through an interface has no
  fact and is `unknown`, unless the method name matches a documented cleanup
  contract such as `Close`.
- **Unexported functions, and more than 64 parameters** — a fact is only
  exported for exported functions; package internals are the local proof's job.
- **Counts and ordering** ("adds two", "closes after waiting") — a fact is
  *just bits*.

Most of these limits are the cost of keeping the summary small, cacheable, and
precise; they are deliberate choices in the current model rather than rules
fixed for all time. Only extend the model for a real, reusable pattern whose
values and paths the analyzer can pin down exactly — a single false positive
found while dogfooding is not reason enough.

## Where facts live

Facts are imported and exported (`analysis.Pass.ImportObjectFact` and
`analysis.Pass.ExportObjectFact`) only inside the package that defines the fact
type. Consumers use `lifecyclefacts.LifecycleEvidence`, which checks local evidence
first and imported facts second, all on one path. A second fact family with a
different evidence model — `closedomain`'s field marker — gets its own package
for the same reason: one vocabulary per family.
`TestObjectFactsStayInTheirDefiningPackage` enforces the boundary.
