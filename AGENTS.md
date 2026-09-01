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

## Analyzer organization

Follow the layering common in mature Go analyzer projects: keep analyzer
registration and top-level traversal easy to find, and isolate substantial
evidence engines behind focused implementation files.

- Start an analyzer in one file. Split it only when a concern has distinct
  vocabulary or invariants, such as source-level API contracts versus SSA flow
  analysis; file length alone is not a reason to split.
- Give every analyzer its own package under `internal/analyzers/<name>`. Name
  focused files by their concern, such as `timer.go`, `contracts.go`, or
  `immutability.go`; do not create a package per helper or analysis phase.
- Keep registry and runner code small. It should select configuration,
  construct shared inputs, invoke evidence helpers, and report diagnostics—not
  contain the full proof itself.
- Promote a helper to `analysisutil` only after multiple analyzers need the
  same general contract. Analyzer-specific precision policy belongs beside the
  analyzer even when its implementation looks reusable.
- Keep each analyzer's minimized accepted and diagnostic cases under its local
  `testdata` tree. Place fixture-only dependency stubs there as well.

This follows the one-package-per-pass layouts used by `x/tools` and
Staticcheck while retaining gosec's practice of splitting a substantial
analyzer into focused files within its package. Analyzer groups are catalog
metadata, not Go package boundaries.

The `analysisutil` package is intentionally unsupported and undocumented for
external consumers. It is exposed only for Veritas's current integration.

## Documentation website

Use `make site-review` when testing the documentation website. It starts the
Astro development server together with the Agentation services, so annotations
can be submitted to an agent during review.
