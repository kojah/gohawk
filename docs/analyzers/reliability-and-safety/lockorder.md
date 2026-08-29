---
title: lockorder
description: "Acquire locks consistently."
---

## What it detects

Acquire locks consistently and release them on every return path. A missing
unlock is reported only when another path demonstrates that the function owns
the corresponding release policy.

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

```go gohawk="W3sibWVzc2FnZSI6ImNvbnRyYWRpY3RvcnkgbG9jayBvcmRlcjogZmlyc3QgYW5kIHNlY29uZCIsInN0YXJ0TGluZSI6MTMsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjoxMywiZW5kQ29sdW1uIjoxNH1d"
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
