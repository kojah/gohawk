<p align="center">
  <img src="site/public/gohawk-logo.png" alt="gohawk logo: a hawk sheltering the Go gopher" width="400">
</p>

# gohawk

[![Go 1.26](https://img.shields.io/github/actions/workflow/status/kojah/gohawk/go-1.26.yml?branch=main&label=Go%201.26)](https://github.com/kojah/gohawk/actions/workflows/go-1.26.yml)
[![Go 1.27](https://img.shields.io/github/actions/workflow/status/kojah/gohawk/ci.yml?branch=main&label=Go%201.27)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/Coverage-90.3%25-brightgreen)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)

gohawk is an industrial-grade static analyzer that finds resource management
and concurrency issues in Go code. It has been used to find and fix bugs in
[Docker](https://github.com/moby/moby/pull/53517),
[Kubernetes](https://github.com/kubernetes/kubernetes/pull/142429), and
[Caddy](https://github.com/caddyserver/caddy/pull/7968).

It finds problems such as:

- a file, HTTP response body, or SQL rows value that is not closed on every
  return path;
- a `context` cancel function that is not called on every return path;
- a goroutine that is not waited for before its function returns;
- two locks taken in opposite orders, which can deadlock.

Each finding points at the exact code and shows the evidence for it, such as
the return that leaks a file. gohawk reports only what it can prove, so a
finding is worth acting on.

[Read the documentation](https://gohawk.dev/)

## Quick Start

```sh
# Install.
go install github.com/kojah/gohawk@latest

# Run the conservative default set.
gohawk ./...

# See every analyzer with its tier and group.
gohawk list

# Inspect an analyzer or one of its checks.
gohawk doc lockorder
gohawk doc lockorder/missing-release

# Use it with go vet.
go vet -vettool="$(command -v gohawk)" ./...

# Run a selected analyzer, or exclude one from the defaults.
gohawk -enable=channelsafety ./...
gohawk -disable=channelsafety ./...

# Run complete analyzer groups with their default checks.
gohawk -enable-groups=concurrency,resources ./...

# Run one experimental check by its ID.
gohawk -enable-checks=producerlifecycle/unclosed-range ./...

# Remove groups from the ordinary run or from -enable-all.
gohawk -disable-groups=concurrency ./...

# Run every analyzer and check.
gohawk -enable-all ./...
```

## How gohawk works

gohawk is heavily inspired by Meta's [Infer](https://fbinfer.com/) and its
compositional summary model. gohawk writes a summary of each function: what
the function does with its arguments and results, such as closing a file or
waiting for a goroutine. When one function calls another, gohawk reads the
callee's summary instead of analyzing the callee again. This lets gohawk
follow a resource through helper functions and across packages, and keeps
analysis fast on large code bases.

## How gohawk compares to other analyzers

gohawk aims to complement other Go analyzers, not replace them. Each tool
looks for a different kind of problem, so running several together catches
more than any one alone.

| Tool | What it focuses on | How gohawk fits in |
| --- | --- | --- |
| [`go vet`](https://pkg.go.dev/cmd/vet) | A small set of suspicious code patterns, shipped with Go | gohawk can run as a `go vet` tool |
| [Staticcheck](https://staticcheck.dev/) | A large, mature set of bug, style, and simplification checks | gohawk goes deeper on one topic: who owns a value and who cleans it up |
| [NilAway](https://github.com/uber-go/nilaway) | Possible nil pointer panics | gohawk does not look for nil panics |
| [gosec](https://github.com/securego/gosec) | Security problems | gohawk looks for correctness bugs, not security issues |
| [go-critic](https://github.com/go-critic/go-critic) | Many quick style and code-quality checks | gohawk follows how values move through your program |

gohawk also runs inside [golangci-lint](https://golangci-lint.run/), next to
the linters you already use.

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
