---
title: Ownership and lifecycle
description: Checks for cancellation, channel, goroutine, process, and resource ownership.
---

These analyzers look for work or resources whose owner cannot be identified on
every relevant path.

<!-- gohawk:generated-analyzers:start -->
| Analyzer | What it catches |
| --- | --- |
| [`cancellationownership`](cancellationownership/) | Checks context and signal-derived cancellation functions are called on every return path. |
| [`channelpolicy`](channelpolicy/) | Checks channel capacity and closing ownership. |
| [`deferinloop`](deferinloop/) | Checks cleanup defers whose lifetime extends across loop iterations. |
| [`exitpolicy`](exitpolicy/) | Checks process termination that bypasses registered defers. |
| [`goroutineownership`](goroutineownership/) | Checks that explicit goroutines have a recognizable join handle or lifecycle owner. |
| [`processownership`](processownership/) | Checks that started os/exec commands are waited on or transferred to a wait owner. |
| [`resourcelifetime`](resourcelifetime/) | Checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path. |
<!-- gohawk:generated-analyzers:end -->
