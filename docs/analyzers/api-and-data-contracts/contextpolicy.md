---
title: contextpolicy
description: "Put context first."
---

## What it detects

Reports `context.Context` parameters that are not first, contexts stored in
structs, and calls that pass a definitely nil context. In supported test code,
it also prefers `t.Context()` or `b.Context()` over `context.Background()`.

### Checks

<!-- gohawk:generated-checks:start -->
<div class="analyzer-check-list">
  <article class="analyzer-check" id="check-contextpolicy-context-first">
    <code class="analyzer-check-id">contextpolicy/context-first</code>
    <p>Reports context.Context parameters that are not first.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#reliability">reliability</a>
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-contextpolicy-context-storage">
    <code class="analyzer-check-id">contextpolicy/context-storage</code>
    <p>Reports context.Context values stored in structs.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#reliability">reliability</a>
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-contextpolicy-test-context">
    <code class="analyzer-check-id">contextpolicy/test-context</code>
    <p>Reports tests that use context.Background instead of the testing handle&#39;s context.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-contextpolicy-nil-context">
    <code class="analyzer-check-id">contextpolicy/nil-context</code>
    <p>Reports definitely nil context.Context arguments.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#correctness">correctness</a>
    </div>
  </article>
</div>
<!-- gohawk:generated-checks:end -->

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

```go gohawk="W3siY2hlY2siOiJjb250ZXh0cG9saWN5L2NvbnRleHQtZmlyc3QiLCJtZXNzYWdlIjoiY29udGV4dC5Db250ZXh0IG11c3QgYmUgZmlyc3QgcGFyYW1ldGVyIiwic3RhcnRMaW5lIjowLCJzdGFydENvbHVtbiI6NSwiZW5kTGluZSI6MCwiZW5kQ29sdW1uIjoxM31d"
func LoadUser(id string, ctx context.Context) error {
  return nil
}
```

#### Context stored in a struct

```go gohawk="W3siY2hlY2siOiJjb250ZXh0cG9saWN5L2NvbnRleHQtc3RvcmFnZSIsIm1lc3NhZ2UiOiJkbyBub3Qgc3RvcmUgY29udGV4dC5Db250ZXh0IGluIGEgc3RydWN0Iiwic3RhcnRMaW5lIjoxLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MSwiZW5kQ29sdW1uIjoyNX1d"
type Request struct {
  Context context.Context
}
```

#### Nil context argument

```go gohawk="W3siY2hlY2siOiJjb250ZXh0cG9saWN5L25pbC1jb250ZXh0IiwibWVzc2FnZSI6ImRvIG5vdCBwYXNzIG5pbCBjb250ZXh0LkNvbnRleHQiLCJzdGFydExpbmUiOjMsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjozLCJlbmRDb2x1bW4iOjIwfV0"
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
