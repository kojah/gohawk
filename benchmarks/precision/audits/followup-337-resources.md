# Resource follow-up: all 210 original sites

Input: the resource rows of `benchmarks/precision/audits/followup-337-input.tsv`.
Each original acquisition location was reopened from its retained pinned source,
alongside its original concrete source-review rationale. Independent groups
received deeper callee/cleanup inspection and actual SSA/evidence traces before
implementation. Original verdicts are not overwritten. The per-site output is
`.build/followup337-resources.tsv`; each row states its remaining evidence boundary
or its successful scoped replay receipt. Candidate tests, applications and
generators were not executed.

## Implemented bounded changes

1. **Transaction-owned SQL rows.** Finishing the exact transaction cancels active
   rows, including those queried through an exact Tx-prepared statement. Direct
   and dominating deferred Commit/Rollback establish uncertainty about an alleged
   leak, not synchronous Rows.Close. Different transactions, conditional finishes,
   replaced statements and DB/Conn parents keep their independent obligations.
   Three accepted fixtures failed first; accepted and diagnostic controls pass.
   This extends the existing SQL contract/classifier vocabulary without another
   traversal or flow engine. The existing large contracts file remains cohesive:
   these are SQL parent API contracts, not another evidence engine.

2. **Retained aggregate wrappers.** One wrapper around an aggregate positively
   containing the resource may carry it when wrapper effects cannot exclude
   retention. Passing that wrapper to a retaining callee makes ownership unknown,
   not settled. This handles MultiWriter installed as logger output. It does not
   infer ownership from arbitrary data dependence, logger names or call results.
   Known ignored inputs/read-only transformations, discarded wrappers, decoder
   use and scalar observations retain diagnostic controls. Longer slog handler
   and logger construction chains remain unsupported. Initial broad variants
   confused decoded output and response cookies with response-body ownership;
   tracing and negative controls forced narrowing to positive containment.
   Exact direct-resource inputs are also excluded: passing a file straight to
   bufio.Scanner is not the distinct aggregate-wrapper relation. A minimized
   scanner regression failed before this final exclusion and passes afterward.
   Intermediate broad-rule replays are NOT correction evidence. All 104 scopes
   are replayed against the final bounded implementation.

3. **Coordinating-agent fixes.** Exact response/error pairs plus positive helper
   cleanup evidence produce unknown, not unconditional cleanup. Exact nonnil
   filesystem-sentinel equality excludes a successful acquisition. Shared summary
   generation no longer infers every cleanup effect from a panic-only method.
   The latter explained all 11 Sonic reports: a panic-only SetOutboundIPv6 method
   invented a cleanup contract. The idempotent Close guard was secondary, and no
   broad Close-name exemption was added.

## Reassessment by unresolved family

| Family | Sites in original input | Direction and limitation |
|---|---:|---|
| Empty/memory/HEAD HTTP body | 48 | Keep unresolved without exact client, transport, handler, timeout and body-wrapper provenance. HEAD or a status constant alone is insufficient. |
| Fixture prevents acquisition | 26 | Assertions alone prove no success impossibility. Need exact injected failure or hermetic fixture-state semantics; no test-function exemption. |
| Correlated guards/errors | 24 | Shared feasible-path or conditional-result evidence would be required. Do not add loop-count arguments or enumerate application configuration. |
| Process-bounded lifetime | 14 | One-shot process exit differs from reusable helper leakage. No blanket main/example exemption or name-based fatal inference. |
| Returned/retained owners | 12 | Retained lifetimes differ from returned read-only views. Follow exact storage and real cleanup contracts; no automatic ownership based on type/method name. |
| Retained logging output | 12 | Exact aggregate-wrapper boundary implemented; longer wrapper chains and global-map cardinality remain limitations. |
| Paired helper cleanup | 11 | Positive partial cleanup plus exact acquisition/error pair supports unknown; not a universal conditional-summary engine. |
| Vacuous cleanup summaries | 11 | Shared implementation defect; panic-only methods are not evidence of acquiring/releasing fields. |
| Receiver/manager ownership | 10 | Need a retained-result relation: manager ownership and a returned handle can coexist. Do not demand caller Close merely because the result type can Close. |
| Captured-cell cleanup | 9 | Deferred captures observe later contents. Historical aliases and nil guards cannot establish exact completion on their own. |
| Aggregate/table cleanup | 7 | Need exact element and callback mapping; no loop cardinality guesses. |
| Constructor without live resource | 7 | Cleanup-capable type does not prove this constructor returned a live resource. |
| Transaction-parent rows | 6 | Bounded exact-parent classifier boundary implemented. |
| Callback/Once/cleanup-stack | 3 | Registration/invocation/identity remain separate questions; storing a callback is not proof it executes. |
| Invalid URL | 2 | Invalid under pinned transport, but no broad protocol/name exceptions. |
| Infallible in-memory operation | 2 | Requires bounded operation semantics, not suppressing every error return. |
| SSH parent | 2 | Exact child/parent lifecycle evidence required; no generic parent-Close contract. |
| Environmental acquisition | 1 | Review correction to inconclusive: unoccupied localhost:55555 is assumed, not established. |
| Request-context completion | 1 | Need exact request/transport cancellation, not mere context use. |
| Exact error sentinel | 1 | Bounded exact-error relation implemented. |
| SQL query-context completion | 1 | Query cancellation can close rows, but this relation remains unimplemented. |

No check was retired, disabled or demoted. Hard cases remain explicitly visible;
they are not relabeled as fixed because a general-purpose model would be costly.

## Replay provenance

Final disposition: **36 verified absent, 173 retained-needs-proof, one review
correction**. Of the 36 absences, 33 were present in the immutable baseline and
three were already absent (Gitlawb/zero two, regclient one). Those three
are not new fixes. Every one of the 210 sites has a row and a successful final
scope receipt; absence also requires no JSON analyzer error.

The scoped original true-positive control audit covers 72 sites: 67 remain
reported, while five wg-portal pfSense helpers are now suppressed by paired-error
uncertainty. These five original labels are correct: a ReadAll error returns
before Body.Close is deferred. They are not review corrections or fixed bugs;
they are a concrete accepted-false-negative cost of conservative may-cleanup
uncertainty, acknowledged by the parent workflow and recorded in the paired-error
fixture header. The decoder and scanner controls exposed during implementation are
both restored. See `.build/followup337-resources-controls.tsv` for exact sites.
Unstamped scratch labels at `.build/followup337-resources-labels.tsv` contain
36 original false-positive labels and 67 preserved true-positive labels with
their original pinned revisions and reasons. The five suppressed true positives
remain in the controls ledger and original batch labels; they were not relabeled.

- Baseline: c9b65a6, binary SHA256
  `33743c44dfe1f68cbe8dc146989dbe76c0ab02bda55fa8f030a10e32861dfa02`.
- Initial frozen resource binary: `.build/gohawk-followup337-resources-contained`, SHA256
  `28584c84253a04b1b394e45d36896f3b055d954ab275938ac1c0236bc29a4e59`.
  Built from the frozen implementation committed as 5a56fb4.
- Canonical final binary: `.build/gohawk-followup337-final-contained`, SHA256
  `3da9ce166dfd8dc72c3225dc1229298040ae27b04a394c49b2edcf7c3ca772a7`.
- Authoritative scoped receipts: `.build/followup337-resource-canonical/*.json`, one static
  scoped Go package invocation per receipt, with exact checkout revision, binary
  hash, command and environment. Standard error and diagnostic JSON are retained
  separately. All 104 package scopes completed successfully, including a fresh
  wg-portal replay with the same final binary. The initial coordinating-agent
  receipts remain at `.build/followup337-root-replay/receipts.json`, but final
  absence does not rely on their older binary. All 104 were rerun with the
  canonical final binary selecting the exact missing-release check and tests;
  72 scoped original TP controls still have the same 67 retained/five missing
  disposition. There were no further changes to the 210 target positions.
- Initial baseline runner used one package-slug collision for the two auth
  modules; corrected/final receipt names include module identity. No baseline
  absence claim is made for the skipped v2 package.
- Graph generation was checked at start and again after edits; local evidence
  paths have no recorded coverage gap. External `.build` checkouts are excluded
  deliberately and were read directly, not indexed.

Canonical round-58 validation contradicted an earlier resource-only scoped
absence for geesefs core/cfg/logger.go:37:16. It remains a false-positive finding,
but its status is now retained-needs-proof/profile-dependent, not verified absent.
The same frozen binary reports it under vet enable-all and direct invocation but
not vet resource-only; the canonical final binary reports it under both vet
profiles. Selecting only the exact missing-release check still suppresses it
even in the canonical binary. Baseline direct invocation also reports it. The
failure is selection/profile-sensitive, not evidence of a verified fix.
Profile comparison receipts
are in `.build/followup337-geesefs-profiles/receipts.json`. No analyzer rule was
changed to hide this finding, and it was removed from the absence label cohort.

Validation contribution: resourcelifetime TestAnalyzer passes with new safe and
defect fixtures, including decoder and scanner regressions; its final focused
race run passes in 70.054s. Original scoped
true-positive controls are separately listed
in [the retained control ledger](followup-337-resource-controls.tsv). The
[overview](followup-337.md) records final repository-wide validation and
its limitations.
