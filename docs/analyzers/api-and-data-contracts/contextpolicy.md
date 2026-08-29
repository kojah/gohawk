---
title: contextpolicy
description: "Put context first."
---

## What it detects

Reports `context.Context` parameters that are not first, contexts stored in
structs, and calls that pass a definitely nil context. In supported test code,
it also prefers `t.Context()` or `b.Context()` over `context.Background()`.

## Why this is flagged

Keeping `context.Context` as the first parameter makes cancellation and
deadlines easy to pass through a call chain. Storing a context or passing nil
can give it the wrong lifetime or cause failures far from the call site.

Further reading: [Package context](https://pkg.go.dev/context),
[Contexts and structs](https://go.dev/blog/context-and-structs).

## How to fix it

Accept the context as the function's first parameter and pass it directly to
the work that needs it. Do not store it in a struct or pass nil; use an
appropriate real context, such as the test's context in test code.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

#### Context parameter order

```go gohawk="W3sibWVzc2FnZSI6ImNvbnRleHQuQ29udGV4dCBtdXN0IGJlIGZpcnN0IHBhcmFtZXRlciIsInN0YXJ0TGluZSI6MCwic3RhcnRDb2x1bW4iOjUsImVuZExpbmUiOjAsImVuZENvbHVtbiI6MTN9XQ"
func LoadUser(id string, ctx context.Context) error {
  return nil
}
```

#### Context stored in a struct

```go gohawk="W3sibWVzc2FnZSI6ImRvIG5vdCBzdG9yZSBjb250ZXh0LkNvbnRleHQgaW4gYSBzdHJ1Y3QiLCJzdGFydExpbmUiOjEsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjoxLCJlbmRDb2x1bW4iOjI1fV0"
type Request struct {
  Context context.Context
}
```

#### Nil context argument

```go gohawk="W3sibWVzc2FnZSI6ImRvIG5vdCBwYXNzIG5pbCBjb250ZXh0LkNvbnRleHQiLCJzdGFydExpbmUiOjMsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjozLCJlbmRDb2x1bW4iOjIwfV0"
func acceptContext(context.Context) {}

func loadWithoutContext() {
  acceptContext(nil)
}
```

### Accepted code

```go
func LoadUserCorrectly(ctx context.Context, id string) error {
  return nil
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `prefer-test-context` | `true` | Prefer t.Context or b.Context over context.Background in tests. |
<!-- gohawk:generated-options:end -->
