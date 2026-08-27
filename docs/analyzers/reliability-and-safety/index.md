---
title: Reliability and safety
description: Checks for deterministic output, error ownership, global state, lock order, and unsafe data flow.
---

These analyzers cover failure modes that often survive ordinary type checking
and code review.

<!-- gohawk:generated-analyzers:start -->
| Analyzer | What it catches |
| --- | --- |
| [`concurrentcapture`](concurrentcapture/) | Checks locals mutated by goroutines launched repeatedly. |
| [`determinism`](determinism/) | Checks map iteration reaching ordered output without explicit sorting. |
| [`errorownership`](errorownership/) | Checks that errors are handled once and classified structurally. |
| [`globalstate`](globalstate/) | Checks mutable package-level state. |
| [`lockorder`](lockorder/) | Checks contradictory mutex acquisition order and unreleased return paths. |
| [`taintpolicy`](taintpolicy/) | Checks untrusted environment and argument data reaching sensitive sinks. |
<!-- gohawk:generated-analyzers:end -->
