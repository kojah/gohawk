# Bounded completion callee resolution

Beads `gohawk-dho.44.11.5.27.5` consolidates completion launch dispatch and
exact callback resolution into `internal/engine/lifecycle/completion_callees.go`.
The parent is `270f201`. This is shared-engine consolidation, with no credited
production FP removal. Graph tools were unavailable; bounded source searches
covered every caller of resolveCallees, calleesOf, closureCallees and
exactCallbacks, including enclosing cleanup and callback capability queries.

## Evidence boundary

The former exact callback fold and stable-storage queries used nil budgets
inside otherwise budgeted completion requests. They now use the same request
allowance. Launch dispatch and materialization of each callback target also
charge it. The resolution preserves target identity, order, duplicates,
selected transparent forms, all-phi resolution, stable storage and the existing
OnceFunc/deferred, testing Cleanup and WaitGroup.Go contracts. No application
call graph, interface target inference or fact schema is introduced.

Every origin must resolve before any callee set is returned. An opaque,
cyclic, reassigned or cut alternative supplies no partial set. This exact
resolution remains separate from the may-carry callback capability query,
which deliberately accepts any origin. A body is still credited only by the
existing completion coverage and binding engine.

An early dispatch cutoff now reports `budget-exhausted` without local-body
provenance; unavailable code reached with sufficient allowance still reports
`evidence-unavailable`. The prior reason-priority control was updated for this
additional charged work, retaining its observer detail and exact outcome checks.

## Actual SSA and counterfactual controls

Controls cover direct literals, two completing alternatives, a completing/
non-completing merge, an opaque alternative, stable stored callbacks and cells
modified by another deferred callback. Every cold dispatch cutoff must discard
all targets. A fresh child preserves exact target identities, order and launch
metadata; bounded completion agrees with the unbounded baseline for each small
fixture. Parent pools remain available after child cutoff.

The 41-target fixture separately verifies padded origin cutoff, dispatch cutoff
and fresh target resolution. Its full body coverage exceeded both attempted
20,000- and 200,000-step child allowances during initial validation. Those
attempts are not successful recovery receipts; the final control keeps origin
resolution separate from the later coverage cost. No production limit was
raised and no whole-query time or memory bound is claimed.

Initial mutation checks showed that unbounded origin traversal could be masked
by the subsequent target-materialization charge, while a padded prefix cut
before reaching a closure did not test partial publication. Direct origin
controls now exercise both boundaries. Four final overlays fail on assertions:

| Counterfactual | Observed failure |
| --- | --- |
| Detach the origin fold allowance | Two- and 41-alternative origin queries complete through their cutoff. |
| Detach stable-storage allowance | A stored callback resolves through a two-step cutoff. |
| Return accumulated origins after a failed all-origin fold | A cut publishes one callback from the two-alternative merge. |
| Replace exact every-origin resolution with any-origin resolution | Mixed, opaque and padded alternatives expose shortened target sets. |

Receipts: `.build/goal-callee-resolution-overlays/{fold,storage,partial,polarity}-final.log`.
The final focused receipt is `.build/goal-callee-resolution-focused-final.log`.

## Remaining scope

Named-result storage, transitive identity/path/heap/type/alias internals and
package conditional-caller/exclusivity inventories retain separate review work.
Eleven recorded production FP sites and Rune's publication case remain open.
No full precision-regression corpus or local race run belongs to this iteration.


## Scoped production replay

The immutable `.build/goal-callee-resolution-reviewed` implements the parent
plus this production change, SHA-256
`8c30507cc803d0376d94ead5950c5fab88e6cc1c81f5aa268d5d853e6b107b0a`.
Both clean pinned scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`,
`GOWORK=off`, a 180-second timeout and `go vet -vettool=/absolute/binary
-enable-all -json`; candidate tests, generators and applications are not run.

| Repository and pin | Scope | Reviewed controls retained |
| --- | --- | --- |
| majestrate/XD, `b905a14ecfeceaa21a5dde82b52f164d075002ab` | `./lib/configparser` | Missing releases at `configparser.go:115:4` and `121:3`. |
| ctdk/goiardi, `937cae400a92d8036b88ae2f65d93506271c292e` | `./datastore ./indexer` | Read-lock writes at `datastore.go:542:5` and `file_index.go:708:4`. |

Both scans exit zero with empty stderr and JSON byte-identical to the parent
retention receipts: 838 bytes for XD and 1,476 for goiardi. Receipts are
`.build/goal-callee-resolution-{xd,goiardi}.json` and `.err`.

The first canonical gate passed ordinary tests (118 seconds), vet, formatting,
deadcode and self-dogfood. It failed lint on two integer-range suggestions and
one test-source line of 161 columns; those formatting issues were corrected.
The first gate's receipt remains `.build/goal-callee-resolution-verify.log`.

The final canonical `make verify VERIFY_TIMINGS=1` passes all local targets,
including ordinary tests, lint, formatting, vet, deadcode and self-dogfood:
`.build/goal-callee-resolution-verify-final.log`. The ordinary suite reuses
unchanged passing results and completes in eight seconds. No full precision
replay or local race run was performed.
