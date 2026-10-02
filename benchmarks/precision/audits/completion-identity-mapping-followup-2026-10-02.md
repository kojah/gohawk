# Completion identity mapping follow-up — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.9`. Parent: `62c37f4`.

## Decision and scope

Argument mapping and receiver matching now use the existing bounded identity,
derivation, access-path, containment and stored-path queries. The static
storage-owner root walk charges selections and path questions to the request.
The strict projection implementation is unchanged but exposes a structured
proof through `ProveStrictProjectionPathWithin`: the existing QueryBudget child
cap remains, and its cutoff is unknown even with an available parent. A stopped
strict projection or stored-path query cannot fall through into a broader
mapping. Exact-target handling, possible-alias exclusion, mapping priority and
sibling-field rejection retain their original meanings.

Two default adapters became unreachable: `heapmodel.StoredPath` and
`ssaflow.ProveIdentity`. They are removed; tests and maintained references use
the authoritative bounded queries with nil when the default behavior is wanted.
There is no parallel inference engine or new ownership policy.

Graph tools were unavailable. Source review covered completion mapping/search,
heap projection/access-path/derivation and shared SSA identity/path queries.
Parameter spill-path extraction, exact wrapper cleanup receivers, deferred
storage referrer/path censuses, and graph/alias/type internals remain separate
costs. This is neither a whole-query bound nor a complete architecture audit.

## Controls and corrections during validation

Actual SSA controls cover a nested field path, dynamic index, unrelated root,
a path deeper than the original child cap, and fresh shallow recovery on the
same parent. Static storage-owner controls sweep allowance through stable,
reassigned and nested owners. Existing mapping, deferred owner, sibling-field,
callback, resource and concurrency fixtures continue to govern the consumers.

Focused heap and lifecycle controls pass in
`.build/goal-completion-identity-controls.log`. Removing the shared allowance
fails the one-step caller control; inflating the original child cap fails the
deep-path unknown control. Both overlays fail on assertions, not compilation:
`.build/goal-completion-identity-{detached,uncapped}.log`.

The original padded callback recovery exhausted its 2,000-step test allowance
once numeric capture derivation was charged. Recovery now succeeds with 4,000
steps in the callback capability and both lock retention/handoff controls.
The sixty repetitions and thirty-step cutoff assertions are unchanged; no
production allowance was raised. The initial exhaustion receipt is
`.build/goal-completion-identity-nested.log`; the final lock recovery receipt
is `.build/goal-completion-identity-lock-recovery-final.log`.

SSA dumps for the shallow and 1,001-child paths succeed with empty stderr:
`.build/goal-completion-identity-{field,deep}.ssa` and `.err`. These confirm
actual field/load chains, rather than a simulated IR.

The initial canonical gate finds the obsolete adapters and padded lock recovery.
The next gate finds the same compiled recovery test plus one overlong line
introduced by the API rename. Both failures were corrected, and their receipts
are retained as failed runs: `.build/goal-completion-identity-verify.log` and
`.build/goal-completion-identity-verify-final.log`.

## Production checks and remaining work

Intermediate cleanup binary `.build/goal-completion-identity-reviewed-final`, SHA-256
`63267e926fa82eb8f725ee79da4af675212350b0e4dd7c1c219144a829488a2b`.
The initial binary and receipts remain separate artifacts.

Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab` scope
`./lib/configparser` and goiardi `937cae400a92d8036b88ae2f65d93506271c292e`
scopes `./datastore ./indexer` use readonly modules, CGO disabled, workspace
disabled, 180-second limits, and all-check vet JSON. Four reviewed controls
are missing releases at configparser.go:115/121 and read-lock writes at
datastore.go:542/file_index.go:708. Initial current JSON is byte-identical to
the preceding binding-mapping receipts (838 and 1,476 bytes), empty stderr.
Final receipts use `.build/goal-completion-identity-{xd,goiardi}-final`.

Reassessment of the remaining FP source families found no small justified
classifier correction: guarded fresh-lock publication, dynamic participant
protocols, constructor state and caller preconditions require distinct evidence.
Eleven recorded production FP sites and Rune remain unresolved; no correction
is credited. No full precision replay or local race run was performed.

## Independent child-cut memo correction

The clean intermediate gate passed in
`.build/goal-completion-identity-verify-clean.log` (ordinary tests 15 seconds).
Final source review then found that a strict-projection or stored-path child
cut could leave the request available and let the enclosing completion memo
retain the shortened answer. Both boundaries now call `memo.Incomplete()` as
well as preserving the unknown result. `TestMappingChildCutoffInvalidatesMemo`
uses the actual 1,001-selection argument and verifies two cold attempts on the
same memo key with an available parent. The current control passes in
`.build/goal-completion-identity-memo.log`; removing invalidation fails on the
cached-answer assertion in `.build/goal-completion-identity-cached-cut.log`.

Final immutable binary `.build/goal-completion-identity-reviewed-memo`, SHA-256
`67cfdf58f34d5846d24a8a931bac532c07b3f1a2201fc5918bedcf550de6dadd`.
Final production receipts are
`.build/goal-completion-identity-{xd,goiardi}-memo.json` and `.err`.
The prior final-suffix receipts belong to the intermediate cleanup binary and
remain distinct. The final canonical receipt is
`.build/goal-completion-identity-verify-memo.log`.

The final memo binary's pinned scans both exit zero with empty stderr and
byte-identical JSON to the preceding binding-mapping receipts. The final
`make verify VERIFY_TIMINGS=1` passes every target, including ordinary tests,
architecture checks, generation, module verification, formatting, vet, deadcode,
lint and local all-check dogfood. No full precision replay ran.
