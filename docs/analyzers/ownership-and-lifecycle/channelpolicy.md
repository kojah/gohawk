---
title: channelpolicy
description: "Let the creator close the channel."
---

## What it detects

Reports channels with large constant capacities that have no nearby rationale,
code that closes a channel received from its caller, and sends that can happen
after the channel has been closed.

## Why this is flagged

Closing a channel from the wrong place can race with a sender and panic. Giving
one owner responsibility for closing makes the channel's lifetime predictable;
avoiding unexplained large buffers also keeps backpressure problems visible.

Further reading: [Go concurrency patterns: Pipelines and cancellation](https://go.dev/blog/pipelines).

## How to fix it

Let the code that creates and sends on the channel close it after the last
send. Prefer an unbuffered or small channel unless the larger capacity has a
clear, documented reason.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6ImRvIG5vdCBjbG9zZSBhIGNoYW5uZWwgcmVjZWl2ZWQgZnJvbSBjYWxsZXIiLCJzdGFydExpbmUiOjEsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjoxLCJlbmRDb2x1bW4iOjIxfV0"
func consume(events chan Event) {
  defer close(events)
  for event := range events {
    handle(event)
  }
}
```

### Accepted code

```go
func consumeSafely(events <-chan Event) {
  for event := range events {
    handle(event)
  }
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `max-unexplained-capacity` | `1` | Largest channel capacity allowed without a rationale; negative disables the check. |
<!-- gohawk:generated-options:end -->
