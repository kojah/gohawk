# Lock-function setup inventory consolidation

Beads `gohawk-dho.44.11.5.27.2` consolidates the repeated lock-function setup
censuses into one inventory. It earns no production FP removal. The eleven
recorded unresolved production locations, Rune's fresh-lock publication issue
and the broader architecture goal remain open.

## Evidence ownership

`setup.go` inventories instructions, synchronous calls, defers and direct mutex
effects once under the function allowance. Acquisition eligibility, earliest
source-position actions for caller-owned locks and possible deferred writer
witnesses consume that inventory. State expansion reuses the direct effects
instead of rebuilding their metadata for every path. The former independent
summary, acquisition and caller-owned census helpers were removed.

Helper inference and effect binding retain one shared 2,000-step summary cap,
drawn from the function pool. Call dispatch, inventory visits, first-action
metadata and deferred-writer dominance also charge the function allowance.
Direct operations and builtins bypass helper inference; builtins remain on
the existing mutation-check path rather than being represented by an empty
helper summary. Opaque wrapper calls can supply only a possible writer witness,
never a lock-order or recursive-lock proof.

A structured setup proof publishes its inventory only after all requested
stages finish. Either function or summary-cap exhaustion supplies unknown with
no setup pointer, including when an earlier helper summary completed. The flow
therefore receives neither partial ownership metadata nor an absence proof.
Fresh queries can recover after interrupted evidence inference.

Nested identity, field-path, heap, alias and type query internals retain their
separate costs. Final return/report/held-contract metadata, callee ordering and
exclusivity remain in parent `gohawk-dho.44.11.5.27`. This is no whole-query
wall-time or memory guarantee. Graph MCP tools were unavailable; verification
used scoped source reads and searches of the lock analyzer and existing APIs.

## Controls

`setup_test.go` uses actual SSA for direct and helper-forwarded acquisition,
caller-owned first releases, empty functions, builtin mutation dispatch and
opaque embedded-lock writer witnesses. Cold cutoffs, child versus parent
availability, same-engine fresh retries and warm zero allowances are covered.
A padded helper isolates both function exhaustion and the independent summary
cap after an earlier completed helper. A separate binding cutoff discards
partial ordered effects and recovers with a fresh allowance. Existing analyzer
fixtures include imported helpers and continue to pass.

Three ignored source overlays fail behaviorally:

| Counterfactual | Failure |
| --- | --- |
| Publish a partial inventory at census cutoff | The cold-zero control rejects proven setup from interrupted evidence. |
| Detach the summary cap from the function pool | A padded helper completes despite exhausting the caller's allowance. |
| Remove effect-binding charges | A one-step child publishes both ordered effects. |

Receipts are `.build/goal-lock-setup-overlays/partial-final.log`, `summary.log`
and `binding.log`. These are assertion failures, not compilation failures.
The first overlay exposed a test gap: the successful branch initially failed
to check interrupted evidence. The strengthened control rejects that branch.

## Scoped production replay

`make verify VERIFY_TIMINGS=1` passes ordinary tests, formatting, vet, lint,
deadcode and self-dogfood; receipt `.build/goal-lock-setup-verify-final.log`.
The initial gate found a missing deferred-writer rationale comment, corrected
before the successful run. The focused documentation/commentary gate also
passes in `.build/goal-lock-setup-docs.log`. A relocated acquisition rationale
was restored afterward; that comment has its own focused architecture check.

The immutable `.build/goal-lock-setup-reviewed` implements parent `de54d5e`
plus this production change; SHA-256
`e8501c75e3078e15e95825948273351fadf867c87aa6d08da6b9388adef445da`.
A later production rationale update and strengthened test do not change the
scan's behavior. The clean pinned checkouts use `CGO_ENABLED=0`,
`GOFLAGS=-mod=readonly`, `GOWORK=off`, a 180-second timeout and
`go vet -vettool=/absolute/binary -enable-all -json`:

| Repository and pin | Scope | Reviewed controls retained |
| --- | --- | --- |
| majestrate/XD, `b905a14ecfeceaa21a5dde82b52f164d075002ab` | `./lib/configparser` | Missing releases at `configparser.go:115:4` and `121:3`. |
| ctdk/goiardi, `937cae400a92d8036b88ae2f65d93506271c292e` | `./datastore ./indexer` | Read-lock writes at `datastore.go:542:5` and `file_index.go:708:4`. |

Both scans exit zero with empty stderr. JSON is byte-identical to the
[release-query parent](lock-release-queries-followup-2026-10-02.md):
838 bytes for XD and 1,476 for goiardi. Receipts are
`.build/goal-lock-setup-{xd,goiardi}-reviewed.json` and `.err`.
The goiardi trace has 34,673 valid JSONL lock events in a separate file;
diagnostic JSON remains unchanged. Candidate tests, generators and applications
were not executed in these production checkouts. No full precision-regression
corpus or local race test runs.
