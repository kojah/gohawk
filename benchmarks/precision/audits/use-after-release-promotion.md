# Use-after-release promotion, 2026-09-22

Analyzer revision: `e6b8399`.

Promote `resourcelifetime/use-after-release` from experimental to core after
extending its exact identity proof with shared local storage. This is a
bounded invalidation check, not double-close detection or general heap analysis.

## Proof and regression coverage

The operation must be on the original resource, a direct release must dominate
it in the same iteration, and unmodeled effects must not invalidate the proof.
The supported operation names belong to known resource types, not arbitrary
methods named Close. Commit requires a proven success branch.

Fixtures pair reports through fields, saved aliases, constant array/slice
elements, agreeing stores and read-only helpers with accepted replacements,
mixed stores, different elements, opaque calls, asynchronous exposure and
whole-object overwrite. HTTP Body replacement and compression Reset are
accepted. A gzip reader's cleanup does not establish invalidation. Double
Close, deferred cleanup, rows.Err and rollback following a failed commit
remain accepted. In-memory writers are subject to invalidation independently
of their leak exemption.

The focused fixture run reports 12 intended uses. Its trace records five opaque
effect decisions, one non-dominating release and one unproven Commit success.
Traced and untraced JSON output compares byte-for-byte equal. `make verify`
passes, including scoped race tests of the tracer.

## Public-source validation

Two groups of retained, pinned batch-48 repositories were analyzed. Only root
modules were scanned, with test sources included. This did **not** execute
tests, generators, applications, or repository scripts.

| Group | Repository | Revision | Unique first-party direct-release sites |
| --- | --- | --- | ---: |
| A | abahmed/kwatch | ca51953b129adb8e772cd1318d6e79103a034353 | 17 |
| A | grafana/mcp-grafana | 20b20b3aec8ebc56162ffac233ac9de9e46f5684 | 3 |
| A | raviqqe/muffet | ea33f85e5644c609a114b00e1f4dfc757b15c8ee | 2 |
| B | google/certificate-transparency-go | f7ce2e30d372f5487a8d9b8e749ca71805c4f68f | 3 |
| B | hashicorp/terraform-mcp-server | e2481878ee40560a91f07c38c09a478ede9fb87a | 3 |
| B | pb33f/libopenapi | 07795ddc2c097af8581138ef290d6cf964110d74 | 2 |
| B | fluent/fluent-operator | 352f22c9458ccfcfcebd80541f1c4d026cc9b665 | 0 |

All seven scans completed without a use-after-release diagnostic. There were
38 first-party direct-release events, representing 30 unique source sites;
test variants account for duplicates. Dependency events are excluded. Fluent
loaded but had no matching first-party releases, so its silence is not positive
coverage of the proof. None of these repositories reached an invalidating-use
candidate; they provide accepted-code exposure, **not a recall measurement**.
The diagnostic and uncertainty boundaries are exercised by the local fixtures.
Other checks' findings are not classified as clean by this report.

An additional scan of joshmedeski/sesh at
`f470ccf2e336c21382689d5cea08f163b64cd9a7` was incomplete because generated test
mocks were absent. It is excluded from the seven completed scans and site count;
no generator was run to repair it.

The round-50 and round-51 all-check replays also passed: all 15 reviewed true
positives remain present and all 11 false positives remain absent, with no
baseline drift. These protect neighboring lifecycle behavior; they are not
new use-after-release labels.

## Reproduction

Build the analyzer revision containing this promotion. At each pinned checkout:

```sh
CGO_ENABLED=0 GOFLAGS=-mod=readonly GOWORK=off GOTOOLCHAIN=local \
  go vet -vettool=/absolute/path/to/gohawk \
  -enable=resourcelifetime \
  -enable-checks=resourcelifetime/use-after-release \
  -gohawk-include-tests -json \
  -gohawk-trace=resourcelifetime/use-after-release \
  -gohawk-trace-file=/absolute/path/to/fresh-trace.jsonl ./...
```

Use a fresh trace destination because trace output appends. Count unique
`known-resource-direct-release` candidate positions inside the checkout, not
dependency paths. Check diagnostic categories rather than assuming a JSON vet
exit code proves absence of findings. Each scan was bounded to three minutes.

Replay existing controls with `make precision-regression ROUND=round-50` and
`ROUND=round-51`, optionally setting `GOHAWK` and `CHECKOUT_ROOT` to reuse the
binary and pinned checkouts. No labels or baselines were changed.
