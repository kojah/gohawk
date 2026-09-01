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
investigating analyzer behavior. Every diagnostic must flow through
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

## Analyzer organization

### Well-known symbol identity

Match known package functions, receiver-qualified methods, builtins, and
package variables through `analysisutil.Symbol` and the AST or SSA symbol
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
- Promote a helper to `internal/analysisutil` only after multiple analyzers
  need the same general contract. Analyzer-specific precision policy belongs
  beside the analyzer even when its implementation looks reusable.
- Put shared prerequisite `analysis.Analyzer` passes under
  `internal/analysispasses`; these are execution infrastructure, not catalog
  analyzers or general-purpose `internal/analysisutil` helpers.
- Keep each analyzer's minimized accepted and diagnostic cases under its local
  `testdata` tree. Place fixture-only dependency stubs there as well.

This follows Staticcheck's grouped one-package-per-pass layout while retaining
gosec's practice of splitting a substantial analyzer into focused files within
its package. Analyzer groups are catalog metadata mirrored by container
directories, not Go package boundaries.

Shared program-analysis helpers live under `internal/analysisutil`; they are
implementation details rather than an external integration API. Cross-cutting
diagnostic, catalog, flag, and trace infrastructure lives in its own focused
internal package instead of being folded into analysis utilities.

Repository-wide source conformance tests live under `internal/architecture`.
Keep behavioral tests for the facilities they enforce, such as symbol matching,
beside the implementation in its owning package.

## Documentation website

Use `make site-review` when testing the documentation website. It starts the
Astro development server together with the Agentation services, so annotations
can be submitted to an agent during review.
