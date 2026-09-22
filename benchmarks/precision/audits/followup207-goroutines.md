# Goroutine ownership: follow-up of 17 remaining reports

Status: scoped precision replay and repository gates passed; implementation
shipped in `511ed2b`. Later unvalidated experiments were archived, not applied,
at the Claude handoff. Historical ledgers remain unchanged.

The [109-site ledger](followup207-goroutines.tsv) includes all assigned sites,
the existing controls, and every newly exposed finding. Eight assigned false
positives are absent and nine remain.
Remaining reports retain their reviewed false-positive verdict; they are not
accepted permanent noise. No check is disabled, retired, or demoted.

| Site group | Baseline | Final select-v10 |
| --- | ---: | ---: |
| Assigned false positives | 17 reported | 8 absent, 9 reported |
| Previously corrected false positives | 23 absent | 23 absent |
| Existing true-positive controls | 56 reported | 56 reported |
| Newly exposed findings | 13 reported by intermediate select-v4 | 3 true positives reported, 10 false positives absent |

All 66 pinned package scopes parsed without analysis errors. Comparing every
goroutineownership diagnostic in those scopes, the final candidate adds only
the three reviewed true positives. This is a precision/control measurement,
not a recall claim about arbitrary programs.

## Reusable classifier boundaries

- Positive callee-retention summaries connect a captured reader to the exact
  transport cleaned up by the caller (Arrakis and Peirates).
- Exact opposite results of documented `io.Pipe` and `net.Pipe` calls expose
  a potential shutdown participant when handed to a helper after launch
  (Mutagen, two zgrab tests, and GrokCLI).
- A visible returned cleanup literal must capture and close the exact sibling
  resource before it can supply the same evidence (ttrpc).
- A straight-line completion relay can use an independent wait on its exact
  dependency group as an alternative completion handle (Conduit).

These are `unknown`, not guaranteed joins. Unrelated resources, ignored helper
arguments, creation without handoff, ordinary cleanup before launch, and
conditional cleanup remain insufficient. Visible worker sends are excluded:
releasing I/O cannot settle a subsequent abandoned publication. Hidden helper
sends remain a documented coverage gap. Strict join mode keeps its reports.

## Select correction under review

Shared feasibility improvements removed an impossible return that accidentally
kept a Stargz bug control visible. That exposed an existing unsound join:
offering a completion receive in a `select` credited all other cases too.
The candidate places receive evidence on the exact selected CFG edge. Shared
successor blocks cannot borrow one predecessor's receive. Cancellation uses
the same structural decoder, with cancellation-specific policy kept local.

The correction exposed additional real bugs and false positives. None may be
silently accepted to rescue the candidate:

| Newly exposed family | Sites | Current assessment |
| --- | ---: | --- |
| Nonfatal timeout abandons completion send | tusd 1 | True positive |
| Cancellation wins against completion send | gleam 1, gonc 1 | True positives |
| Fatal assertion in timeout arm | Conduit 7 | False positives; strict factory origin is recovered with existing storage and delegated to the shared termination contract; relaxed or mixed origins still report |
| Opaque handler retains the selected canceled context | zgrab HTTP 1 | False positive; exact retention and opaque handoff now yield unknown only on the cancellation edge |
| WaitGroup-to-channel relay | GrokCLI 1, bazel-remote 1 | False positives; one-hop exact group/cancellation or queue-participant evidence yields unknown, not a generic `Wait`+`close` exemption |

Intermediate regressions were investigated rather than hidden:

- zgrab HTTP/2 Ping closes an error channel only on failure. A successful
  unsignaled return means this is not an unconditional completion obligation.
- Indigo closes on one path and sends on the other. Requiring literal close
  on every exit lost its existing true-positive control. The revised boundary
  accepts exact same-channel notification by either close or send; targeted
  replay restores the original report. Standalone send obligations are unchanged.
- Full select-v8 replay lost eleven Badwolf controls: an opaque backend also
  receives a send-capable output channel, ignores cancellation, and can block
  publishing to the abandoned receiver. The candidate now refuses the context
  exemption for that shape regardless of buffer capacity. All eleven reports
  are restored in targeted select-v10 replay while TimeoutHandler stays absent.
- The initial transport-cleanup candidate lost Pterodactyl's completion-send
  control. Visible sends and send-selects now exclude this uncertainty rule;
  targeted and subsequent full replays preserve the control.

Selected-context uncertainty requires positive retention of the exact standard
context passed to an unreadable helper. An unrelated or ignored context does
not qualify; a visible subsequent send still reports. An opaque helper can
ignore cancellation, so this deliberately loses coverage rather than proving
termination. No custom method-name convention is introduced.

Relay evidence is bounded to one basic block, at most 64 instructions, exactly
one Wait followed only by completion closes, loads, and return. An independent
wait on that exact group is an alternative completion handle, correcting the
original Conduit report too. A queue item carrying that exact group or an
existing locally-canceled-worker proof for a same-group participant gives only
unknown; arbitrary participant completion, group counts, and scheduling remain
unmodeled. Extra work, publication, multiple waits, and strict join mode do not
use the relay rule.

The instruction classifier remains cohesive at 422 lines: it is the same
authoritative classification path, including the storage-backed testing API
adapter. Its size trigger was reviewed; no parallel acceptance policy or
duplicate API-name catalog was introduced.

## Remaining nine reports

| Family | Sites | Unresolved evidence |
| --- | ---: | --- |
| Transport shutdown | clawk 2, gonc 1 | The worker sees a caller-provided reader or an inner-round socket while cleanup is outside the current local ownership path. No generic close-name or deadline exemption was added. |
| Transitive completion | Kadeessh 1, k8s-csi-s3 1 | A watchdog's completion and a downstream consumer/error flag relate multiple participants. The exact relay-only rule does not apply. |
| Receiver-stored cancellation | my-geektime 1 | Context stored on a receiver and semaphore lifetime are not established by the local canceled-context proof. |
| Process lifetime | Bitrise 1, driftctl 1 | Process-exit caller context is not yet propagated to these workers; CLI placement does not waive an obligation. |
| Repeated field guard | Ghostferry 1 | Launch and join use a receiver-field condition whose stability across calls is not proved by the local flag rule. |

These remain ongoing work, not a claim that the families are unmodelable.

## Immutable replay receipts

All external work is static analysis only, at the original repository pins.
Canonical profile: direct CLI `-enable-all -gohawk-include-tests -json .`,
`CGO_ENABLED=0`, `GOWORK=off`, `GOFLAGS=-mod=readonly -p=2`,
`GOMAXPROCS=2`, `GOTOOLCHAIN=local`, `PROTO_REPORTER=text`.

- Baseline: `.build/gohawk-followup78-goroutines-v5`; canonical outputs
  `.build/followup78-goroutines-final/` contain 56 true-positive controls,
  23 already-corrected false positives, and these 17 remaining reports.
- Full candidate select-v4:
  SHA-256 `c60d578c5d5979a04fdaded1da7f6e380839aaeac53290660fca2871aa02734e`;
  `.build/followup207-goroutines-select-v4/`: all 66 scopes parsed, no analysis
  errors; 30/40 original false positives absent, 10 retained; 55/56 true
  positives retained, with the Indigo loss explained above. Thirteen new
  goroutine reports require the dispositions in the table.
- Targeted select-v5:
  SHA-256 `222a9afa60ae56aae8f5f6b7cd77eac9a4930da4a50dfdd782b7e2e098fd7b23`;
  `.build/followup207-goroutines-select-v5-targeted/`: three parsed scopes;
  Indigo control restored, zgrab HTTP control retained, TimeoutHandler and
  Ping absent, both original pipe false positives absent.
- Final select-v10:
  SHA-256 `69a465a245e520c168379327a6e64664dffad19ccb5a240026d7e87ed75c956f`;
  `.build/followup207-goroutines-select-v10/`: all 66 scopes parsed, no analysis
  errors, all 56 existing true positives retained, all 23 previous false
  positives remain absent, eight assigned false positives corrected, nine
  retained. Exactly three new true positives remain; all ten newly exposed
  false positives are absent. Three initial targeted scopes reused by the full
  run have the identical binary, pin, command, and scan profile.

Earlier select-v2 crashed on an unrelated string comparison; select-v3 was
stopped after a timeout during the subsequently corrected shared performance
regression. Neither partial run establishes absence or contributes final counts.

Focused goroutineownership, ssaflow, and cancellationownership tests pass with
`-count=1`; focused lint reports zero issues. Earlier architecture passes cover
the changed files; the latest combined run encountered an unrelated in-flight
resource transparency check owned by the parent workflow. Final repository
gates and commit coordination are parent-owned.
The same three changed packages also pass `go test -race -count=1 -p=2`
with `GOMAXPROCS=2`; receipt `.build/followup207-goroutines-race-v10.log`.
The paired cancellation-send fixtures were added after the immutable build;
they change no production behavior and pass uncached package tests.
