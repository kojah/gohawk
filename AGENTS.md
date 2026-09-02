# gohawk development guidance

Value high precision over high recall. Avoiding false alerts is more important
than detecting every possible violation: users must be able to trust that a
gohawk diagnostic is actionable.

- Report only when the analyzer has strong evidence that the policy is
  violated on a feasible path. When ownership or lifecycle behavior is
  uncertain, prefer no diagnostic.
- Model common cleanup, ownership-transfer, registration, and lifecycle
  patterns before expanding an analyzer's coverage.
- Do not use suppression comments as a substitute for fixing a recurring
  false-positive pattern.
- Give every analyzer change focused regression fixtures for both the
  diagnostic and accepted forms. Minimize patterns found through dogfooding
  rather than copying external repositories into the test suite.
- Dogfood changes on representative real-world repositories and investigate
  newly introduced findings before enabling broader coverage.
- Avoid project-name or function-name exemptions unless they represent a
  documented, general API contract.
- Prefer general evidence models over accumulating special cases. Before
  adding an exception or API-specific rule, identify the broader contract it
  represents and implement that contract at one reusable decision point. If a
  pattern cannot be generalized or explained clearly, conservatively decline
  to analyze it.

## Development commands

Prefer existing Makefile targets for repository-wide validation so local and
CI workflows use the same commands. Run `make help` to discover available
targets. Direct commands remain appropriate for focused testing and debugging.

## Development skills and references

Task procedures live as project-local skills under `.agents/skills/`, and the
knowledge they depend on lives in `docs/`. Load the skill that matches the
task rather than re-deriving the procedure:

- `.agents/skills/gohawk-codebase/SKILL.md` — orientation, where new code
  belongs, and whether a shared helper already exists.
- `.agents/skills/gohawk-analyzer-change/SKILL.md` — changing an analyzer,
  fixing a false positive or negative, responding to a failed precision label.
- `.agents/skills/gohawk-precision-audit/SKILL.md` — running a precision
  round, labelling findings, recording a batch audit.
- `.agents/skills/gohawk-debugging/SKILL.md` — reading SSA dumps, fact dumps,
  and evidence traces to explain a diagnostic.

References: `docs/architecture.md` (layers and enforced invariants),
`docs/development/shared-helpers.md` (every shared helper by the question it
answers), `docs/development/fact-model.md`, and
`docs/development/debugging-reference.md`. This file states policy; those
files hold the procedures and inventories, so keep procedural detail there.

## Shared working tree

Several agent sessions commit from this checkout at once. Do not switch
branches in it, run `git add -A`, or revert a file you did not change. Read
`git status` before staging, and stage and commit only your own files.

## Code clarity

Make the code instructive and easy to follow. Prefer simple, clear English,
especially in comments.

Keep Go source within the repository's 160-column lint limit. When a condition
combines distinct kinds of evidence, extract named predicates that expose the
policy structure instead of merely wrapping a long Boolean expression.

## File cohesion and size

Organize a file around one vocabulary and one reason to change. Split a file
when it acquires a second evidence model, lifecycle, or external boundary; do
not split a cohesive implementation merely to satisfy a line-count target.
Name extracted files for their concern, such as `contracts.go`, `flow.go`, or
`persistence.go`, rather than creating `helpers.go`, `types.go`, or a package
per type.

Treat 400 lines for production source, 700 lines for tests, 60 lines for a
function, and cognitive complexity above 25 as review triggers rather than
hard limits. When a change crosses one of these thresholds or materially grows
an existing outlier, either extract a focused responsibility or document why
the code remains cohesive. Split large test files by behavior and keep shared
fixture construction separate from the scenarios it supports.

If automated size or complexity checks are added, baseline existing outliers
and fail only new regressions. Existing debt must not block unrelated changes,
and generated files, lockfiles, fixtures, and other intentionally mechanical
content should be excluded or evaluated under separate thresholds.

## Process termination

Production analyzer and reusable library code must not call `panic()`,
`log.Fatal()`, or `os.Exit()`. Return errors to the caller and let the command
entry point decide how to present failures and choose an exit status. Test
fixtures may use these operations when they are the behavior being analyzed.

## Analyzer rationale comments

Add comments at non-obvious precision boundaries: ownership transfers,
feasible-path assumptions, conservative bailouts, and distinctions between
default and opt-in diagnostics. Explain why the analyzer accepts or rejects a
pattern and what evidence makes that decision safe; do not merely restate the
code.

When dogfooding reveals a representative real-world pattern, include a
commit-pinned source link in the nearby rationale comment. Keep the minimized
fixture as the executable regression test. Prefer one durable comment at the
decision point over repeating the explanation throughout helper functions.

Preserve these comments when refactoring. If behavior or its supporting
evidence changes, update the rationale, link, and regression fixture together.

## File responsibility comments

Add a short responsibility comment near the top of a non-obvious implementation
file when its role, evidence boundary, or relationship to the rest of the
package would otherwise require reading several functions to reconstruct.

- Describe the concern the file owns, the evidence it consumes or produces,
  and the conservative boundary where analysis stops.
- Prefer stable invariants and reasoning over inventories of functions or
  references to neighboring filenames.
- Do not add a header when the package declaration, filename, and first types
  already make the responsibility clear.
- Keep the comment with the implementation when code moves, and update it in
  the same change when the responsibility or precision boundary changes.

## Analyzer tracing

Use the structured evidence tracer instead of temporary print statements when
investigating analyzer behavior. Use `gohawk ssa -func NAME PACKAGE` to read
the SSA the analyzers see instead of reconstructing the lowering by hand, and
`gohawk facts PACKAGE` to read the lifecycle summaries a package exports and
imports; the evidence records carry the SSA text of the instruction they
judged. Every diagnostic must flow through
`check.Report` or `check.Reportf`, which provide repo-wide
candidate and suggested-fix events. Shared analyzer wrappers trace whether a
candidate is reported, suppressed by an ignore comment, or removed by check
selection.

Instrument non-obvious evidence and conservative bailout decisions near the
policy code that makes them. Use the common phases consistently:

- `candidate` for a potentially reportable construct;
- `evidence` for facts that support accepting or rejecting it;
- `decision` for the final report or suppression outcome; and
- `fix` for the availability or rejection of a suggested edit.

Reason codes are a diagnostic interface: use stable, concise kebab-case names
that describe why the decision was made. Keep details compact and avoid dumping
AST or SSA values. Call `trace.Enabled` before allocating maps or computing
expensive trace-only metadata so disabled tracing remains effectively free.

Analyzer changes that add a new precision boundary should trace the decisive
reason and test it when practical. Trace output must remain valid JSONL under
parallel analysis, and enabling tracing must not change which diagnostics are
reported.

## Proof model cohesion

Keep one authoritative decision path for each analyzer check.

- Do not maintain parallel Boolean lists that independently decide whether the
  same diagnostic is accepted or rejected. Exported helpers, tracing paths, and
  reporting paths must delegate to the same proof function, even when a caller
  only needs the final Boolean result.
- Represent non-trivial precision decisions with a structured proof containing
  at least the outcome and stable reason code. Reserve bare `bool` helpers for
  small mechanical facts whose explanation is supplied by their caller.
- Before adding another predicate to a top-level condition, identify the
  evidence family it belongs to and extend that family's proof model. If the
  predicate introduces different vocabulary or invariants, extract a focused
  evidence file instead of growing the orchestration function.
- Keep analyzer entry points limited to collecting inputs, requesting proofs,
  tracing the returned reason, and reporting. They must not duplicate the
  evidence rules implemented by lower-level helpers.

Keep default proof systems deliberately bounded:

- A default diagnostic requires positive structural evidence of both an
  obligation and its violation. The absence of a recognized cleanup, join, or
  owner is not itself proof of a defect.
- Treat opaque callbacks, registries, receiver storage, framework handoffs,
  and other ambiguous ownership as `unknown`, and suppress the default
  diagnostic. Do not accumulate project, function-name, or lifecycle-name
  exceptions to guess beyond the proven boundary.
- Add a proof family only for a reusable structural contract with exact value
  provenance and feasible-path semantics. A dogfood false positive alone is
  not sufficient reason to expand the model.
- Keep heuristic audits explicitly opt-in. Do not make a heuristic
  increasingly elaborate in an attempt to support a default correctness
  claim.

Prefer stable false negatives over an analyzer whose precision depends on an
open-ended catalog of framework and naming conventions.

### Lifecycle analyzers: classify, then ask the flow once

Ownership and lifecycle checks should share one shape, as
`cancellationownership`, `goroutineownership`, `resourcelifetime`, and
`lockorder` do:

1. An obligation finder resolves what the worker or callee promises (a
   channel it signals, a group it settles, a cancel it must release) back to
   the caller's exact SSA values.
2. A classifier labels each instruction after the obligation as `join`,
   `transfer`, `unknown`, or `none`. `unknown` is any consumption the
   analysis cannot see through: an opaque call, a send, an `append`, a
   callback handed to unmodeled code, or a helper that lets the value escape.
3. One flow query decides the outcome: honored when exact actions cover every
   return, unknown when only opaque actions do, violated otherwise.

When a reviewed precision label fails, respond in this order and stop at the
first step that holds: widen `unknown` at the classifier; accept the false
negative and delete the fixture; add one structural predicate at an existing
decision point with a fixture and a commit-pinned link. Do not add a new
proof file, a loop-count argument, a name, or a framework guess. A fixture
whose diagnostic becomes an accepted false negative must be deleted, with the
gap recorded in the fixture file's header comment, rather than left as an
accepted case.

The precision replay runs with every check enabled, so noise from an opt-in
audit fails the gate like a default check. An audit whose labels keep failing
should be retired rather than refined.

## SSA traversal consistency

Treat SSA wrapper, alias, closure, and return traversal as explicit analysis
policy.

- Do not independently reproduce the same `ChangeInterface`, `ChangeType`,
  `Convert`, `MakeInterface`, `Phi`, or similar traversal in several analyzers.
  When multiple analyzers need the same mechanics, add one narrowly named
  helper under `internal/ssaflow`.
- Do not create a universal "unwrap everything" helper. Callers must select the
  exact transparent forms that are sound for their proof, and fixtures must
  cover a wrapper that remains intentionally opaque.
- Do not hand-roll a recursive value walk with its own visited set. Fold over
  reaching values with `ssaflow.ReachingWalk` (`Any`, `Every`, `EveryOf`, or
  `ResolveReachingValue`), passing the analyzer's own transparent forms and
  leaf predicate; the fold owns the visited set and the phi fan-out. Drive a
  path-sensitive work list with `ssaflow.WalkStates`, keeping the state type,
  the transfer, and the successor policy in the analyzer. Analyzer code must
  not read `Phi.Edges` or declare a visited set keyed by SSA values; the
  architecture tests reject both. Pair edges with their predecessor blocks
  through `ssaflow.PhiIncoming`, and ask use-after questions with
  `ssaflow.InstructionsReachableAfter`. `ssaflow.IdentitySource` peels a load
  for identity resolution only; it is deliberately not a fold form, because
  a loaded value is not the cell it came from.
- Keep analyzer-specific acceptance policy beside the analyzer. Shared SSA code
  should provide identity and traversal mechanics, not silently decide whether
  evidence is sufficient for a diagnostic.
- Lifecycle completion is one search in `internal/ssaflow`: resolve the
  callee an instruction launches, map the target onto its parameters and
  captures, and require the cleanup call before every normal return. Callers
  choose the instructions they submit and whether they need a must-complete
  or may-complete answer; do not add launch-specific completion modes or
  reproduce the search for one analyzer.

## Semantic heuristics

Names may support a proof but should rarely establish semantics by themselves.

- Do not infer serialization, ownership, lifecycle, synchronization, or
  cancellation solely from a project-defined type, function, field, or method
  name.
- Name-only matching is allowed only for a documented language, standard
  library, external API, or explicit structural contract. Document that
  contract at the decision point.
- Every name-based heuristic must include an accepted fixture using a
  misleading but plausible name, so ordinary concepts such as rows, contexts,
  owners, or registries do not become accidental evidence.
- Prefer type identity, method signatures, data flow, registration structure,
  and feasible-path evidence over suffixes or name fragments.

## Precision fixture organization

Organize regression fixtures by evidence family rather than maintaining one
ever-growing scenario catalog.

- When a fixture file covers several independent proof models, split it into
  focused files within the same testdata package, such as `joins.go`,
  `returned_owners.go`, or `helper_cleanup.go`.
- Keep accepted and diagnostic forms for one precision boundary close together.
- A new fixture should make it obvious which proof function owns the behavior
  without requiring a search through hundreds of unrelated scenarios.

## Analyzer organization

### Well-known symbol identity

Match known package functions, receiver-qualified methods, builtins, and
package variables through `syntax.Symbol` and the AST or SSA symbol
matchers. Do not reconstruct declaration identity from package paths and raw
names. Keep analyzer-specific symbol declarations beside the contract that
uses them; the shared package owns identity mechanics, not a global catalog.

Name-only matching remains appropriate for documented structural contracts,
such as cleanup or ownership methods on an already-proven receiver. Package-
wide API families and user-configured qualified names may use package metadata,
but each such escape is an explicit architecture-test review point.

Follow the layering common in mature Go analyzer projects: keep analyzer
registration and top-level traversal easy to find, and isolate substantial
evidence engines behind focused implementation files.

- Start an analyzer in one file. Split it only when a concern has distinct
  vocabulary or invariants, such as source-level API contracts versus SSA flow
  analysis; file length alone is not a reason to split.
- Give every analyzer its own package under
  `internal/analyzers/<group>/<name>`. Group directories mirror the catalog
  for navigation but remain organizational containers, not shared Go
  packages. Name focused files by their concern, such as `timer.go`,
  `contracts.go`, or `immutability.go`; do not create a package per helper or
  analysis phase.
- Keep registry and runner code small. It should select configuration,
  construct shared inputs, invoke evidence helpers, and report diagnostics—not
  contain the full proof itself.
- Promote a helper to `internal/syntax` or `internal/ssaflow` only after multiple analyzers
  need the same general contract. Analyzer-specific precision policy belongs
  beside the analyzer even when its implementation looks reusable.
- Put shared prerequisite `analysis.Analyzer` passes under
  `internal/passes`; these are execution infrastructure, not catalog
  analyzers or general-purpose source and flow helpers.
- Keep each analyzer's minimized accepted and diagnostic cases under its local
  `testdata` tree. Place fixture-only dependency stubs there as well.

This follows Staticcheck's grouped one-package-per-pass layout while retaining
gosec's practice of splitting a substantial analyzer into focused files within
its package. Analyzer groups are catalog metadata mirrored by container
directories, not Go package boundaries.

Shared source-level helpers live under `internal/syntax`, while SSA flow and
ownership mechanics live under `internal/ssaflow`; they are implementation
details rather than an external integration API. Cross-cutting
diagnostic, catalog, flag, and trace infrastructure lives in its own focused
internal package instead of being folded into analysis utilities.

Repository-wide source conformance tests live under `internal/architecture`.
Keep behavioral tests for the facilities they enforce, such as symbol matching,
beside the implementation in its owning package.

## Documentation website

Use `make site-review` when testing the documentation website. It starts the
Astro development server together with the Agentation services, so annotations
can be submitted to an agent during review.
