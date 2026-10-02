# Cache entry removal consolidation — 2026-10-02

Beads: `gohawk-dho.46`. Parent: `f757980`.

## Shared removal boundary

Failed graph publication and cache eviction repeated the same stale marking,
matching-entry deletion and LRU removal. They now delegate to one private
`removeIndexedEntryLocked` in the existing graph-cache family. The helper
requires the cache lock and removes only the slot naming that exact entry;
a finishing older build cannot delete a replacement for the same function.

Full eviction is deliberately not reused by failed publication. It also
cleans dependency indexes for a published graph. Publication owns the deferred
build-completion notification under the cache lock, while eviction must not
notify that an in-progress build has finished. The smaller shared operation
preserves both callers' behavior, including repeated publication controls;
no graph construction, generation check, lookup order, dependency registration,
notification order or evidence policy changes.

Graph tools were unavailable. Source fallback inspected the cache entry
creation/graph assignment, both removal callers, dependent invalidation,
generation registry and cache controls. Production entry.graph is assigned
only by publication. Existing tests also republish a ready graph repeatedly;
the extracted boundary preserves that helper behavior without inferring a
stronger lifecycle from production's single-builder convention.

## Regression evidence

Existing cache publication/lookup concurrency controls pass. New controls
exercise an old build evicted before replacement: eviction leaves its done
channel open, failed publication closes it, the replacement cache/LRU slot
survives and the rejected graph is never assigned. Another control registers
a new summary generation after construction's observed version: publication
rejects the build before dependency indexing and still completes its done
notification. These tests exercise cache identity and scheduling boundaries,
not fabricated SSA provenance. No analyzer proof boundary is expanded.

Three temporary counterfactuals fail on assertions and are restored:

- Removing exact entry matching deletes the replacement:
  `.build/goal-cache-removal-replacement-counterfactual.log`.
- Notifying from removal finishes an evicted but still-running build:
  `.build/goal-cache-removal-notification-counterfactual.log`.
- Omitting stale marking loses the changed-summary rejection state:
  `.build/goal-cache-removal-stale-counterfactual.log`.

Focused all-heapmodel tests pass in `.build/goal-cache-removal-focused.log`.
`make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, self-dogfood and ordinary tests including
architecture checks. Receipt `.build/goal-cache-removal-verify.log`.
No full precision replay or local race test ran. Existing CI race coverage
remains responsible for cache concurrency; no concurrency contract is changed.

## Duplication scope and production controls

The same normalized complete-body scan covers 315 production internal files,
2,144 functions and five remaining domain-distinct candidate groups. No changed
cache function appears in a group. Receipt
`.build/goal-cache-removal-duplicates.json`. The production-only dupl scan at
35 tokens drops from 29 reports/13 groups to 27/12; the removed group is the
cache entry copy. Receipt `.build/goal-cache-removal-dupl35.json` and connected
group inventory `.build/goal-cache-removal-dupl35-groups.json`. Other groups
retain the source-backed dispositions from the preceding flow-location audit.
These thresholded scans do not prove absence of smaller, differently shaped,
or excluded duplication. Broader architecture/fact-consumer/cost reviews stay
active; no whole-query bound or complete architecture claim is made.

Immutable binary `.build/goal-cache-removal-reviewed`, SHA-256
`c30d2d011ae4e9d6ac45f4d91a566aa81fbf1c768c8d19473f612385ac5cb898`.
Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab`,
`./lib/configparser`; pinned goiardi `937cae400a92d8036b88ae2f65d93506271c292e`,
`./datastore ./indexer`. Readonly-module, CGO-disabled, workspace-disabled
all-check vet scans use 180-second limits. Both exit zero with empty stderr
and byte-identical JSON against the prior immutable binary (838/1,476 bytes).
Receipts `.build/goal-cache-removal-{xd,goiardi}.json` and `.err`. Missing
releases at configparser.go:115/121 and read-lock writes at
datastore.go:542/file_index.go:708 remain.
Eleven production FP sites plus Rune remain unresolved. No production FP
correction is credited by this mechanical consolidation.
