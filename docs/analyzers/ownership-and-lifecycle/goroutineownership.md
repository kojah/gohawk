---
title: goroutineownership
description: "Join spawned goroutines."
---

## What it detects

Give spawned goroutines a recognizable lifecycle owner. Local producer
goroutines must also be able to stop sending when their receiver leaves; use a
cancellation-aware `select`, drain the channel, or provide enough proven buffer
capacity for every send.

## Why this is flagged

A goroutine without a clear owner may keep running after its caller is done or
block forever while sending a result nobody will receive. These leaks consume
memory and can eventually make shutdowns or the whole program hang.

Further reading: [Go concurrency patterns: Pipelines and cancellation](https://go.dev/blog/pipelines),
[Go concurrency patterns: Context](https://go.dev/blog/context).

## How to fix it

Give the goroutine a clear stopping rule: cancel it with a context, join it
before returning, or hand it to a longer-lived owner. Make sends cancellation
aware so the goroutine can stop if its receiver leaves.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6Imdvcm91dGluZSBpcyBub3Qgam9pbmVkIG9uIGV2ZXJ5IHJldHVybiBwYXRoIiwic3RhcnRMaW5lIjoxLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MSwiZW5kQ29sdW1uIjoxOH1d"
func refresh() {
  go updateCache()
}
```

### Accepted code

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
