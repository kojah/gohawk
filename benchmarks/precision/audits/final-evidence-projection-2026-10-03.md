# Final reportability evidence projection

Bead: `gohawk-dho.23.30`. Parent: `fe06fe7`.

Producer lifecycle and concurrent capture now use `trace.DiagnosticOutcome` for
final reportability evidence. This removes their independent presentation
branches without moving proof policy, report gates or source deduplication.
Producer findings preserve proven/rejected, disproven/accepted and unknown
polarity. Capture mutation proofs currently supply proven or unknown; the shared
projection preserves both, without turning a possible guard into race freedom.

The bounded final-presentation census covers the producer/capture adapters and
existing process, resource, defer and lock consumers of this helper. Cancellation
and goroutine adapters use domain-specific outcomes; instruction/edge labels have
a different proposition and retain their own presentation. This census does not
prove all transitive classifier mechanics are consolidated.

Existing consumer trace fixtures pass, and the shared mapper test covers all
256 evidence enum values. No new test mirrors the removed branches. Complete
all-check parent/current GOPATH fixture payloads preserve 22 producer and 15
capture findings. Four traced receipts preserve 216 producer and 74 capture
events per version, including complete contents and order within each candidate;
traced payloads also match untraced ones. CGO/modules/workspaces are disabled.
Receipts are in `.build/goal-final-projection-scans/`.

Final canonical verification passes all eight gates. The frozen and canonical
binary SHA-256 is `f22c1cbb794d9bd22d642830cfeb9d557ee13d3789df0eceefeff4fb601a2af1`.
Final architecture validation after documentation updates is recorded in the
Bead. Five production sites and broader semantic consolidation remain open.
No full precision replay, local race run or FP credit is part of this cleanup.
