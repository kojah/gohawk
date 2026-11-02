# gohawk review rules

These rules restate `AGENTS.md`. Precision is the product: a false diagnostic
costs more than a missed one.

## Findings

- Report a finding only when you can name a concrete feasible path to the
  violation. If ownership or lifecycle behavior is uncertain, stay silent.
- Do not suggest suppression comments or analyzer exemptions as a fix. Fix the
  pattern or extend the evidence model instead.
- Do not propose project-name, package-name, or function-name special cases
  unless they encode a documented general API contract.
- Prefer one general evidence model over accumulating per-API exceptions. If a
  pattern cannot be generalized or explained clearly, declining to analyze it is
  the correct outcome.

## Analyzer changes

- A new check or analyzer change needs accepted-form fixtures, not only a
  diagnostic fixture. Flag a PR that adds a diagnostic case without accepted
  cases.
- Every check needs an entry in `analyzers/catalog_specs.go` and a matching page
  under `docs/analyzers/`.
- `Kind` and `Tier` in the catalog must match what the docs claim about the
  check.

## Scope

- Style, formatting, and naming belong to golangci-lint. Skip those nits.
- Generated output (hyrule/stringer, `*_string.go`, `*.generated.*`) comes from
  `make generate`. Do not review it.
- Coverage and race testing run only in CI. Do not suggest local `-race` or
  `-cover` commands; `make verify` is the local entry point.