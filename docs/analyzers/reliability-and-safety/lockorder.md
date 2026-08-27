---
title: lockorder
description: "Acquire locks consistently."
---

## Rule details

Acquire locks consistently and release them on every return path. A missing
unlock is reported only when another path demonstrates that the function owns
the corresponding release policy.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImNvbnRyYWRpY3RvcnkgbG9jayBvcmRlcjogZmlyc3QgYW5kIHNlY29uZCIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjU1LCJlbmRMaW5lIjoxLCJlbmRDb2x1bW4iOjY3fV0"
func forward() { first.Lock(); defer first.Unlock(); second.Lock(); defer second.Unlock() }
func reverse() { second.Lock(); defer second.Unlock(); first.Lock(); defer first.Unlock() }
```

### OK

```go
func forwardSafely() { third.Lock(); defer third.Unlock(); fourth.Lock(); defer fourth.Unlock() }
func reverseSafely() { third.Lock(); defer third.Unlock(); fourth.Lock(); defer fourth.Unlock() }
```
<!-- gohawk:generated-examples:end -->
