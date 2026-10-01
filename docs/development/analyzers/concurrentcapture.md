# concurrentcapture design notes

The public page is [concurrentcapture](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

For a straight-line worker, the check now requests ordered synchronization
effects through the shared summary broker and uses the synchronization
region's exact lock state at each mutation. A lock acquired only after the
write, or released before it, no longer suppresses that write. Lock effects
inside a complete helper summary participate in the same order.

This is still a bounded hazard check, not a general race detector. Branching
workers, opaque helper effects, and unsupported mutation sites retain the
older conservative syntax boundary; they are not interpreted as proof of an
unguarded write. Worker-specific guards and channel semaphore shapes are
still handled separately. A lock local to each worker may also suppress a
report even though it does not serialize workers, so the check does not claim
that every accepted capture is race-free.

## Authoritative mutation decision

After selecting a repeated write to a captured outer local, the collector asks
`captureEvidence.proveMutation` for one structured state and reason. That proof
checks ordered lock evidence, the syntax lock fallback, worker-specific guards
and channel guards in the existing order. Suppression remains unknown: none
of those rules proves that the capture is race-free. Reporting and the final
trace consume the same proof; a reportable `capture-unguarded-write` now traces
as rejected rather than accepted.

The trace regression checks exactly one decision per traced candidate and
covers held locks, writes before acquisition or after release, worker and
channel guards, and the branching-worker lock fallback. The accepted
`conditionalLockMayGuard` fixture documents the intentionally uncertain
fallback, even though the condition may leave the write unlocked. This
consolidation changes trace polarity and decision ownership, not diagnostics.
