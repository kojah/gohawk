---
title: Testing
description: Checks for cancellable waits and explicit helper ownership in tests.
---

These analyzers keep test failures bounded and make helper behavior visible at
the call site.

<!-- gohawk:generated-analyzers:start -->
| Analyzer | What it catches |
| --- | --- |
| [`blockingtest`](blockingtest/) | Checks cancellation ownership for blocking test channels. |
| [`testpolicy`](testpolicy/) | Checks lifecycle ownership in test helpers. |
<!-- gohawk:generated-analyzers:end -->
