# Batch 53: source review and unresolved evidence boundaries

All 250 pinned repositories were attempted with the unchanged `c9609c6`
baseline: 201 completed, 49 were incomplete, and 119 complete scans had no
findings. All 386 original findings have individual source reviews: 322 true
positives, 48 false positives and 16 inconclusive. Incomplete scans are not clean.
These are source judgments, not runtime reproductions or a measurement of recall.

This is the second of four additional 250-repository batches. The selection
expanded the earlier Go 1.26+ convention after exhausting that fresh pool;
this batch declares Go 1.25. Prior audits, cohorts, outreach and the other
in-flight batches were excluded. Companion selection and scan metadata preserve
the full pins, search provenance, binary/runner hashes, module results and errors.
Candidate tests, generators and applications were never executed.

## Check-level reassessment

All 48 false positives remain explicitly unresolved in this batch. No check was
disabled, retired or demoted, and no passing correction is inferred from a
proposed model. The per-finding ledger is authoritative for positions and pins.

- **Captured cleanup cells (six comqtt sites):** the deferred closure is
  registered before acquisition and observes a response cell written by three
  mutually exclusive branches. All actual returns close the selected response.
  SSA and evidence tracing confirm this is storage provenance, not merely a
  merged argument. Broadly treating any captured response as released would hide
  the genuine overwritten-response defect in batch 52's vk-turn-proxy. Retain
  the check; a shared future-observation contract would need exact cell writes,
  exclusivity and no later replacement. That is beyond a bounded classifier
  adjustment and remains unresolved rather than a new local storage engine.
- **No acquired HTTP body:** exact HEAD requests and local empty test responses
  account for repeated false positives in OpenSCA, Bazel Remote, go-chromecast
  and go-pkgz/auth. Review checked transport and Client.Timeout: an empty body
  with a timeout wrapper can still own cancellation cleanup. Sentry's schemeless
  URL cannot successfully acquire a response. A generic exemption for HEAD,
  status codes or tests would be unsound with custom transports and timers.
  Reuse exact acquisition contracts if expanded; do not infer semantics from
  names or replace the positive controls with blanket HTTP suppression.
- **Infeasible errors and correlated ownership:** fixed valid template parsing,
  zlib output to bytes.Buffer, a fixed header fitting a fresh buffered writer,
  helper error predicates, and paired Boolean/sentinel lock guards cause false
  alerts. Gophercloud's retry bound makes its alleged lock-error path unreachable.
  The remaining rules require domain-specific failure or relational path facts;
  matching a possible cleanup somewhere is insufficient. Keep unresolved, with
  real copy/encode failures and feasible lock omissions retained as controls.
- **Retained resource owners:** Pretender returns its logger even on error and
  callers close it; Uniqush retains the writer in returned loggers; Kube Burner
  installs a MultiWriter in the global logger. Reuse exact retained-owner facts
  rather than logging-name exemptions. Callback, returned-error-owner and nested
  aggregate shapes still require evidence not established by the current model.
- **Indirect worker completion:** Rill joins through drained merged output;
  cdebug uses the exact progress cancellation lifecycle; ttrpc and gonc close
  exact transports; zkep workers have cancellation-bound I/O and matched tokens.
  Those source-proven lifecycles do not imply arbitrary connection closure or
  cancellation always joins a worker. Skirk and gonc mux cases retain uncertainty
  because extra acknowledgement and buffering paths were not fully resolved.
- **Process completion:** Sonar's failed-start fallback merges command identity,
  its Linux adoption helper cannot fail, gonc guards the exact started command,
  and go-shirei reaps via Process.Wait in test cleanup. The latter is not a
  blanket substitute for Cmd.Wait when pipe-copy workers or context watchers
  also need completion. Retain these checks; helper/error correlation and exact
  subprocess obligations need bounded shared facts before broadening acceptance.
- **Producer cardinality:** all four go-metrics reports assume repeated sends
  from a range that actually contains at most one or zero entries. The evidence
  comes from exact collection updates and aggregation, not syntax alone.
  A general cardinality proof would be materially broader; leave unresolved
  rather than add loop counts or collection-name special cases.

## Inconclusive cases and controls

The 16 inconclusive rows preserve their specific uncertainty: process-scoped
services/terminals, locally canceled streams, indirect shutdown, very unlikely
cryptographic identifier collisions, timeout scheduling, and a fresh pipe Close
failure whose feasibility was not established. They are not counted as fixes
or true positives. Missing explicit Close is distinguished from proof of a
persistent leak; ordinary bounded file-cleanup policy findings are described as
such in their individual reviews.

True-positive controls include body-bearing HTTP responses, actual error paths
before Close/Wait, database pools abandoned by tests, shared writes, lock
hazards, and senders whose result channels remain unbuffered after cancellation.
The longer lock-cycle reports establish an ordering hazard, not a runtime proof
that all participants can deadlock simultaneously.

## Verification

The ledger assembler checks every original finding key, check ID, revision,
checkout HEAD, source file, line and byte column. All 386 findings are covered
exactly once. The frozen binary is
`5b98c2f24b8b5b3b229c2a18bf0728a23fdec1eda891e74bb66c18bc3b3e009e`.
Later fixes in other batches do not rewrite these baseline verdicts.
