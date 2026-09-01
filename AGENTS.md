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

## Analyzer tracing

Use the structured evidence tracer instead of temporary print statements when
investigating analyzer behavior. Every diagnostic must flow through
`analyzerbase.Report` or `analyzerbase.Reportf`, which provide repo-wide
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

All shared analysis helpers live under `internal/analysisutil`; they are
implementation details rather than an external integration API.

## Documentation website

Use `make site-review` when testing the documentation website. It starts the
Astro development server together with the Agentation services, so annotations
can be submitted to an agent during review.
