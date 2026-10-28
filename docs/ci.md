---
title: Continuous integration
description: Run gohawk as a step in your build pipeline.
---

gohawk runs as a `go vet` tool and exits with status 3 when it reports a
finding, so a pipeline step that runs it fails on findings. For GitHub
Actions:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod

- run: go install github.com/kojah/gohawk@v0.7.0
- run: gohawk ./...
```

In a `go.work` workspace, point `go-version-file` at `go.work` and run
`gohawk work`, which analyzes every module.
