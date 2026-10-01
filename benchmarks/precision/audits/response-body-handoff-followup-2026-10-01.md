# HTTP Body references in aggregate sends

Correction `bc39b44` removes ACP's reviewed resource false positive from the
[22-site replay](pending-production-fp-replay-2026-10-01.md). Together with the
[ferro destination correction](indirect-destination-followup-2026-10-01.md),
two of those 22 sites now have verified corrections. This follow-up does not
resolve the other 20 sites or change frozen batch labels and queue snapshots.

The pinned [ACP HTTP worker](https://github.com/Contextualist/acp/blob/579b477d0281df41ab8753a7cbcb8f7807e52e2c/pkg/pnet/p2p.go#L79-L91)
loads the response's Body into a local struct, loads that aggregate by value,
and sends it to the caller. The obligation tracks the response object while
containment sees its Body reference. The existing send/select classifier now
recognizes that subresource using `Storage.Projection` at the Body load and
`heapmodel.ContainsAt` at the handoff. Neither the flow query nor shared storage
semantics changed, and no new SSA traversal was added.

The handoff is unknown ownership, never proof of cleanup. Saved original Body
references and aggregate snapshots remain valid after later replacement.
Replaced Body loads, overwritten aggregate fields, unrelated responses, opaque
response mutation, metadata and bytes do not establish the new boundary.
Focused proof tests isolate those distinctions; diagnostic absence alone would
not verify them because other ownership rules can already decline a mutated
response. Resource fixtures retain diagnostics for discarded aggregates and
metadata/byte sends, and accept Body sends and selects. The new send fixture
reports before classification changes and is accepted afterwards.

## Scoped replay

The parent binary implements `d474f09` and has SHA-256
`1570b08871f37eb419b9bfec769e1d9a5eb48c9296473d19166cb6587b2b15cd`.
The corrected binary was built before its commit, implements that production
source, and has SHA-256
`6c54d1d7a22043db52398de1fcb68a692eca7cca6fd0bf911cb61b1fdd453299`.
All scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, and
`go vet -vettool=/absolute/path/to/gohawk -enable-all -json`.
The [ledger](response-body-handoff-followup-2026-10-01.tsv) records exact pins,
keys, scopes, exits, binary hashes and local receipts.

| Repository | Scope | Result |
| --- | --- | --- |
| Contextualist/acp | `./pkg/pnet`, parent and corrected | Both exit 0; the only finding difference is removal of `p2p.go:79:16`. |
| Contextualist/acp | `./pkg/pnet`, corrected with tracing | Exit 0; JSON findings match the ordinary corrected run. |
| ozontech/cute | `./...`, corrected | Exit 0; reviewed production leak retained, with no difference from the previous complete control receipt. |
| basecamp/basecamp-cli | `./...`, corrected | Exit 0; reviewed production leak retained, with no difference from the previous complete control receipt. |

The ACP trace in `.build/goal-body-acp.trace.jsonl` associates the reviewed
acquisition with `response-body-aggregate-handoff`, phase `label`, outcome
`unknown`, at `send t20 <- t26` in `ExchangeConnInfo$1`. This confirms the
positive ownership boundary caused the suppression, rather than a budget
give-up. Request cancellation, channel names and receiver cleanup are not
assumed by the proof. The package scan is not a complete ACP repository scan.
Candidate tests, generators and applications were not executed. No full
precision-regression corpus ran.

The focused analyzer tests, trace assertion, and final `make verify` pass.
The ownership trace assertion is shared with the indirect-destination boundary
instead of duplicating its decoder and phase checks. Final architecture and
replay-ledger checks pass. Beads `gohawk-dho.6` owns this correction; the
consolidation epic and remaining queue stay active.
