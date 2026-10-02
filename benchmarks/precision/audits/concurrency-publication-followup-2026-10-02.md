# Concurrency publication allowance

Beads `gohawk-dho.44.11.5.25.5` carries the caller allowance through concurrency
fact inference and publication validation. Previously, export re-ran embedded
field-path searches without the inference budget, and local declaration lookup
lost its supplied allowance at the same boundary. Linear inference, validation
and a branch-alternative retry now share one per-function allowance. Parameter,
field, worker, condition/context and returned-entry metadata consume it.

Publication remains declaration-relative, with schema version 9. A failed
validation returns only the version marker, discarding all earlier effects,
cancellation requirements, workers and alternatives. Cached inference cannot
bypass validation; an existing projection cannot rescue an interrupted path.
The tests use actual SSA for cold and cached exports, local declarations,
workers, cancellation requirements, branch conditions/returns, empty facts,
and a padded spill whose initialization-order search exceeds a child allowance.
An alternative metadata cutoff discards an already completed first path.

Imported clone loops and nested alternative mutation isolation are separately
tracked by `gohawk-dho.44.11.5.25.6`. Package inventory, heap graph/type internals,
constant/string formatting and fact codec costs are not certified by this change.
Graph discovery tools were unavailable; inspection used scoped source fallback.
This work supplies no demonstrated production FP correction; the queue remains
11 unresolved sites and frozen audit labels are unchanged.

## Validation

The canonical local gate passes (`make verify VERIFY_TIMINGS=1`,
`.build/goal-publication-verify-final.log`): generation, module verification,
vet, formatting, deadcode, lint, local dogfood (58s) and ordinary tests (99s).
Post-receipt documentation checks also pass (`.build/goal-publication-docs.log`).

Focused concurrency tests pass (`.build/goal-publication-package.log` and
`.build/goal-publication-focused-reviewed.log`). Restoring only the unbounded
field query fails the padded-spill control; skipping only context-position
charges fails the partial-alternative control. The source overlays leave the
checkout unchanged. Both counterfactuals exit 1; receipts are
`.build/goal-publication-{field,context}-counterfactual.log`.

The final immutable binary `.build/goal-publication-reviewed` implements parent
`8ab714d` plus these production changes; SHA-256:
`8e05ca4bf38cd966449b87f5e8c1808fb207586ffc8ea0d63a6e34f8fe3fa8e2`.
Scoped direct runs use `-enable-all -json`, `CGO_ENABLED=0`,
`GOFLAGS=-mod=readonly`, and `GOWORK=off`. Both exit 3 with empty stderr:

- stargz pin `624678b4e421947534cbf0618f9609853cccee0f`, `./store`, retains
  the reviewed goroutine TP at `manager.go:193:2`, with return witness `217:2`;
- Openase pin `e530faf137e764337d5beaaf68af3be159eb17aa`,
  `./internal/orchestrator`, retains the two reviewed cancellation TPs at
  `runtime_launcher.go:414:22` and `runtime_process_lifecycle_slice.go:192:22`.

Receipts `.build/goal-publication-{stargz,openase}-reviewed.{json,err}` are
byte-identical to the respective spill-path parent JSON controls. Initial
`go vet` JSON scans also retain those findings, exiting 0 as that driver does
in JSON mode, and adding a trailing newline. Direct scans supply comparable
exit codes and exact bytes. Checkouts remained pinned and clean; no candidate
tests, generators or applications ran. No local race test or full
precision-regression audit is part of this iteration.
