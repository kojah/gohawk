# 500-repository precision audit

`500-repository.tsv` is the append-only selection ledger for the 50-batch
`-enable-all` audit. Repository revisions and analyzed modules are pinned so a
finding can be reproduced after the temporary checkout is gone. `reviewed`
means the batch's diagnostics were grouped and inspected for generalizable,
high-confidence false positives; it does not mean every policy finding was
adopted by the external project.

## Batch 1

Ten repositories and eleven modules were scanned. Three recurring evidence
gaps were fixed:

- an immediately invoked closure that registers a nested cleanup defer;
- a launched process waiter that registers `Wait` through a nested defer; and
- direct reaping through `cmd.Process.Wait`.

The fixes remove findings in uzomuzo, feint, and mache. Nearby unclosed HTTP
responses, transactions without rollback, and successful process starts with
no complete reap path remained reportable and are pinned in precision round 5.
Cloudemu produced no findings. Uzomuzo's CGO-only tree-sitter package did not
load under the audit's `CGO_ENABLED=0` environment; its other packages were
still analyzed.

## Batch 2

Ten repositories and twenty-one modules were selected. Twenty modules loaded
under `CGO_ENABLED=0`; TerraTidy's `examples/go-rule` module was blocked by a
missing committed `go.sum` entry.

Four general evidence gaps were fixed:

- send-only and receive-only conversions of the same channel now retain one
  identity;
- a map-range worker launch may be paired with receives bounded by the stable
  length of that same map;
- `testing/synctest.Test` owns goroutines started in its bubble; and
- declarations compiled only from `_test.go` are not treated as production
  APIs.

More involved dynamic-lock, guarded-branch, recursive-worker, custom lifecycle,
and multi-stage resource transfers remain conservative rather than receiving
project-specific exceptions. Nearby caller-channel ownership, goroutine,
resource, and defer-in-loop findings remain reportable in precision round 6.
