# Replay of the 22 pending production FP sites

All 22 `needs-current-replay` rows in the
[queue refresh](production-fp-queue-2026-10-01.tsv) now have successful affected-
package scans. Every original diagnostic key, including its check ID, remains
reported. The [replay ledger](pending-production-fp-replay-2026-10-01.tsv)
retains the source pins, package scopes, exits, receipts, and Beads items.
No FP removal, new correction, or clean repository scan is credited here.
The earlier queue refresh remains a snapshot of the evidence available then.

The executable implements production correction `d899cfd`; SHA-256:
`ddc506d2665febea8bfdef02d207180561a60d091b5f4a05d092f30594dc5203`.
Scans use `go vet -vettool=/absolute/path/to/gohawk -enable-all -json` with
`CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, and `GOWORK=off`. The package arguments
are in the ledger. One refresh used at most two concurrent scans and a
180-second timeout per scan; no full precision-regression corpus ran.
Candidate tests, generators, and applications were not executed.

The initial FDio attempts used the root module and failed because its examples
belong to the nested `extras` module. The corrected scan from `extras/gomemif`
of `./examples/bridge ./examples/icmp_responder_poll` exited zero and reproduced
both reports. Those successful receipts supersede the root-module attempts;
neither report is excluded as unscannable.

The subsequent [caller-owned destination correction](indirect-destination-followup-2026-10-01.md)
verifies ferro’s statement-storage FP is absent. The 22-site replay above remains
a snapshot taken before that correction.

The subsequent [HTTP Body handoff correction](response-body-handoff-followup-2026-10-01.md)
also verifies ACP’s reviewed FP is absent. Both follow-ups preserve this earlier
replay snapshot and leave 20 of its sites unresolved.

## Bounded next work

Ferro's
[statement preparation](https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/internal/admin/repository/sql_store.go#L73-L99)
builds local structs containing addresses of statement fields on the receiver,
then ranges over the structs and writes through the selected address. The SSA
ends with `t73 = &t56.dest`, `t74 = *t73`, `*t74 = t65`. The current structural
external-ownership query reaches the local copied aggregate rather than its
caller-owned destination. Beads `gohawk-dho.5` tracks shared address-origin
evidence, including local destination, replacement, nil, and opaque controls.
No new analyzer-local value traversal or SQLStore-name exemption is proposed.

ACP's
[HTTP worker](https://github.com/Contextualist/acp/blob/579b477d0281df41ab8753a7cbcb8f7807e52e2c/pkg/pnet/p2p.go#L79-L91)
stores the exact response Body in `readerOrError.ReadCloser`, loads that local
aggregate, and sends it on the result channel. The obligation is represented
by the response object, while aggregate containment sees the Body reference.
Beads `gohawk-dho.6` tracks that exact subresource handoff at an existing
classifier boundary. A send supplies uncertain ownership; it must not prove
cleanup, and arbitrary response-derived bytes must not become owners.
Replacement and unrelated-response controls remain necessary.

Skywalking's
[read cursor update](https://github.com/apache/skywalking-rover/blob/e83d5925500a7e63dd55c080a9b1542d6cedaefb/pkg/tools/buffer/buffer.go#L629-L654)
also needs evidence about which fields a lock protects: its reset and else
paths write the cursor fields without `eventLocker`, while the locked path
reads the event list. The existing ledger's field-association assessment stays
unresolved; this refresh does not invent a protection contract from a shared
receiver alone.

The other families retain their original source reviews and require concrete
policy or lifecycle evidence. Main/process-exit intent and caller-only branch
assumptions do not justify blanket exemptions. Transport shutdown and
registered callback lifecycles need exact participant identity and a documented
contract. `gohawk-dho.4` remains active for those assessments and fixes; the
overall consolidation objective is not complete.

The subsequent [captured worker-field correction](opaque-worker-field-followup-2026-10-01.md)
also verifies Lynx’s shutdown-path FP is absent. Three of this replay’s 22
sites now have verified corrections, leaving 19 unresolved. The original
replay snapshot above is unchanged.

The subsequent [imported async correction](imported-async-followup-2026-10-01.md)
also verifies Viewcore’s profiling-writer FP is absent. Four of the original
22 sites now have verified corrections, leaving 18 unresolved. The original
replay snapshot and earlier follow-up counts remain historical evidence.
