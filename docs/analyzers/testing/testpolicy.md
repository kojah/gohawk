---
title: testpolicy
description: "Mark test helpers."
---

## What it detects

Finds non-entry-point test functions that accept `*testing.T` or `*testing.B`
and can return without calling the handle's `Helper` method.

## Why this is flagged

Without `t.Helper()`, a failure inside a shared test helper points at the
helper itself instead of the test call that caused it. Marking the helper gives
developers a useful file and line when the test fails.

Further reading: [`testing.T.Helper`](https://pkg.go.dev/testing#T.Helper).

## How to fix it

Call `t.Helper()` near the start of every function that acts as a test helper.
Do it on every path before the helper reports a failure or returns control to
the test.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6InRlc3QgaGVscGVyIGFjY2VwdGluZyB0IG11c3QgY2FsbCB0LkhlbHBlcigpIG9uIGV2ZXJ5IHJldHVybiBwYXRoIiwic3RhcnRMaW5lIjowLCJzdGFydENvbHVtbiI6NSwiZW5kTGluZSI6MCwiZW5kQ29sdW1uIjoxNn1d"
func requireUser(t *testing.T, user *User) {
  if user == nil {
    t.Fatal("expected a user")
  }
}
```

### Accepted code

```go
func requireUserSafely(t *testing.T, user *User) {
  t.Helper()
  if user == nil {
    t.Fatal("expected a user")
  }
}
```
<!-- gohawk:generated-examples:end -->
