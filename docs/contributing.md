---
title: Contributing
description: How to make precise, well-tested changes to gohawk.
sidebar:
  order: 3
---

Thanks for helping improve gohawk.

## Start here

gohawk values precision over recall. A missed finding is usually less harmful
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

Each analyzer also has a `testdata/src/doc<analyzer>` package containing one
flagged and one accepted example between `//gohawk:example` markers. These are
ordinary analyzer fixtures. The docs generator runs the analyzer against them
and publishes their source, messages, and diagnostic ranges, so edit the
fixture—not the generated Examples section on the analyzer page.

## Before opening a pull request

If analyzer registration, grouping, documentation, flags, or suggested-fix
support changed, refresh the generated documentation first:

```sh
go generate ./...
```

Run the same core checks as CI:

```sh
go mod verify
test -z "$(gofmt -l .)"
go run ./internal/cmd/gendocs -check
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/gohawk ./...
```

If test coverage changed, refresh the README badge with the same inputs used by
CI:

```sh
go test ./... -covermode=count -coverpkg=./... -coverprofile=coverage.out
go tool cover -func=coverage.out -o=coverage-summary.out
go run github.com/AlexBeauchemin/gobadge@v0.4.0 \
  -filename=coverage-summary.out \
  -target=README.md \
  -link=https://github.com/kojah/gohawk/actions/workflows/ci.yml
```

In the pull request, describe the policy change, the evidence behind it, and
any precision tradeoffs you considered.
