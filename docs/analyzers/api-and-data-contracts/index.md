---
title: API and data contracts
description: Checks for API shapes, context use, closed domains, and wire formats.
---

These analyzers make contracts visible in Go types and APIs, where callers and
tools can rely on them.

<!-- gohawk:generated-analyzers:start -->
| Analyzer | What it catches |
| --- | --- |
| [`apishape`](apishape/) | Checks exported API parameter and receiver shape. |
| [`contextpolicy`](contextpolicy/) | Checks context placement, storage, nil use, and test ownership. |
| [`closedomain`](closedomain/) | Finds builtin-string fields used as closed semantic domains. |
| [`wirepolicy`](wirepolicy/) | Checks serialized structs and their composite literals. |
<!-- gohawk:generated-analyzers:end -->
