# Shared named-result cell census

Beads `gohawk-dho.44.11.5.27.6` consolidates named-result cell recognition.
The parent is `522c557`. Graph tools were unavailable; scoped source searches
covered all production and test callers of NamedResultCellWithin,
resultReadFromWithin and ValueAtReturnWithin, plus result-guard discovery.
No production FP removal is credited by this consolidation.

## One authoritative recognition query

`ssaflow.ProveNamedResultCellsWithin` produces one complete unordered map of
function-owned allocation cells directly read at the same first result slot
on every SSA return. It retains the previous structural policy, including
synthetic result cells and recovery returns; it adds no declaration-name,
reachability, wrapper or stored-value inference. The former single-cell recognition loop and its adapter are removed;
production discovery and fixture setup now use the complete map. No-result
and no-return bodies supply no cells.

Instruction, result and candidate-intersection visits share the allowance.
A cutoff publishes neither positive cells nor a completed empty map.
The proven outcome means the census finished, not that a deferred action
completed. An empty candidate intersection must remain empty on later returns;
no cell can enter because only one later return happens to read it.

Lifecycle guard discovery lazily builds this census when it encounters the
first captured allocation, then reuses the completed map across subsequent
captures and deferred literals in that discovery. Capture ordering and
completion coverage stay with their existing owners. A cut supplies neither
a partial cell map nor a partial guard list. The map is local to discovery;
there is no shared mutable cache or cross-function reuse.

The all-cell census inspects complete return results rather than early-rejecting
one candidate. The repeated-capture consumer performs that census once, with
later queries paying only capture visits. No production allowance
was raised; these are evidence-step controls, not time or memory bounds.

## Actual SSA and regression controls

The source fixtures cover multiple named slots, duplicate direct reads,
slot swaps across returns, a missing read on one return, an opaque converted
result, no-return and no-result bodies, and foreign cells. Every cold cutoff
rejects a partial map and is followed by a fresh child that preserves the
complete baseline map. Slot and empty-map assertions independently check the census policy.

Initial fixtures used defer to force addressable locals; their dumps showed
synthetic result spill cells and recovery returns, which did not exercise the
intended mismatched/direct-read cases. Those fixtures now create unused
capturing closures where needed. The ordinary named-result fixture still
covers deferred recovery returns. No production policy was changed to satisfy
those mistaken source-level expectations.

A repeated-capture lifecycle control completes the initial census, resolves
the next two closures with only their capture-count allowance, and recovers
from a cut cold census with a fresh child. Existing guarded-return and
per-return cleanup controls remain in the focused receipt:
`.build/goal-named-results-focused-final.log` (including SSA dumps).

Five source overlays fail on assertions:

| Counterfactual | Failure |
| --- | --- |
| Publish positive accumulated cells at census cutoff | Named, swapped and missing-read cases expose cut prefix maps. |
| Stop intersecting return agreement | A swapped slot or missing read is accepted. |
| Choose the last duplicate slot | The duplicate-read cell is assigned slot 1 instead of slot 0. |
| Detach census from the allowance | Zero/cold queries complete through cutoff. |
| Recompute the census for each capture | The warm capture-only allowance is exhausted. |

Receipts are `.build/goal-named-results-overlays/{partial,agreement,first-slot,budget,reuse}-final.log`. The positive-prefix variant fails at cuts 14, 12 and
8 with nonempty maps, independently verifying actual partial publication.

## Remaining scope

This closes named-cell discovery review, not all result storage or transitive
identity/path/heap/type/alias work. Package conditional-caller/exclusivity
inventories and the broader semantic/partial-duplication review remain open.
Eleven recorded production FP locations and Rune's publication case are
unresolved. No full precision-regression corpus or local race run is performed.


The initial local gate found a now-unreachable single-cell adapter and a nested
signature branch. The adapter and its redundant test are removed, fixture setup
uses the shared census, and the signature check returns early. Those corrections
are part of the final change; the initial receipt is
`.build/goal-named-results-verify.log`.


## Scoped production replay

The immutable `.build/goal-named-results-reviewed-final` implements the parent
plus the final production change, SHA-256
`77e7d4411aca53c49dbdff61cb35489ca614b66e4e7584153c78ceab0d141b2e`.
The earlier `goal-named-results-reviewed` build predates removal of the obsolete
adapter and is not the reviewed replay binary.

Clean pinned checkouts use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`,
`GOWORK=off`, a 180-second timeout and `go vet -vettool=/absolute/binary
-enable-all -json`. No candidate tests, generators or applications execute.

| Repository and pin | Scope | Receipt |
| --- | --- | --- |
| majestrate/XD, `b905a14ecfeceaa21a5dde82b52f164d075002ab` | `./lib/configparser` | Both reviewed missing releases remain; 838 bytes, two JSON documents and two diagnostics, identical to the parent. |
| ctdk/goiardi, `937cae400a92d8036b88ae2f65d93506271c292e` | `./datastore ./indexer` | Both reviewed read-lock writes remain; 1,476 bytes, two JSON documents and two diagnostics, identical to the parent. |
| ctdk/goiardi, same pin | `./shovey` | Parent/current output identical: 8,433 bytes, one JSON document and nine diagnostics. The two recorded transaction FPs at `sql_funcs.go:460:13` and `492:13` remain. Other findings are not newly labelled by this replay. |

All four scans exit zero with empty stderr. Parent transaction output uses
`.build/goal-callee-resolution-reviewed`, SHA-256
`8c30507cc803d0376d94ead5950c5fab88e6cc1c81f5aa268d5d853e6b107b0a`.
Receipts are `.build/goal-named-results-{xd,goiardi,shovey,shovey-parent}.json`
and `.err`. Byte equality and complete JSON decoding validate the comparisons.
This scoped replay earns no FP correction or corpus-wide precision claim.


## Final local validation

`make verify VERIFY_TIMINGS=1` passes generation, formatting, module validation,
vet, deadcode, lint (zero issues), self-dogfood and the ordinary suite (85
seconds). The final receipt is `.build/goal-named-results-verify-final.log`.
Generated helper references include the new proof and remove the obsolete API.
No full precision-regression corpus or local race run was performed.
