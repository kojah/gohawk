<p align="center">
  <img src="site/public/gohawk-logo.png" alt="gohawk logo: a hawk sheltering the Go gopher" width="400">
</p>

# gohawk

[![CI](https://github.com/kojah/gohawk/actions/workflows/ci.yml/badge.svg)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/Coverage-85.9%25-brightgreen)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)

gohawk is a focused set of static analyzers for Go, designed to run alongside
`go vet`, Staticcheck, and go-critic. It covers gaps around ownership,
lifecycle, concurrency, API contracts, and path-aware policy checks.

It draws inspiration from [go-critic](https://github.com/go-critic/go-critic),
but is deliberately more focused on correctness and reliability, with fewer
opinionated style checks.

[Read the documentation](https://gohawk.dev/)

## Quick Start

```sh
# Install.
go install github.com/kojah/gohawk@latest

# Run the default profile.
gohawk ./...

# See every analyzer, its profile, and its group.
gohawk list

# Inspect an analyzer or one of its checks.
gohawk doc contextpolicy
gohawk doc contextpolicy/nil-context

# Use it with go vet.
go vet -vettool="$(command -v gohawk)" ./...

# Preview safe fixes, then apply them.
gohawk -fix -diff ./...
gohawk -fix ./...

# Run selected opt-in analyzers, or exclude one from the defaults.
gohawk -enable=wirepolicy,globalstate ./...
gohawk -disable=oncepolicy ./...

# Run complete analyzer groups with their default checks.
gohawk -enable-groups=ownership,testing ./...

# Run one opt-in check.
gohawk -enable-checks=contextpolicy/test-context ./...

# Remove groups from the default profile or from -enable-all.
gohawk -disable-groups=testing ./...

# Run every analyzer and check.
gohawk -enable-all ./...
```

## golangci-lint integration

gohawk can run as a module plugin inside a custom golangci-lint binary. See the
[golangci-lint integration guide](https://gohawk.dev/golangci-lint/)
for build and configuration instructions.

## Contributing

Contributions are welcome. See [How to contribute](https://gohawk.dev/contributing/)
for the development workflow, analyzer requirements, and verification steps.

## AI policy

gohawk was developed with assistance from LLMs, and AI-assisted contributions
are permitted. Contributors must disclose AI usage, and every contribution must
meet the project's strict standards for quality, testing, analyzer precision,
and human readability. See the full [AI policy](https://gohawk.dev/ai-policy/).

## Sponsorship

If gohawk is useful to you or your organization, consider sponsoring its
continued development. Sponsorship helps fund maintenance, new analyzers, and
improvements to the documentation and developer experience. To discuss
sponsorship, get in touch with [@kojah](https://github.com/kojah).

## License

Licensed under the MIT License.
