---
title: channelpolicy
description: "Let the creator close the channel."
---

## Rule details

Let the creator close the channel.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImRvIG5vdCBjbG9zZSBhIGNoYW5uZWwgcmVjZWl2ZWQgZnJvbSBjYWxsZXIiLCJzdGFydExpbmUiOjEsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjoxLCJlbmRDb2x1bW4iOjIxfV0"
func consume(events chan Event) {
  defer close(events)
  for event := range events {
    handle(event)
  }
}
```

### OK

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
