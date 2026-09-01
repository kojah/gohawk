---
description: Use before writing or reviewing Go production code, tests, fixtures, or tools where maintainability, API shape, errors, concurrency, or testability matters.
metadata:
    github-path: skills/go-coding-conventions
    github-pinned: 906d407570467b2a27c6cafa25cfc19193f66845
    github-ref: 906d407570467b2a27c6cafa25cfc19193f66845
    github-repo: https://github.com/kojah/skill-vault
    github-tree-sha: 5021401305effbf5467044ee913238f724266307
name: go-coding-conventions
---

# Go coding conventions

Write clear, testable Go with explicit dependencies and predictable failure
behavior. Repository instructions, supported Go version, public compatibility,
and established local conventions override these defaults; surface conflicts
rather than silently restyling code.

**MUST** / **MUST NOT** mark defects. **SHOULD** / **SHOULD NOT** are defaults;
deviate with a stated reason. Prefer clarity over cleverness.

## TL;DR

1. Run `gofmt`/`goimports` on changed files plus repository vet, lint, and
   race-enabled test gates.
2. Use MixedCaps and consistent acronym case; give domain states, enums, and
   bitsets defined types instead of passing raw primitives.
3. Keep packages cohesive; reject both grab bags and package-per-type
   fragmentation.
4. Return early; keep happy path at left margin.
5. Handle each error once; wrap with operation context and `%w`; inspect with
   `errors.Is` or `errors.As`.
6. Never panic for expected failures.
7. Accept narrow interfaces, return concrete types, and define interfaces at
   consumers only when needed.
8. Put `ctx context.Context` first; never store it or pass `nil`.
9. Start no goroutine without an exit condition and join owner.
10. Avoid mutable global state; inject replaceable dependencies, including
    clocks.
11. Prefer table-driven behavioral tests, small fakes, stdlib `testing`, and
    `go-cmp` when repository policy permits it.
12. Document exported contracts and non-obvious rationale; follow
    [Documentation](#documentation).
13. Honor the repository's supported Go floor. Prefer modern syntax and stdlib
    APIs within that floor; never require a newer version accidentally.

## Documentation

- Non-trivial packages need package documentation covering purpose, main entry
  points, boundaries, important design choices, and compatibility or
  performance constraints a reader would otherwise reconstruct.
- Document every exported identifier from its name. Explain its contract, not
  its signature: relevant invariants, ownership, errors, side effects,
  concurrency guarantees, and performance characteristics.
- Add runnable Go examples when correct use, ordering, lifecycle, or composition
  is not obvious. Prefer realistic examples that compile as tests.
- Add an inline comment when code cannot state its reason: an obvious
  alternative is wrong, an external rule constrains the choice, a bound is
  derived, a platform or library behaves surprisingly, or an omission is
  deliberate. State what breaks without the choice.
- For concurrent code, name goroutine ownership, cancellation and join paths,
  channel-closing authority, and required ordering where code does not make
  them evident.
- Cite the owning specification instead of duplicating a normative rule. Keep
  item comments short; put system-level reasoning in package documentation.
- Long rationale is correct when the constraint is subtle. Delete comments
  that merely narrate the next operation, such as “ReadFile reads a file.”

## Rule routing

Read only references needed by current work. Section numbers remain stable for
external citations.

- Naming or package topology: [§1–§2](references/01-naming-and-packages.md).
- Function shape or errors: [§3–§4](references/03-code-shape-and-errors.md).
- Interfaces, structs, constructors, receivers, or file organization:
  [§5–§6](references/05-interfaces-and-structs.md).
- Goroutines, channels, contexts, or synchronization:
  [§7](references/07-concurrency.md).
- Unit tests or testability: [§8](references/08-testing.md).
- Hygiene or architecture-changing refactors:
  [§9–§10](references/09-hygiene-and-refactorability.md).
- Language-version choices, modern stdlib or testing APIs, tool dependencies,
  structured logging, or suspected toolchain faults:
  [§11](references/11-modern-go.md).

## Test scope

While iterating, judge test scope from the reach of each change. Keep to the
changed package for a body-local or unexported
change. Widen to importing packages when an exported identifier, interface, or
struct field changed. Run the full suite for goroutine, channel, `init`, or
package-level state changes, or a change spanning several packages. When reach
is genuinely unclear, choose the narrower scope: the completion gate still runs
everything, so a miss costs one late failure rather than a suite on every edit.

## Verify

While iterating narrowly, run the changed packages with
`go test ./internal/foo/...`. Add importing packages when an exported
identifier changed; `go list -deps ./...` identifies them.

Run the race-enabled full suite once before reporting the work complete, with
the repository's canonical formatter, vet, and linter. Reuse current passing
receipts until an affecting source or build change lands. When a parent
workflow owns final verification, contribute a deduplicated command set and
report unrun gates or intentional deviations.

Selection cannot see effects that cross the graph invisibly. Go straight to the
full suite when a dependency, `go.mod`, build tag, or generated file changed.
