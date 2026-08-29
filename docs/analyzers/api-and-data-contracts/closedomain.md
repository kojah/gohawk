---
title: closedomain
description: "Represent closed sets with named types."
---

## What it detects

Finds exported string fields that are used like a fixed set of choices—for
example, a status that is repeatedly assigned or compared with the same small
group of values—but are still declared as plain strings.

## Why this is flagged

A plain string or integer can hold values that the program does not actually
support. A named type with constants makes invalid values harder to introduce,
helps tools find every use, and makes missing cases easier to spot.

## How to fix it

Create a named type for the set and define a constant for each supported value.
Convert strings or numbers from users and external systems at the boundary,
rejecting any value that is not part of the set.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6ImZpZWxkIFN0YXRlIHVzZXMgYSBjbG9zZWQgc3RyaW5nIGRvbWFpbjsgZGVmaW5lIGEgbmFtZWQgc3RyaW5nIHR5cGUgYW5kIGNvbnN0YW50cyIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjIsImVuZExpbmUiOjEsImVuZENvbHVtbiI6MTR9XQ"
type Job struct {
  State string
}

func finished(job Job) bool {
  return job.State == "done" || job.State == "failed"
}
```

### Accepted code

```go
type TaskState string

const (
  TaskDone   TaskState = "done"
  TaskFailed TaskState = "failed"
)

type Task struct {
  State TaskState
}
```
<!-- gohawk:generated-examples:end -->
