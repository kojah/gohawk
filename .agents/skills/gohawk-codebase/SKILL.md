---
description: Use when orienting in the gohawk codebase, deciding where new code belongs, or checking whether ssaflow or lifecyclefacts already has a helper before writing SSA traversal, provenance, or ownership code.
metadata:
    source: project
name: gohawk-codebase
---

# gohawk codebase orientation

gohawk is a suite of precision-first Go analyzers. Most work touches one
analyzer package, but every analyzer stands on a shared engine, and the most
expensive mistakes in this repository come from not knowing what that engine
already provides. Read this before writing any traversal or ownership code.

## Read the map first

- [Codebase layout](../../../docs/architecture.md): the layers, how
  a run works, and the architectural invariants that tests enforce.
- [Shared helpers](references/shared-helpers.md): the `ssaflow` and
  `lifecyclefacts` helpers indexed by the question each answers, followed by
  the generated index of every exported helper for searching by name.
- [The fact model](../../../docs/development/fact-model.md): what cross-package
  lifecycle facts can and cannot express.

## Where does this code belong?

Walk the decision in order and stop at the first fit.

1. **Analyzer-local.** Precision policy — what counts as a join, a transfer,
   an obligation — always stays beside the analyzer that owns it, even when the
   implementation looks reusable.
2. **`internal/ssaflow`** for SSA mechanics a second analyzer needs: value
   provenance, control flow, storage and escape checks, symbol matching. Share
   *how to walk*, never *whether evidence is sufficient*.
3. **`internal/syntax`** for source-level helpers and well-known symbol
   identity.
4. **`internal/passes`** for a prerequisite `analysis.Analyzer` that several
   analyzers require, such as `lifecyclefacts`.
5. **`internal/check` and `internal/trace`** for cross-cutting reporting and
   evidence tracing.

Promote a helper out of an analyzer only after a second analyzer needs the same
general mechanic. One consumer is not a reason to share.

## Before writing traversal code

Check the [shared helpers](references/shared-helpers.md) table
first. In particular:

- Walking what flows into a value: `ssaflow.NewReachingWalk` with Any, Every,
  or `ResolveReachingValue`. Never fan out over phi edges or thread a visited
  set yourself; `TestAnalyzersUseSharedTraversal` rejects it.
- Path-sensitive state over blocks: `ssaflow.WalkStates`.
- Does an action cover every return: the `ssaflow.UnownedReturn*` family.
- Peeling wrappers: `ssaflow.UnwrapTransparentValue` with an explicit form set.
  There is deliberately no universal unwrap helper.
- Collecting instructions of one type: `ssaflow.InstructionsOf`.
- Closure captures: `ssaflow.ClosureBindingPairs`.

New shared traversal is born higher-order (it takes the leaf predicate) and
form-selective (it takes the transparent forms), so the next analyzer can
reuse it instead of copying it.

## Invariants the tests enforce

`internal/architecture` fails the build when these break: dependency
direction, one package per analyzer, diagnostics only through `check.Report`,
symbol identity through `syntax.Symbol`, no process termination in library
code, rationale comment density, object facts imported and exported only in
the package that defines them, and value-provenance traversal only through
`ssaflow`. See the invariants section of the architecture guide for what each
one protects.
