# Lock return contracts and final publication

Beads `gohawk-dho.44.11.5.27.3` consolidates the held-on-success and conditional
caller-release queries previously spread across flow, operation and state
files. This stage earns no production FP removal. The eleven recorded production
FP locations and Rune's fresh-lock publication issue remain unresolved.

## Query and publication boundary

`return_contracts.go` owns return contracts and their request allowance.
The setup inventory now retains returns and branches, eliminating repeated
whole-function censuses for acquisition contracts, result polarity and guarded
error returns. Held masks, acquisition dominance, caller dispatch, cycle checks
and coverage share the function pool. Caller coverage uses the existing shared
obligation engine with exact actions, retaining the original held-arm policy
and positive release witness. It does not introduce a second flow algorithm.

Private-lock use classification charges its referrer census. Diagnostic names
and related acquisition evidence reuse the setup's direct-effect map rather
than recomputing it, and each missing-release candidate prepares that metadata
once for all its uncovered returns. Naming still supplies no semantic evidence.

The final decision inspects availability after each contract family. Cutoff is
unknown, never a caller-transfer guarantee or a release violation. The function
publication barrier now runs after final reporting too: an interrupted final
query discards earlier buffered read-lock findings and staged order edges.
The original held-success, error, comma-ok, caller escape, exact global mutex,
private-lock and diagnostic policies remain; rationale links moved with them.

Return retention/merging and returned unlock-owner inference, named-result
storage, mutex identity, heap/type/alias internals, callee ordering and package
caller/exclusivity inventories retain separate costs under parent `.27` and
the broader transitive review. The function allowance is no whole-query
wall-time or memory guarantee. Graph tools were unavailable; discovery and
verification used scoped source reads and searches.

## Controls and validation

`return_contracts_test.go` uses actual SSA for held-on-success, both Boolean
held-result polarities, exact guarded error returns and escaped caller
rejection. Every cold cutoff is followed by a fresh child query, checking that
the parent remains usable and interrupted evidence never proves a contract.
A padded caller isolates coverage charges; final-flow cutoffs retain neither
reports nor order edges, including evidence accumulated before final metadata.
Existing accepted/diagnostic and imported-helper fixtures pass.

Two ignored source overlays fail behaviorally:

| Counterfactual | Failure |
| --- | --- |
| Remove the final publication availability check | The function publishes an earlier read-lock finding and order edge at a final metadata cutoff. |
| Give caller coverage an independent allowance | Padded coverage completes without exhausting its 20-step child. |

Receipts are `.build/goal-lock-returns-overlays/{publication,caller}.log`.
Both are assertion failures, not compilation failures.
`make verify VERIFY_TIMINGS=1` passes ordinary tests, formatting, vet, lint,
deadcode and self-dogfood in `.build/goal-lock-returns-verify.log`.
Focused controls pass in `.build/goal-lock-returns-focused-final.log`.
No full precision-regression corpus or local race test runs.

## Scoped production replay

The immutable `.build/goal-lock-returns-reviewed` implements parent `729704b`
plus this production change; SHA-256
`e1dafbf237f74cf6e65031a693821e1bcf8836f00e8809acb7ff7d4e8b2598ed`.
The clean pinned checkouts use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`,
`GOWORK=off`, a 180-second timeout and
`go vet -vettool=/absolute/binary -enable-all -json`:

| Repository and pin | Scope | Reviewed controls retained |
| --- | --- | --- |
| majestrate/XD, `b905a14ecfeceaa21a5dde82b52f164d075002ab` | `./lib/configparser` | Missing releases at `configparser.go:115:4` and `121:3`. |
| ctdk/goiardi, `937cae400a92d8036b88ae2f65d93506271c292e` | `./datastore ./indexer` | Read-lock writes at `datastore.go:542:5` and `file_index.go:708:4`. |

Both scans exit zero with empty stderr. JSON remains byte-identical to the
[setup parent](lock-setup-inventory-followup-2026-10-02.md): 838 bytes for XD,
1,476 for goiardi. Receipts are `.build/goal-lock-returns-{xd,goiardi}-reviewed.json`
and `.err`. The goiardi scan separately writes 34,673 valid lock trace JSONL
events and leaves diagnostics unchanged. Candidate tests, generators and
applications were not executed in these production checkouts.
