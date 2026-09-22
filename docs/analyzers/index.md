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

These analyzers check goroutine lifecycles, channel protocols, and synchronization.

<div class="analyzer-grid">
  <a class="analyzer-card" href="concurrency-and-synchronization/channelprotocol/">
    <span class="analyzer-name">channelprotocol</span>
    <span class="analyzer-detects">Checks for proven channel waiting cycles between a caller and worker.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/channelsafety/">
    <span class="analyzer-name">channelsafety</span>
    <span class="analyzer-detects">Checks channel operations for reachable use after close.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/concurrentcapture/">
    <span class="analyzer-name">concurrentcapture</span>
    <span class="analyzer-detects">Checks locals mutated by goroutines launched repeatedly.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/condsafety/">
    <span class="analyzer-name">condsafety</span>
    <span class="analyzer-detects">Checks Cond waits on proven unlocked mutexes.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/goroutineownership/">
    <span class="analyzer-name">goroutineownership</span>
    <span class="analyzer-detects">Checks that proven goroutine completion obligations are honored.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/lockorder/">
    <span class="analyzer-name">lockorder</span>
    <span class="analyzer-detects">Checks contradictory mutex acquisition order and unreleased return paths.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/oncepolicy/">
    <span class="analyzer-name">oncepolicy</span>
    <span class="analyzer-detects">Checks sync.Once function wrappers that are immediately discarded.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/producerlifecycle/">
    <span class="analyzer-name">producerlifecycle</span>
    <span class="analyzer-detects">Checks that goroutine producers cannot outlive their receivers.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/syncmapatomicity/">
    <span class="analyzer-name">syncmapatomicity</span>
    <span class="analyzer-detects">Checks non-atomic sync.Map load-and-delete claims.</span>
  </a>
  <a class="analyzer-card" href="concurrency-and-synchronization/waitgroupsafety/">
    <span class="analyzer-name">waitgroupsafety</span>
    <span class="analyzer-detects">Checks proven WaitGroup counter underflows.</span>
  </a>
</div>

## Resources and lifecycle

These analyzers check resource ownership, cleanup, and lifetimes.

<div class="analyzer-grid">
  <a class="analyzer-card" href="resources-and-lifecycle/borrowedstorage/">
    <span class="analyzer-name">borrowedstorage</span>
    <span class="analyzer-detects">Checks borrowed mutable storage transferred to a second owner.</span>
  </a>
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
  <a class="analyzer-card" href="general-correctness/evalorder/">
    <span class="analyzer-name">evalorder</span>
    <span class="analyzer-detects">Checks later operands that mutate values evaluated earlier.</span>
  </a>
  <a class="analyzer-card" href="general-correctness/inlineerror/">
    <span class="analyzer-name">inlineerror</span>
    <span class="analyzer-detects">Checks inline error declarations for mismatched conditions.</span>
  </a>
</div>
