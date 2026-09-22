# Batch 50: bounded false-positive corrections

This follow-up removes **eight additional false-positive locations** using the
existing evidence machinery. Together with the eleven previously verified
removals, 19 of the original 55 false positives have correction evidence;
**36 remain unresolved**. The original finding ledger is unchanged.
Implementation commit: `393351c`.

## Corrected boundaries

| Repository | Finding | Correction and deliberate limit |
| --- | --- | --- |
| alajmo/sake | `lockorder`, `core/run/exec.go:1081:3` | A direct local mutex used only by synchronous mutex operations cannot affect another caller after return. Decline missing-release there; recursive acquisition and shared mutexes remain checked. |
| encodeous/nylon | `lockorder`, `polyamide/device/receive.go:179:6` | An unknown owner no longer falls back to a type-wide field identity. A lock declaration is not proof of one runtime instance. Fixed-key opaque getters can consequently be missed; direct known identities remain checked. |
| hashicorp/vault-secrets-operator | `goroutineownership`, `vault/client_factory.go:898:3` | Conditional deferred Done registration does not establish an unconditional completion obligation or prove an early Done. Helper defers require return coverage. Missing Done on an optional branch remains outside the proof; unconditional and independent obligations remain checked. |
| kubernetes/registry.k8s.io | `producerlifecycle`, `cmd/archeio/main_test.go:70:3` | Only same-worker sends that can precede a candidate compete for its receive. The later send at line 71 still reports; different workers and repeated sends remain checked. This is attribution, not a protocol model. |
| cloudflare/artifact-fs | `resourcelifetime`, `internal/fusefs/fuse_unix.go:275:18,277:18` | Storing a resource on a collection-selected owner is unknown: that owner may already be retained elsewhere. Direct local array slots and ordinary local owners are not exempted. |
| Autumn-27/ARTEX | `resourcelifetime`, `traffic/traffic.go:1159:13` | Calling a closure capturing a separate mutable aggregate owner is unknown when exact cleanup is unavailable. This does not prove rollback; a read-only use of such an owner may hide a leak. Direct resource captures and unrelated owners remain checked. |
| replicatedhq/troubleshoot | `deferinloop`, `pkg/supportbundle/aftercollection.go:32:3` | A consumer receiving a derived struct-pointer wrapper makes ownership unknown. Constructing a request without consuming it still leaves the deferred resource live. No HTTP-specific exemption was added. |

The implementation does not add shared infrastructure, a solver, pointer
analysis, channel scheduling, or an API-name exception. No check was retired,
demoted, or disabled. Diagnostic fixtures removed for the narrowed conditional
Done and unknown lock-instance boundaries are recorded as coverage gaps in
their fixture headers, not mislabeled as safe programs.

## Validation

- Focused analyzer tests and the full `make verify` passed. The full gate ran
  in an isolated source snapshot so downloaded candidate source beneath the
  main checkout's `.build` could not pollute architecture checks. It included
  module verification, formatting, generation checks, vet, lint/dead-code,
  self-analysis, all gohawk tests, and the repository's trace-component race
  tier.
- Round 53 pins seven repositories and passes all 25 labels with
  `REQUIRE_SCANNABLE=1`: 14 FP suppressions (eight new and the six earlier
  Nylon corrections) plus 11 true-positive controls. Nylon's fourth surviving
  order finding is still inconclusive, not promoted to a true-positive label.
- `findings.tsv` retains the original reviewed batch findings for those seven
  repositories, including unresolved findings. Passing labels do not claim
  that these repositories are diagnostic-free.
- Round 52 checked 28 labels: all 22 checked FP suppressions and six TP controls
  held. Cerbos's unrelated test-package build constraints prevented its two
  util labels from being checked by the whole-repository harness. A separate
  static production-file replay of the pinned util package verified both
  locations absent; this does not make the whole-repository gate a pass.
  The reported new gup process finding at `internal/lockfile/lockfile_test.go:371`
  also reproduces with the pre-change `b3f9996` binary, so it is baseline drift,
  not a regression introduced by these changes.
- The combined replay executable has SHA-256
  `3090dfc7d190df2901a6b8bb2c6ab699c2819168e7c3d3409a35805a48e45819`.
  It is a working-tree validation artifact, not a release or clean-revision
  provenance stamp. The frozen original batch scanner was not replaced.

All external-repository work was static analysis and source inspection. No
candidate tests, generators, applications, or benchmarks were executed, and
no outreach or GitHub comments were made.

## Remaining evidence gaps

| Analyzer | Remaining original FP locations | Why these were not patched here |
| --- | ---: | --- |
| resourcelifetime | 17 | Caller-owned SQL transaction cleanup (3), intended failed acquisitions (3), empty HTTP bodies (2), SMB map ownership/guard correlation (4), branch-merged owner cleanup (1), raw-descriptor aliases (4). These need stronger evidence than a name or test-only exemption. |
| concurrentcapture | 7 | Sake's conditional capacity-one semaphore (4), metatube's distinct worker indices (2), and Benthos's mutex/channel handoff (1). These are synchronization or worker-correlation gaps, not cleanup exceptions. |
| processownership | 5 | Fence's process owners/collection (3), kluctl's pre-registered mutable owner cleanup (1), and Opskat's intentional relaunch lifecycle (1). Resolving a Start receiver through Storage fixed small fixtures but not the real guarded-owner cases; that exploratory change was discarded. |
| lockorder | 4 | Neffos's repeated by-value field guards require reliable symbolic-field correlation. Address-string equality would be unsafe. |
| cancellationownership | 2 | Playground's cross-goroutine feasibility and tonutils-go's selected timeout arm. Settling an entire select would hide leaks on its other arms; the timeout arm has no exclusive instruction before its loop backedge. |
| goroutineownership | 1 | Panel test cleanup through a subsequently populated returned owner remains unproved. |

The four unresolved batch-49 cases also remain unchanged. No remaining case
is declared fundamentally unmodelable merely because this bounded pass did
not resolve it. Retirement or demotion remains a separate approval decision.
