---
title: apishape
description: "Group error-prone parameters."
---

## What it detects

Reports exported functions with too many parameters, long runs of parameters
that share one type, and adjacent optional scalar parameters that are easy to
swap. It also reports types that mix pointer and value receiver methods.

### Checks

<!-- gohawk:generated-checks:start -->
<div class="analyzer-check-list">
  <article class="analyzer-check" id="check-apishape-parameter-count">
    <code class="analyzer-check-id">apishape/parameter-count</code>
    <p>Reports exported APIs with more than the configured maximum number of parameters.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-apishape-mixed-receivers">
    <code class="analyzer-check-id">apishape/mixed-receivers</code>
    <p>Reports types that mix pointer and value receiver methods.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-apishape-adjacent-same-type">
    <code class="analyzer-check-id">apishape/adjacent-same-type</code>
    <p>Reports long adjacent runs of parameters that share one type.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-apishape-adjacent-optional-scalars">
    <code class="analyzer-check-id">apishape/adjacent-optional-scalars</code>
    <p>Reports adjacent optional scalar parameters that are easy to swap.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
</div>
<!-- gohawk:generated-checks:end -->

## Why this is flagged

Long parameter lists are difficult to read at call sites, and adjacent values
of the same type can be swapped without the compiler noticing. Grouping related
values in a named type makes each argument's meaning explicit and makes the API
safer to change later.

## How to fix it

Put related parameters into a small, clearly named options or request type.
When two adjacent values could be confused, give them distinct named types or
group them into a type whose field names explain their purpose.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

#### Too many parameters

```go gohawk="W3siY2hlY2siOiJhcGlzaGFwZS9wYXJhbWV0ZXItY291bnQiLCJtZXNzYWdlIjoiZXhwb3J0ZWQgQVBJIGhhcyA1IHBhcmFtZXRlcnM7IHVzZSBhbiBJbnB1dCBvciBjb25maWcgc3RydWN0Iiwic3RhcnRMaW5lIjowLCJzdGFydENvbHVtbiI6NSwiZW5kTGluZSI6MCwiZW5kQ29sdW1uIjoxNX1d"
func CreateUser(name string, age int, active bool, score float64, role byte) error {
  return nil
}
```

#### Adjacent optional parameters

```go gohawk="W3siY2hlY2siOiJhcGlzaGFwZS9hZGphY2VudC1vcHRpb25hbC1zY2FsYXJzIiwibWVzc2FnZSI6ImFkamFjZW50IG9wdGlvbmFsIHNjYWxhciBwYXJhbWV0ZXJzIGFyZSBlYXN5IHRvIHN3YXA7IHVzZSBhbiBJbnB1dCBzdHJ1Y3QiLCJzdGFydExpbmUiOjAsInN0YXJ0Q29sdW1uIjo1LCJlbmRMaW5lIjowLCJlbmRDb2x1bW4iOjEzfV0"
func FindUser(firstName, lastName *string) error {
  return nil
}
```

### Accepted code

```go
type CreateUserInput struct {
  Name, Email, City, Country, Role string
}

func CreateUserWithInput(input CreateUserInput) error {
  return nil
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `max-adjacent-same-type` | `2` | Maximum adjacent parameters of one type; 0 disables the check. |
| `max-parameters` | `4` | Maximum exported function parameters; 0 disables the check. |
<!-- gohawk:generated-options:end -->
