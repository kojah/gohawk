![gohawk logo: a hawk sheltering the Go gopher](assets/gohawk-logo.png)

# gohawk

[![CI](https://github.com/kojah/gohawk/actions/workflows/ci.yml/badge.svg)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/Coverage-80.3%25-brightgreen)](https://github.com/kojah/gohawk/actions/workflows/ci.yml)

gohawk is a focused set of static analyzers for Go, designed to run alongside
`go vet`, Staticcheck, and go-critic. It covers gaps around ownership,
lifecycle, concurrency, API contracts, and path-aware policy checks.

[Read the documentation](https://kojah.github.io/gohawk/)

## Quick Start

```sh
# Install.
go install github.com/kojah/gohawk@latest

# Run the default profile.
gohawk ./...

# See every analyzer, its profile, and its tags.
gohawk list

# Use it with go vet.
go vet -vettool="$(command -v gohawk)" ./...

# Preview safe fixes, then apply them.
gohawk -fix -diff ./...
gohawk -fix ./...

# Run selected opt-in checks, or exclude one from the defaults.
gohawk -wirepolicy -globalstate ./...
gohawk -determinism=false ./...

# Run every analyzer, including opt-in checks.
gohawk -enable-all ./...
```

## Documentation

The [documentation website](https://kojah.github.io/gohawk/) contains the full
analyzer reference, configuration and suppression guidance, installation
options, and contributing guide.

## License

Licensed under either the Apache License, Version 2.0 or the MIT License, at
your option.
