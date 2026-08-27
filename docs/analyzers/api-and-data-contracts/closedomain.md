---
title: closedomain
description: "Represent closed sets with named types."
---

## Rule details

Represent closed sets with named types.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImZpZWxkIFN0YXRlIHVzZXMgYSBjbG9zZWQgc3RyaW5nIGRvbWFpbjsgZGVmaW5lIGEgbmFtZWQgc3RyaW5nIHR5cGUgYW5kIGNvbnN0YW50cyIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjIsImVuZExpbmUiOjEsImVuZENvbHVtbiI6MTR9XQ"
type Job struct {
  State string
}

func finished(job Job) bool {
  return job.State == "done" || job.State == "failed"
}
```

### OK

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
