---
title: channelpolicy
description: "Let the creator close the channel."
---

## What it detects

Reports channels with large constant capacities that have no nearby rationale,
code that closes a channel received from its caller, and sends that can happen
after the channel has been closed.

### Checks

<!-- gohawk:generated-checks:start -->
<div class="analyzer-check-list">
  <article class="analyzer-check" id="check-channelpolicy-capacity-rationale">
    <code class="analyzer-check-id">channelpolicy/capacity-rationale</code>
    <p>Reports large constant channel capacities without a nearby bounded rationale.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-channelpolicy-caller-close">
    <code class="analyzer-check-id">channelpolicy/caller-close</code>
    <p>Reports functions that close channels received from their callers.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#reliability">reliability</a>
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-channelpolicy-send-after-close">
    <code class="analyzer-check-id">channelpolicy/send-after-close</code>
    <p>Reports sends reachable after a channel has been closed.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#correctness">correctness</a>
    </div>
  </article>
</div>
<!-- gohawk:generated-checks:end -->

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

```go gohawk="W3siY2hlY2siOiJjaGFubmVscG9saWN5L2NhbGxlci1jbG9zZSIsIm1lc3NhZ2UiOiJkbyBub3QgY2xvc2UgYSBjaGFubmVsIHJlY2VpdmVkIGZyb20gY2FsbGVyIiwic3RhcnRMaW5lIjoxLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MSwiZW5kQ29sdW1uIjoyMX1d"
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
