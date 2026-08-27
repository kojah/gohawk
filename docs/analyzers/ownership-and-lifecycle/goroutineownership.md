---
title: goroutineownership
description: "Join spawned goroutines."
---

## Rule details

Give spawned goroutines a recognizable lifecycle owner. Local producer
goroutines must also be able to stop sending when their receiver leaves; use a
cancellation-aware `select`, drain the channel, or provide enough proven buffer
capacity for every send.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6Imdvcm91dGluZSBpcyBub3Qgam9pbmVkIG9uIGV2ZXJ5IHJldHVybiBwYXRoIiwic3RhcnRMaW5lIjoxLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MSwiZW5kQ29sdW1uIjoxOH1d"
func refresh() {
  go updateCache()
}
```

### OK

```go
func refreshSafely() {
  var group sync.WaitGroup
  group.Add(1)
  go func() {
    defer group.Done()
    updateCache()
  }()
  group.Wait()
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `mode` | `context` | Ownership policy: context, lifecycle, or join. |
<!-- gohawk:generated-options:end -->

By default, a context is enough to own a worker. Use
`-goroutineownership.mode=lifecycle` to require a lifecycle owner, or
`-goroutineownership.mode=join` to require an explicit join.
