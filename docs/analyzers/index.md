---
title: All analyzers
description: The gohawk analyzer catalog, generated from the registered Go analyzers.
---

<!-- Run go generate ./... to update this page; do not edit it by hand. -->

gohawk ships a focused set of analyzers rather than a general-purpose lint
catalog.

## API and data contracts

These analyzers make contracts visible in Go types and APIs, where callers and tools can rely on them.

<div class="analyzer-grid">
  <a class="analyzer-card" href="api-and-data-contracts/apishape/">
    <span class="analyzer-name">apishape</span>
    <span class="analyzer-detects">Checks exported API parameter and receiver shape.</span>
  </a>
  <a class="analyzer-card" href="api-and-data-contracts/contextpolicy/">
    <span class="analyzer-name">contextpolicy</span>
    <span class="analyzer-detects">Checks context placement, storage, and nil use.</span>
  </a>
  <a class="analyzer-card" href="api-and-data-contracts/closedomain/">
    <span class="analyzer-name">closedomain</span>
    <span class="analyzer-detects">Finds builtin-string fields used as closed semantic domains.</span>
  </a>
  <a class="analyzer-card" href="api-and-data-contracts/wirepolicy/">
    <span class="analyzer-name">wirepolicy</span>
    <span class="analyzer-detects">Checks serialized structs and their composite literals.</span>
  </a>
</div>

## Ownership and lifecycle

These analyzers look for work or resources whose owner cannot be identified on every relevant path.

<div class="analyzer-grid">
  <a class="analyzer-card" href="ownership-and-lifecycle/cancellationownership/">
    <span class="analyzer-name">cancellationownership</span>
    <span class="analyzer-detects">Checks context and signal-derived cancellation functions are called on every return path.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/channelcapacity/">
    <span class="analyzer-name">channelcapacity</span>
    <span class="analyzer-detects">Checks that large channel buffers document their bound.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/channelownership/">
    <span class="analyzer-name">channelownership</span>
    <span class="analyzer-detects">Checks that channel closing remains with the channel owner.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/channelsafety/">
    <span class="analyzer-name">channelsafety</span>
    <span class="analyzer-detects">Checks channel operations for reachable use after close.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/deferinloop/">
    <span class="analyzer-name">deferinloop</span>
    <span class="analyzer-detects">Checks cleanup defers whose lifetime extends across loop iterations.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/exitpolicy/">
    <span class="analyzer-name">exitpolicy</span>
    <span class="analyzer-detects">Checks process termination that bypasses registered defers.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/goroutineownership/">
    <span class="analyzer-name">goroutineownership</span>
    <span class="analyzer-detects">Checks that explicit goroutines have a recognizable join handle or lifecycle owner.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/producerlifecycle/">
    <span class="analyzer-name">producerlifecycle</span>
    <span class="analyzer-detects">Checks that goroutine producers cannot outlive their receivers.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/processownership/">
    <span class="analyzer-name">processownership</span>
    <span class="analyzer-detects">Checks that started os/exec commands are waited on or transferred to a wait owner.</span>
  </a>
  <a class="analyzer-card" href="ownership-and-lifecycle/resourcelifetime/">
    <span class="analyzer-name">resourcelifetime</span>
    <span class="analyzer-detects">Checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path.</span>
  </a>
</div>

## Reliability and safety

These analyzers cover failure modes that often survive ordinary type checking and code review.

<div class="analyzer-grid">
  <a class="analyzer-card" href="reliability-and-safety/concurrentcapture/">
    <span class="analyzer-name">concurrentcapture</span>
    <span class="analyzer-detects">Checks locals mutated by goroutines launched repeatedly.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/determinism/">
    <span class="analyzer-name">determinism</span>
    <span class="analyzer-detects">Checks map iteration reaching ordered output without explicit sorting.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/errorownership/">
    <span class="analyzer-name">errorownership</span>
    <span class="analyzer-detects">Checks that one function does not both log and return an error.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/errorclassification/">
    <span class="analyzer-name">errorclassification</span>
    <span class="analyzer-detects">Checks that native Go errors are classified structurally.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/inlineerror/">
    <span class="analyzer-name">inlineerror</span>
    <span class="analyzer-detects">Checks inline error declarations for mismatched conditions.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/evalorder/">
    <span class="analyzer-name">evalorder</span>
    <span class="analyzer-detects">Checks later operands that mutate values evaluated earlier.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/globalstate/">
    <span class="analyzer-name">globalstate</span>
    <span class="analyzer-detects">Checks mutable package-level state.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/lockorder/">
    <span class="analyzer-name">lockorder</span>
    <span class="analyzer-detects">Checks contradictory mutex acquisition order and unreleased return paths.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/oncepolicy/">
    <span class="analyzer-name">oncepolicy</span>
    <span class="analyzer-detects">Checks sync.Once function wrappers that are immediately discarded.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/syncmapatomicity/">
    <span class="analyzer-name">syncmapatomicity</span>
    <span class="analyzer-detects">Checks non-atomic sync.Map load-and-delete claims.</span>
  </a>
  <a class="analyzer-card" href="reliability-and-safety/taintpolicy/">
    <span class="analyzer-name">taintpolicy</span>
    <span class="analyzer-detects">Checks untrusted environment and argument data reaching sensitive sinks.</span>
  </a>
</div>

## Testing

These analyzers keep test failures bounded and make helper behavior visible at the call site.

<div class="analyzer-grid">
  <a class="analyzer-card" href="testing/testlifecycle/">
    <span class="analyzer-name">testlifecycle</span>
    <span class="analyzer-detects">Checks that test-owned asynchronous work inherits the test lifecycle.</span>
  </a>
  <a class="analyzer-card" href="testing/testpolicy/">
    <span class="analyzer-name">testpolicy</span>
    <span class="analyzer-detects">Checks lifecycle ownership in test helpers.</span>
  </a>
</div>
