# Batch 49 precision corrections

The original batch-49 scan and verdicts remain unchanged. Follow-up source
review, SSA/evidence traces, minimized fixtures, and package-scoped replays
verify corrections for **25 of the 29 false positives**, with all six sampled
neighboring true positives retained. Round 52 pins 24 corrected FP labels and
six TP controls; mpb is separately verified as described below.
This is not a claim about recall or preservation of every finding in the batch.
The analyzer changes are committed as `c0250a6`.

## Corrected boundaries

- **Compression readers:** removed gzip/zlib reader cleanup obligations and the
  `require-reader-close` option. Reader Close owns no input descriptor and does
  not finalize or validate the stream. A regression control keeps an unclosed
  underlying file reportable. Compression writers remain tracked.
- **Abandoned compression output:** a potentially failing error return or an
  explicit pipe abort leaves finalization uncertain rather than proving a leak.
  Possible identity suffices for that uncertainty across captured pipe loads;
  it does not prove closure. Successful output and an unrelated aborted pipe
  remain diagnostic controls. This deliberately loses coverage where a caller
  publishes failed output despite the error.
- **Optional mutex fields:** repeated nil-guarded loads are not a stable value
  proof. Decline missing-release for these uncertain acquisitions rather than
  manufacture a feasible unreleased path. This is not a general predicate or
  mutation engine and can miss a genuine leak with a changed optional field.
  Direct pointers and Boolean-guard controls remain checked. Returning the
  object containing the held lock likewise exposes uncertain caller ownership;
  no method-name convention proves that the caller unlocks it.
- **Returned collections and callbacks:** a separate aggregate argument can
  carry ownership even when another argument directly borrows the resource.
  Imported retention or an opaque callback consumer makes captured ownership
  unknown. Visible non-retaining observers and known testing-cleanup coverage
  remain subject to the original proof. No monkey-patching framework exemption
  was added.
- **SQL exhaustion:** the false edge of the exact tracked SQL Rows.Next is
  unknown because final exhaustion closes rows, but another result set can
  remain. Early breaks and Scan-error exits remain checked. Replay initially
  exposed an existing unsound transfer: Rows.Scan's imported Stored summary
  describes receiver-local state, not transfer of the caller's ownership.
  Receiver self-retention no longer discharges that obligation. All three yarr
  true-positive controls survive the corrected replay.
- **Worker completion:** looped Done calls are uncertain without counter
  arithmetic; captured collection roots are normalized for existing uncertain
  joins; opaque producers may lend registry-owned groups; and deferred helper
  completion plus storage-resolved returned owners recognizes the mpb pattern.
  No collection-count engine, registry guarantee, or project exemption was added.

## Remaining false positives

These four remain unresolved and are excluded from the passing FP cohort:

| Finding | Missing evidence | Decision |
| --- | --- | --- |
| Cerbos exporter selection | Repeated pure suffix predicates must correlate acquisition with the returned exporter | Defer; do not equate arbitrary repeated calls or add suffix-function exceptions |
| yarr migration rows | Parent transaction cleanup is reached through a named callback in a global function slice | Defer; lexical enclosing-completion evidence cannot prove this dispatch, and borrowed transactions are not universally harmless |
| go-drive canceled OAuth request | Failure depends on token state and a custom multi-package transport chain | Defer; custom transports can ignore cancellation |
| Terway logger file | MultiWriter's input collection must be related to its returned writer before Output's retention fact applies | Defer; a MultiWriter-specific suppression would hide rather than prove that relationship |

The proposed existing-evidence reuse was investigated for every family. These
remaining cases require evidence beyond the bounded changes above; none is
marked fixed merely because the relevant code is plausibly safe.

## Validation and provenance

- Focused analyzer tests include accepted forms and nearby diagnostic controls.
- `make verify` ran in an isolated copy of the working tree, avoiding unrelated
  downloaded audit-source templates beneath the main checkout's `.build`.
  This includes ordinary tests, lint, architecture checks, generated-doc checks,
  self-analysis, and the existing trace-package race tier.
- Package-scoped replay covered all 29 original FP locations and six TP controls.
  The first corrected snapshot hash was
  `0168341ffb38dbb81b42c2708c4e13e53f4340020108c85f78e6c26fa6ad37dc`;
  the captured-pipe follow-up used
  `03cec370ebb0415c1d7ccf82323c11abf2d9c282c5aeb80517ff68bdb3571fee`.
- Cerbos's unrelated test-package loading error was not counted as a clean scan.
  Its two util labels were verified by analyzing the package's production Go
  files, selected with `go list`, without executing any candidate code.
- Round 51's existing SQL cohort retained four FP suppressions and its TP.
  Its historical baseline also lost the seven already-retired exitpolicy rows.
- The first round-52 run passed 30 labels but could not check mpb because its
  unrelated example modules lack dependency sums. Its label was excluded from
  the automated cohort rather than count a skipped result as a pass. The pinned
  root package and local accepted/diagnostic fixture verify this correction;
  the cohort runner needs module selection before that label can join the gate.
- The final `make precision-regression ROUND=round-52 REQUIRE_SCANNABLE=1`
  passed all 30 labels: 24 FP suppressions and six TP controls. No new findings
  appeared relative to the retained historical baseline.

No candidate tests, generators, or applications were executed. The original
batch scanner binary was never replaced, and no outreach or GitHub comments
were made.
