# Lock return retention and callback-origin consolidation

Beads `gohawk-dho.44.11.5.27.4` consolidates lock return retention and callback
capability queries. It earns no production FP removal. The eleven recorded
production FP locations and Rune's fresh-lock publication issue remain open.

## Shared mechanics and bounded witnesses

`return_retention.go` owns held/deferred return recording, independent first
masks, possible/definite merging, return-position deduplication and returned
unlock owners. The function allowance charges mask visits/copies/comparisons,
result/value candidates and owner access-path folds. A returned cleanup
capability or containing owner supplies ownership uncertainty, never proof that
cleanup ran. A cut retention stage cannot publish the function's earlier
findings or order edges.

`lifecycle.ProveValueCallsMethodWithin` projects the existing callback-body
completion engine into one structured may-carry proof. The legacy Boolean
helper delegates to it with nil budget. Returned lock owners and opaque callback
handoffs share the function allowance through this proof. Other default callers
retain their existing nil-budget entry policy.

Callback values now use `ssaflow.ReachingWalk` for transparent wrappers, phi
fan-out and origin recursion, removing the separate callback visited map and
manual phi traversal. Its origin history belongs to the completion request,
including callback mappings reached inside that request. `OnRevisit` lets the
caller invalidate its enclosing memo answer when the fold's guard shortens it;
the hook changes no fold result. Sibling folds retain independent histories.
The original any-origin policy, possible callback-preserving wrapper policy,
stored-value/referrer policy and callee coverage rules remain. Capability does
not imply invocation or cleanup by every possible callback.

The three flow cutoff families now share one assertion helper instead of
duplicating the cold/fresh publication loop. Named-result storage, transitive
identity/heap/type/alias queries, callee ordering and package caller/exclusivity
inventory retain their separate costs. Parent `.27` remains active. These
evidence-step allowances are no whole-query wall-time or memory guarantee.
Graph tools were unavailable; verification used scoped source reads/searches.

## Regression found during validation

The first adapter incorrectly created a new reaching fold for each callback
mapping. The resource helper allowance control failed: mapping successive
scalar call results repeatedly traversed the preceding chain and exhausted its
250,000-step request. A component overlay restoring the parent callback search
passed the same control, confirming this change introduced the regression.

Keeping one fold per completion request restores the original origin scope and
passes `TestHelperCompletionKeepsLargerQueryAllowance`. The revisit hook also
preserves the original rule that a shortened answer cannot enter the completion
memo. The repaired focused receipt is
`.build/goal-lock-retention-repaired-focused.log`, with an earlier isolated
resource control in `.build/goal-lock-retention-resource-fixed.log`.
The initial local gate additionally found duplicate cutoff assertions and a
missing merge rationale; both were corrected. Its recursive-callee wall-time
control also failed during that run; the unchanged control passes in the final
canonical gate. No timeout or evidence allowance was raised.

## Controls

Actual SSA covers direct and literal callbacks, mixed cleanup/no-op phis,
wrappers, interface boxing, stored callbacks, distinct targets, opaque values
and recursive callbacks. Positive capabilities are checked at every cold
cutoff and followed by a fresh child; zero allowance cannot bypass value visits.
Padded callback bodies isolate nested coverage, returned owners and handoffs.

Retention controls distinguish owner/callback returns from retained obligations,
detach incoming masks, preserve union/intersection across paths and reject
partial merges. Cold/fresh full-flow controls discard reports and order edges
after earlier witnesses. Revisit controls cover Any, Every, Mark, Resolve,
sibling propagation and enclosing memo invalidation. Existing fixtures pass.

Six ignored source overlays fail behaviorally:

| Counterfactual | Failure |
| --- | --- |
| Detach callback completion from its allowance | Padded callback completion bypasses its child cutoff. |
| Replace any-origin capability with every-origin coverage | A mixed cleanup/no-op phi loses its possible capability. |
| Detach returned-owner capability queries | A returned padded callback bypasses the function child. |
| Remove merge charges | A one-step child exposes full possible/definite masks. |
| Drop revisit memo invalidation | A shortened negative answer is cached. |
| Reset origin history at each mapping | The existing large-helper resource classification exhausts its allowance. |

Receipts are `.build/goal-lock-retention-overlays/{pool,polarity,owner,merge,memo,scope}-final.log`.
All are assertion failures, not compilation failures.

## Validation and scoped production replay

`make verify VERIFY_TIMINGS=1` passes ordinary tests, formatting, vet, lint,
deadcode and self-dogfood in `.build/goal-lock-retention-verify-final.log`.
The generated helper references include both new shared APIs and the relocated
legacy callback helper. No full precision-regression corpus or local race test
runs. Candidate tests, generators and applications are not executed in the
production checkouts.

The immutable `.build/goal-lock-retention-reviewed-final` implements parent
`e9d340c` plus this production change; SHA-256
`b18106437a295d4aa4d46353f4f1c100be527104c2a1f331cd7ab83440714c01`.
Clean pinned checkouts use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`,
a 180-second timeout and `go vet -vettool=/absolute/binary -enable-all -json`:

| Repository and pin | Scope | Reviewed controls retained |
| --- | --- | --- |
| majestrate/XD, `b905a14ecfeceaa21a5dde82b52f164d075002ab` | `./lib/configparser` | Missing releases at `configparser.go:115:4` and `121:3`. |
| ctdk/goiardi, `937cae400a92d8036b88ae2f65d93506271c292e` | `./datastore ./indexer` | Read-lock writes at `datastore.go:542:5` and `file_index.go:708:4`. |

Both scans exit zero with empty stderr. JSON is byte-identical to the
[return-contract parent](lock-return-contracts-followup-2026-10-02.md): 838 bytes
for XD and 1,476 for goiardi. Receipts are
`.build/goal-lock-retention-{xd,goiardi}-final.json` and `.err`. The separate
goiardi trace has 34,682 valid lock JSONL events; diagnostics remain unchanged.

## Rune reassessment

At clean Rune pin `3e2165f8983280542c985947378dfa740a397d03`,
[LibDir](https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/ide/idepkg/manager.go#L240-L249)
obtains the gate under the registry guard and passes it into
[newPendingIterator](https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/ide/idepkg/manager.go#L2317-L2353).
The returned iterator locks that stored gate in later Next/Err/Close calls.
[finishDownload](https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/ide/idepkg/manager.go#L911-L919)
looks it up and releases it under the registry guard. The installation's fresh
mutex was already stored in the map before its first acquisition.

`ExclusiveAt` establishes confinement before publication; it does not relate
the map readers, guard and later iterator participant. Matching receiver owners
alone cannot establish that guard/field contract. This is a source reassessment,
not a new Rune replay or correction. Beads `gohawk-cnx` retains the publication
issue, with these participant anchors recorded; check enablement remains separate.
