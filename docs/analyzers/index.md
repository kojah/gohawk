---
title: Analyzers
description: The gohawk analyzer catalog, generated from the registered Go analyzers.
---

<!-- Run go generate ./... to update this page; do not edit it by hand. -->

gohawk ships a focused set of analyzers rather than a general-purpose lint
catalog. Every analyzer is enabled by default; select individual analyzers when
you want a narrower run.

## API and data contracts

| Analyzer | What it catches |
| --- | --- |
| [`apishape`](api-and-data-contracts/apishape/) | Checks exported API parameter and receiver shape. |
| [`contextpolicy`](api-and-data-contracts/contextpolicy/) | Checks context placement, storage, nil use, and test ownership. |
| [`closedomain`](api-and-data-contracts/closedomain/) | Finds builtin-string fields used as closed semantic domains. |
| [`wirepolicy`](api-and-data-contracts/wirepolicy/) | Checks serialized structs and their composite literals. |

## Ownership and lifecycle

| Analyzer | What it catches |
| --- | --- |
| [`cancellationownership`](ownership-and-lifecycle/cancellationownership/) | Checks context and signal-derived cancellation functions are called on every return path. |
| [`channelpolicy`](ownership-and-lifecycle/channelpolicy/) | Checks channel capacity and closing ownership. |
| [`deferinloop`](ownership-and-lifecycle/deferinloop/) | Checks cleanup defers whose lifetime extends across loop iterations. |
| [`exitpolicy`](ownership-and-lifecycle/exitpolicy/) | Checks process termination that bypasses registered defers. |
| [`goroutineownership`](ownership-and-lifecycle/goroutineownership/) | Checks that explicit goroutines have a recognizable join handle or lifecycle owner. |
| [`processownership`](ownership-and-lifecycle/processownership/) | Checks that started os/exec commands are waited on or transferred to a wait owner. |
| [`resourcelifetime`](ownership-and-lifecycle/resourcelifetime/) | Checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path. |

## Reliability and safety

| Analyzer | What it catches |
| --- | --- |
| [`concurrentcapture`](reliability-and-safety/concurrentcapture/) | Checks locals mutated by goroutines launched repeatedly. |
| [`determinism`](reliability-and-safety/determinism/) | Checks map iteration reaching ordered output without explicit sorting. |
| [`errorownership`](reliability-and-safety/errorownership/) | Checks that errors are handled once and classified structurally. |
| [`evalorder`](reliability-and-safety/evalorder/) | Checks later operands that mutate values evaluated earlier. |
| [`globalstate`](reliability-and-safety/globalstate/) | Checks mutable package-level state. |
| [`lockorder`](reliability-and-safety/lockorder/) | Checks contradictory mutex acquisition order and unreleased return paths. |
| [`oncepolicy`](reliability-and-safety/oncepolicy/) | Checks sync.Once function wrappers that are immediately discarded. |
| [`syncmapatomicity`](reliability-and-safety/syncmapatomicity/) | Checks non-atomic sync.Map load-and-delete claims. |
| [`taintpolicy`](reliability-and-safety/taintpolicy/) | Checks untrusted environment and argument data reaching sensitive sinks. |

## Testing

| Analyzer | What it catches |
| --- | --- |
| [`blockingtest`](testing/blockingtest/) | Checks cancellation ownership for blocking test channels. |
| [`testpolicy`](testing/testpolicy/) | Checks lifecycle ownership in test helpers. |
