# Contributing to GoHawk

Thanks for helping improve GoHawk.

## Start here

GoHawk values precision over recall. A missed finding is usually less harmful
than a false alert that teaches people to ignore the tool. Only report a problem
when the analyzer has strong evidence that the code is wrong.

Before expanding a check, model common cleanup, ownership-transfer,
registration, and lifecycle patterns. Fix recurring false positives in the
analyzer instead of adding project-specific exemptions or suppression comments.

The `analysisutil` package is shared with Veritas, but it is not a supported
public API. Please do not document or expand it for external use.

## Making a change

- Keep changes focused and explain the behavior they are meant to catch.
- Add small regression fixtures for both flagged and accepted forms.
- Prefer general API contracts over project or function-name exceptions.
- Add analyzer options only for real project-level choices, such as thresholds,
  trust boundaries, recognized contracts, or policy modes.
- Dogfood meaningful analyzer changes on representative Go projects and
  investigate any new findings before broadening the check.

Test fixtures live under `testdata/src`, with analyzer registration and
configuration coverage in `analyzers_test.go`. CLI behavior belongs in
`cmd/gohawk/main_test.go`.

## Before opening a pull request

Run the same core checks as CI:

```sh
go mod verify
test -z "$(gofmt -l .)"
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/gohawk ./...
```

In the pull request, describe the policy change, the evidence behind it, and
any precision tradeoffs you considered.
