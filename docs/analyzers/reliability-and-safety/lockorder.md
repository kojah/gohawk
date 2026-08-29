---
title: lockorder
description: "Acquire locks consistently."
---

## What it detects

Acquire locks consistently and release them on every return path. A missing
unlock is reported only when another path demonstrates that the function owns
the corresponding release policy.

### Checks

<!-- gohawk:generated-checks:start -->
<div class="analyzer-check-list">
  <article class="analyzer-check" id="check-lockorder-missing-release">
    <code class="analyzer-check-id">lockorder/missing-release</code>
    <p>Reports return paths that leave an owned lock held.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#correctness">correctness</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-lockorder-recursive-acquire">
    <code class="analyzer-check-id">lockorder/recursive-acquire</code>
    <p>Reports attempts to acquire a lock that is already held.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#correctness">correctness</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-lockorder-contradictory-order">
    <code class="analyzer-check-id">lockorder/contradictory-order</code>
    <p>Reports inconsistent acquisition order for the same pair of locks.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#correctness">correctness</a>
    </div>
  </article>
</div>
<!-- gohawk:generated-checks:end -->

## Why this is flagged

Two goroutines that acquire the same locks in different orders can wait on each
other forever. Failing to unlock on one return path can similarly block every
later caller that needs the lock.

## How to fix it

Choose one order for acquiring multiple locks and use it everywhere. Arrange
the matching unlock as soon as each lock is acquired, commonly with `defer`, so
early returns cannot leave it held.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3siY2hlY2siOiJsb2Nrb3JkZXIvY29udHJhZGljdG9yeS1vcmRlciIsIm1lc3NhZ2UiOiJjb250cmFkaWN0b3J5IGxvY2sgb3JkZXI6IGZpcnN0IGFuZCBzZWNvbmQiLCJzdGFydExpbmUiOjEzLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MTMsImVuZENvbHVtbiI6MTR9XQ"
var first sync.Mutex
var second sync.Mutex

func forward() {
  first.Lock()
  defer first.Unlock()
  second.Lock()
  defer second.Unlock()
}

func reverse() {
  second.Lock()
  defer second.Unlock()
  first.Lock()
  defer first.Unlock()
}
```

### Accepted code

```go
var third sync.Mutex
var fourth sync.Mutex

func forwardSafely() {
  third.Lock()
  defer third.Unlock()
  fourth.Lock()
  defer fourth.Unlock()
}

func reverseSafely() {
  third.Lock()
  defer third.Unlock()
  fourth.Lock()
  defer fourth.Unlock()
}
```
<!-- gohawk:generated-examples:end -->
