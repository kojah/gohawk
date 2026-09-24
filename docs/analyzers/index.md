---
title: All analyzers
description: The gohawk analyzer catalog, generated from the registered Go analyzers.
---

<!-- Run go generate ./... to update this page; do not edit it by hand. -->

gohawk ships a focused set of analyzers rather than a general-purpose lint
catalog. Every check identifies the kind of claim it makes:

- **Defect** means the available evidence establishes broken or ineffective behavior.
- **Hazard** means the behavior is risky, but harm depends on a wider runtime contract.
- **Policy** means valid Go violates an intentionally selected engineering convention.

Kind is descriptive metadata and does not change whether a check is enabled by default.

## Concurrency and synchronization

These analyzers check goroutine lifecycles, channel use, and synchronization.

<div class="analyzer-grid">
  <a class="analyzer-card" href="concurrency-and-synchronization/channelsafety/">
    <span class="analyzer-name">channelsafety</span>
    <span class="analyzer-detects">Checks channel operations for use after close and bounded dependency cycles.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/concurrentcapture/">
    <span class="analyzer-name">concurrentcapture</span>
    <span class="analyzer-detects">Checks locals mutated by goroutines launched repeatedly.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/goroutineownership/">
    <span class="analyzer-name">goroutineownership</span>
    <span class="analyzer-detects">Checks that proven goroutine completion obligations are honored.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/lockorder/">
    <span class="analyzer-name">lockorder</span>
    <span class="analyzer-detects">Checks contradictory mutex acquisition order and unreleased return paths.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/producerlifecycle/">
    <span class="analyzer-name">producerlifecycle</span>
    <span class="analyzer-detects">Checks that goroutine producers cannot outlive their receivers.</span>
  </a>
</div>

## Resources and lifecycle

These analyzers check resource ownership, cleanup, and lifetimes.

<div class="analyzer-grid">
  <a class="analyzer-card" href="resources-and-lifecycle/cancellationownership/">
    <span class="analyzer-name">cancellationownership</span>
    <span class="analyzer-detects">Checks context and signal-derived cancellation functions proved lost on a normal return path.</span>
  </a>
  <a class="analyzer-card" href="resources-and-lifecycle/deferinloop/">
    <span class="analyzer-name">deferinloop</span>
    <span class="analyzer-detects">Checks cleanup defers whose lifetime extends across loop iterations.</span>
  </a>
  <a class="analyzer-card" href="resources-and-lifecycle/processownership/">
    <span class="analyzer-name">processownership</span>
    <span class="analyzer-detects">Checks that started os/exec commands are waited on or transferred to a wait owner.</span>
  </a>
  <a class="analyzer-card" href="resources-and-lifecycle/resourcelifetime/">
    <span class="analyzer-name">resourcelifetime</span>
    <span class="analyzer-detects">Checks owned files, SQL handles, HTTP responses, and compressors are released on every path.</span>
  </a>
</div>

## General correctness

These analyzers check error handling and expression behavior beyond ordinary type checking.

<div class="analyzer-grid">
  <a class="analyzer-card" href="general-correctness/nilargument/">
    <span class="analyzer-name">nilargument</span>
    <span class="analyzer-detects">Checks calls that pass a nil value where the callee dereferences it on every path.</span>
  </a>
</div>
