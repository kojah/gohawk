---
description: Use when writing or reviewing TypeScript production code, tests, libraries, scripts, or configuration where type safety, API shape, modules, async behavior, runtime validation, or testability requires judgment.
metadata:
    github-path: skills/typescript-coding-conventions
    github-pinned: 8253c776d8e6e467655e04d53aa2076861aade6f
    github-ref: 8253c776d8e6e467655e04d53aa2076861aade6f
    github-repo: https://github.com/kojah/skill-vault
    github-tree-sha: ad0322d8ff8e1e61921ae3236a20828531abee42
name: typescript-coding-conventions
---

# TypeScript Coding Conventions

Write TypeScript whose runtime behavior matches its static model. Keep APIs
small, validate external data, make asynchronous ownership explicit, and add
abstractions only for current variation.

## Authority and orientation

Apply explicit requirements and approved contracts first, then repository
instructions, supported runtimes, `tsconfig` files, package metadata, lint and
format rules, framework constraints, and nearby patterns. Surface material
risks instead of silently overriding repository policy.

Before editing, inspect the nearest `package.json`, lockfile, `tsconfig` chain,
module format, public export map, tests, and configured formatter, linter,
type-checker, and build commands. Use the repository's package manager and
tools. Do not add or replace tooling merely to satisfy this skill.

Keep changes within the touched behavior and necessary seams. Avoid unrelated
restyling, speculative migrations, dependency churn, or repository-wide type
tightening without authorization.

## Compiler version

Use TypeScript 7 for new projects and migrations requesting the preferred
stack. Verify the executed compiler version; a manifest entry alone is not
proof. A tool may retain TypeScript 6 only as a documented compatibility
dependency while project checking runs on 7. For current Svelte tooling, use
the TypeScript 7 native compiler through `svelte-check --tsgo`; keep its required
TypeScript 6 package until `svelte-check` supports 7 directly.

## Model valid states

- Keep strict checking enabled where the repository supports it. New isolated
  packages should use `strict`; changing an existing package's compiler policy
  is a separately scoped migration.
- Use `unknown` for untrusted or erased values. Narrow it before use. Contain
  unavoidable `any` at one boundary and explain why safer typing is impossible.
- Model closed state with discriminated unions and exhaustive checks. Do not
  combine independent booleans that permit contradictory states.
- Distinguish absent, `undefined`, and `null` only when behavior differs. Avoid
  broad `Partial<T>` objects as long-lived domain state.
- Preserve meaningful relationships with generics. Do not add a generic,
  conditional type, branded type, overload, or utility layer for one concrete
  use or hypothetical reuse.
- Prefer unions and object maps over runtime enums unless enum runtime identity
  or compatibility is required.
- Use assertions only beside evidence the compiler cannot express. Never use
  `as unknown as T` or non-null assertions to suppress an unresolved state.
- Use `satisfies` when checking a value while preserving its inferred shape;
  use annotations when callers need the declared abstraction.

## Validate runtime boundaries

Types disappear at runtime. Treat network responses, JSON, storage, environment
variables, command arguments, message events, DOM data, and third-party output
as untrusted. Parse and validate once at the owning boundary, then convert to a
stable internal representation. Validation failure must identify safe,
actionable context without exposing secrets.

Do not claim runtime safety from a cast, generated client, or declared return
type alone. Add a validation dependency only when repository policy permits it
and repeated boundary complexity justifies it.

## Shape modules and APIs

- Give each module and function one coherent responsibility. Separate domain
  decisions from network, filesystem, process, clock, randomness, UI, and
  framework adapters.
- Keep exports intentional. Avoid grab-bag modules, mutable exported objects,
  wildcard barrel files that create cycles, and import-time side effects.
- Prefer plain functions and data. Add classes for owned lifecycle or stable
  identity; add interfaces for real consumer boundaries or multiple current
  implementations.
- Pass replaceable external collaborators explicitly. Avoid service locators,
  hidden singletons, ambient mutable state, and mocks that patch module globals.
- Preserve public compatibility deliberately. Exported types, error shapes,
  serialization, module paths, and package export maps are contracts.
- Use domain names and straightforward control flow. Prefer early returns and
  named intermediate values over dense expressions or type-level cleverness.

## Own async work and resources

- Every promise must be awaited, returned, or deliberately tracked with an
  explicit rejection path. Never discard async failure accidentally.
- Accept or create `AbortSignal` at cancellable boundaries and propagate it to
  owned work. Cancellation does not replace cleanup.
- Remove listeners, clear timers, close handles, and unsubscribe at the same
  lifecycle boundary that acquired them. Cleanup must cover success, failure,
  cancellation, and partial initialization.
- Avoid `forEach(async ...)`. Choose sequential iteration, bounded concurrency,
  `Promise.all`, or `Promise.allSettled` according to ordering and failure
  semantics. Preserve deterministic output when concurrency is added.
- Bound retries, queues, payloads, fan-out, and external waits. Do not add
  concurrency or caching without measured need and explicit ownership.

## Handle errors deliberately

Reject invalid state at the earliest owning boundary. Catch only when adding
context, translating to a stable error contract, recovering, or cleaning up.
Caught values are `unknown`; normalize them before reading properties. Preserve
causes where supported. Never expose credentials, swallow failure, or return a
successful-looking value after incomplete work.

Use exceptions, result objects, or tagged outcomes according to the existing
public contract. Do not introduce a second error model casually.

## Test behavior

- Test through the narrowest stable public boundary. Cover success, invalid
  input, boundaries, asynchronous rejection, cancellation, cleanup, and
  deterministic ordering where applicable.
- Prefer real values and small hand-written fakes at nondeterministic seams.
  Mocking convenience alone does not justify a new interface.
- Use type-level tests when public generics, overloads, narrowing, or expected
  compile failures are part of the contract. Runtime tests cannot prove them.
- Avoid snapshots for behavior better expressed with focused assertions.
- These rules also govern test helpers and fixtures. A parent testing workflow
  owns proof scope and coverage requirements on conflict.

## Test scope

While iterating, judge test scope from the reach of each change. Keep to the
changed module for a body-local or non-exported
change. Widen to importing modules when an exported signature, type, or public
generic changed. Run the full suite for module side effects, shared singletons,
or a change spanning several packages in a workspace. When reach is genuinely
unclear, choose the narrower scope: the completion gate still runs everything,
so a miss costs one late failure rather than a suite on every edit.

## Verify

While iterating narrowly, select by changed file:
`vitest related <files>` or `jest --findRelatedTests <files>`. Type-checking
stays project-wide, since a changed signature breaks callers the test selector
does not run.

Run the full suite once before reporting the work complete, with
repository-configured formatting, linting, type-checking, and build/package
checks. Verify every relevant runtime target and module format. Report unrun
checks, unsafe assertions, contained `any`, new dependencies, and intentional
convention deviations.

Selection cannot see effects that cross the graph invisibly. Go straight to the
full suite when a dependency, lockfile, `tsconfig`, bundler configuration, or
generated type changed.
