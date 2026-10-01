# Returned value-copy wrappers

The [earlier assessment](../audits/returned-logger-assessment-2026-10-01.md)
isolated missing writer-field relationships through scalar edits and nested
struct returns in urunc's zerolog construction chain. Raising the analyzer's
wrapper depth did not help. The shared heap projection now asks the existing
lazy content model for bounded reference fields in returned struct snapshots.
The analyzer's traversal and ownership policy stay the same.

The dependency-free heap fixtures fail on the parent for scalar edits, nested
extraction, and a preserved snapshot; replacement already worked. The imported
resource fixture also fails on parent `3649a40` at `returnedValueCopy`, then
passes with the correction. Discarded and replaced-writer forms still report.
Opaque writes, an earlier copy across an opaque call, arrays, and exhausted
field/slot bounds keep conservative uncertainty.

The corrected binary implements `bc80903` (built before the commit) and has SHA-256
`2d4f92f8f00cd43cb884aba688fd578bc583f816924a367139de28be2a50195a`.
The baseline production binary from `e36fd6a` has SHA-256
`935a652ca97dbcdb1c793b49888de8ed71a3eb5812720a11e8950be41e2b1492`.
At the pins in `repositories.tsv`, the affected urunc package scan exits zero
and removes the reviewed FP. Complete Cute and Basecamp scans retain both
production leak controls with no new or removed findings. `replay.tsv` records
each verification scope; historical test labels are not controls here.

Urunc's full root scan fails under the cohort's `CGO_ENABLED=0` profile:
`runc/libcontainer/nsenter` has no buildable files. Its partial parent/current
findings differ only by the reviewed FP; the `tests/e2e/utils.go` production
finding remains. This partial comparison does not certify the full repository.
`findings.tsv` contains the two complete control findings and that partial
urunc finding. The canonical three-repository replay remains unscannable for
urunc, so its FP verification comes from the successful package scan, not an
exclusion counted as success.

Replay the complete controls with
`make precision-regression ROUND=round-69 REPOSITORY='ozontech/cute basecamp/basecamp-cli' REQUIRE_SCANNABLE=1`.
For urunc, run `CGO_ENABLED=0 GOFLAGS=-mod=readonly GOWORK=off go vet
-vettool=/absolute/path/to/gohawk -enable-all -json ./internal/metrics` in its
pinned checkout. Candidate applications, generators, and tests are not executed.

Focused heap/resource tests, canonical `make verify`, and final architecture
checks pass. This is a focused correction with scoped controls, not a fresh
corpus audit or a cumulative precision replay.
