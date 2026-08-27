---
title: contextpolicy
description: "Put context first."
---

## Rule details

Put context first.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImNvbnRleHQuQ29udGV4dCBtdXN0IGJlIGZpcnN0IHBhcmFtZXRlciIsInN0YXJ0TGluZSI6MCwic3RhcnRDb2x1bW4iOjUsImVuZExpbmUiOjAsImVuZENvbHVtbiI6MTN9XQ"
func LoadUser(id string, ctx context.Context) error {
  return nil
}
```

### OK

```go
func LoadUserCorrectly(ctx context.Context, id string) error { return nil }
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `prefer-test-context` | `true` | Prefer t.Context or b.Context over context.Background in tests. |
<!-- gohawk:generated-options:end -->
