---
title: testpolicy
description: "Mark test helpers."
---

## Rule details

Mark test helpers.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6InRlc3QgaGVscGVyIGFjY2VwdGluZyB0IG11c3QgY2FsbCB0LkhlbHBlcigpIG9uIGV2ZXJ5IHJldHVybiBwYXRoIiwic3RhcnRMaW5lIjowLCJzdGFydENvbHVtbiI6NSwiZW5kTGluZSI6MCwiZW5kQ29sdW1uIjoxNn1d"
func requireUser(t *testing.T, user *User) {
  if user == nil {
    t.Fatal("expected a user")
  }
}
```

### OK

```go
func requireUserSafely(t *testing.T, user *User) {
  t.Helper()
  if user == nil {
    t.Fatal("expected a user")
  }
}
```
<!-- gohawk:generated-examples:end -->
