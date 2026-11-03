---
title: Continuous integration
description: Run gohawk as a step in your build pipeline.
---

gohawk exits with status 3 when it reports findings and status 1 when the
build or analysis fails, so a CI step that runs it fails the job in either
case. In GitHub Actions:

```yaml
- uses: actions/setup-go@v7
  with:
    go-version: stable

- run: go install github.com/kojah/gohawk@latest # or @vX.Y.Z for reproducible builds
- run: gohawk ./...
```

gohawk needs Go 1.26 or newer to build, so install a recent toolchain even if
your `go.mod` targets an older Go version. See
[Installation](/installation/) for the supported Go versions.
