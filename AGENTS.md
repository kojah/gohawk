# GoHawk development guidance

Value high precision over high recall. Avoiding false alerts is more important
than detecting every possible violation: users must be able to trust that a
GoHawk diagnostic is actionable.

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

The `analysisutil` package is intentionally unsupported and undocumented for
external consumers. It is exposed only for Veritas's current integration.
