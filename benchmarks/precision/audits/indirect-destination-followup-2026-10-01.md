# Caller-owned destination copies

Correction `d474f09` removes ferro's statement-storage false positive from the
[22-site replay](pending-production-fp-replay-2026-10-01.md). The original batch
labels and earlier queue snapshots are unchanged. Of those 22 replayed sites,
this follow-up verifies one correction; the other 21 remain unresolved by it.

The pinned [statement preparation](https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/internal/admin/repository/sql_store.go#L73-L99)
copies structs containing addresses of receiver-owned statement fields before
storing the acquired statement through the selected address. The local table
is not the destination owner. Bounded selected struct snapshots now preserve
possible reference destinations across known array/slice windows. The same
field enumeration is used by heap summary projection. Exclusivity permits
several exact fields of one object; exact storage identity still requires one
slot. Resource classification consumes one structured destination proof.

Minimized resource fixtures fail before the classifier correction for the
caller table and opaque destination; local and replaced destinations already
report. They all pass after the correction. Shared heap tests distinguish
caller/local tables, replacements, nested fields and slice offsets. Mixed,
nil, opaque, dynamically replaced, unknown-window and oversized-window forms
remain unknown. The dynamic replacement control caught a descendant wildcard
write that could otherwise reuse old fields; such writes now retain the
clobbered snapshot. Unknown consumption suppresses a diagnostic, never proves
cleanup. No SQL type or project-name exemption is used.

## Scoped replay

The immutable corrected binary was built before the commit, implements its
production source, and has SHA-256
`1570b08871f37eb419b9bfec769e1d9a5eb48c9296473d19166cb6587b2b15cd`.
All scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, and
`go vet -vettool=/absolute/path/to/gohawk -enable-all -json`.
The [ledger](indirect-destination-followup-2026-10-01.tsv) records exact pins,
keys, scopes, terminal exits and local receipts.

| Repository | Scope | Result |
| --- | --- | --- |
| ferro-labs/ai-gateway | `./internal/admin/repository ./mcp` | Exit 0; only the reviewed statement-storage finding disappears compared with the earlier replay receipt. |
| ozontech/cute | `./...` | Exit 0; reviewed production resource leak remains, with no finding difference from its previous control receipt. |
| basecamp/basecamp-cli | `./...` | Exit 0; reviewed production resource leak remains, with no finding difference from its previous control receipt. |

Ferro's process report at `mcp/stdio.go:154:12` and its previously unreviewed
worker finding remain unchanged. This is affected-package verification for
ferro, not a complete repository scan. Candidate tests, generators and
applications were not executed. No full precision-regression corpus ran.

Focused heap, resource, lock-order and lifecycle-fact tests, including the
unknown-destination trace assertion, pass. Final `make verify` passes all local
gates. Beads `gohawk-dho.5` owns this correction; the overall consolidation epic
and the remaining queue stay active.
