# Lifecycle checks recall audit, 2026-09-26

Where do `resourcelifetime/missing-release`, `cancellationownership/release`, and
`goroutineownership/unjoined` lose real bugs among the candidates they examine
and stay silent on? This audit covers stages 1 and 2 of the
[recall audit](../../.agents/skills/gohawk-recall-audit/SKILL.md): a census of
decline reasons and a labelled sample of declined candidates. Stage 3, the
fix-commit replay, has not been run. Recall labels are exploratory and feed no
gate.

## Setup

- Binary: gohawk at `cfc2fd8`, SHA-256 prefix `ead49c91bce1cf59`.
- Corpus: 60 repositories drawn with seed 20260926 from the 323 pinned in
  `benchmarks/precision/round-*/repositories.tsv`, listed with their pins in
  [`lifecycle-2026-09-26-corpus.tsv`](lifecycle-2026-09-26-corpus.tsv). All 60
  scanned without error.
- Scan: `scripts/recall-census.py`, which reuses the precision replay's pinned
  checkouts, three shallowest modules, build environment, and partial-load
  retry, and runs
  `go vet -vettool=gohawk -enable=<the three> -gohawk-include-tests -json
  -gohawk-trace=<the three>` per module. It keeps each candidate's `decision`,
  `considered`, and unknown `label` events in the repository's own source.
- Labels: 12 silent candidates per decline reason, drawn with seed 20260926,
  144 in all. Each was read at its pinned revision and labelled missed,
  correctly declined, or out of class. Seven labels were re-checked by hand:
  all three missed labels and four correctly declined ones. All seven held.
  Every label is in [`lifecycle-2026-09-26.tsv`](lifecycle-2026-09-26.tsv).

Repositories with no candidates, kept apart from those that declined:
`resourcelifetime` has none only in nextzhou/workpool. `goroutineownership` has
none in four: SumoLogic/terraform-provider-sumologic, cirruslabs/packer-plugin-tart,
minio/selfupdate, and terraform-redhat/terraform-provider-rhcs.
`cancellationownership` has none in seven: those four minus terraform-provider-rhcs,
plus DereC4/internships-and-newgrad, piotrnar/gocoin, quackduck/devzat, and
uber-go/zap.

## Census

| Check | Candidates | Repositories | Reported | Proven fine | Silent, undecided |
| --- | ---: | ---: | ---: | ---: | ---: |
| `resourcelifetime/missing-release` | 4,054 | 59 | 165 (4.1%) | 3,022 (74.5%) | 867 (21.4%) |
| `cancellationownership/release` | 4,408 | 53 | 7 (0.2%) | 3,578 (81.2%) | 823 (18.7%) |
| `goroutineownership/unjoined` | 4,720 | 56 | 38 (0.8%) | 1,736 (36.8%) | 2,946 (62.4%) |

The undecided resourcelifetime candidates all end as `opaque-consumption`. The
unknown labels on them name the boundary; a candidate can carry several:
`ambiguous-cleanup-value` 215, `aggregate-owner-may-escape` 93,
`nested-in-transferred-argument` 91, `appended` 70, `unsummarized-callee` 52,
`stored-in-map` 42, `interface-method` 41, `dynamic-callee` 35; 254 carry no
unknown label at all. A resource sent on a channel does not reach the top
fifteen boundaries.

Undecided goroutineownership decisions: `no-completion-obligation` 1,790,
`opaque-ownership-transfer` 661, `signal-consumed-by-worker` 258,
`locally-canceled-context` 142, `buffered-completion-signal` 61, and smaller
reasons. All 823 undecided cancellationownership candidates end as
`ambiguous-cancellation-use`.

## Tracing gaps

- `cancellationownership` labels every opaque use `opaque-cancellation-use`,
  and `goroutineownership` labels every one `opaque-use`. Neither names which
  boundary stopped the proof, so their undecided candidates cannot be broken
  down further without new label reasons.
- The 254 resourcelifetime candidates with no unknown label became uncertain
  on a control-flow edge, such as a repeated guard, which is traced as evidence,
  not as a label. The census keeps no evidence events, so it cannot attribute them.

## Missed-bug rates

| Check | Decline reason | Share of candidates | Sampled | Missed |
| --- | --- | ---: | ---: | ---: |
| `resourcelifetime` | `ambiguous-cleanup-value` | 5.3% | 12 | 0 |
| `resourcelifetime` | `aggregate-owner-may-escape` | 2.3% | 12 | 0 |
| `resourcelifetime` | `nested-in-transferred-argument` | 2.2% | 12 | 0 |
| `resourcelifetime` | `appended` | 1.7% | 12 | 0 |
| `resourcelifetime` | `unsummarized-callee` | 1.3% | 12 | 0 |
| `resourcelifetime` | `stored-in-map` | 1.0% | 12 | 0 |
| `resourcelifetime` | `interface-method` | 1.0% | 12 | 0 |
| `resourcelifetime` | no unknown label (edge uncertainty) | 6.3% | 12 | 0 |
| `cancellationownership` | `ambiguous-cancellation-use` | 18.7% | 12 | 0 |
| `goroutineownership` | `no-completion-obligation` | 37.9% | 12 | 1 |
| `goroutineownership` | `opaque-ownership-transfer` | 14.0% | 12 | 1 |
| `goroutineownership` | `signal-consumed-by-worker` | 5.5% | 12 | 1 |

Every one of the 96 sampled resourcelifetime candidates and all 12 sampled
cancellation candidates are sound code: a deferred or explicit release the
proof did not connect, a hand-off to a receiver field or a returned owner, or
a value appended or stored that is not the resource. The undecided buckets
cost proof, not bugs. Widening those models would mostly turn unknown into
proven release, which is consistent with the zero new findings the
local-collection model produced on moby and containerd.

## Missed bugs

1. [kubernaut test/infrastructure/datastorage.go#L456](https://github.com/jordigilh/kubernaut/blob/528b4f7080bf3522c0fa60f1ce87e48dcbcfe4bb/test/infrastructure/datastorage.go#L456)
   and
2. [kubernaut test/infrastructure/apifrontend_e2e.go#L120](https://github.com/jordigilh/kubernaut/blob/528b4f7080bf3522c0fa60f1ce87e48dcbcfe4bb/test/infrastructure/apifrontend_e2e.go#L120):
   N workers each send one result on a buffered channel, and a loop of N
   receives returns on the first error, so the remaining workers run on and
   write to the shared writer after the function has returned. A reusable
   structural rule could prove it: an exact count from
   `ssaflow.ProveCountedLoop` compared with the proven single sends, where an
   exit before the count is reached leaves workers unjoined. Both sites are in
   one repository and share one author's pattern, so the prevalence is unknown.
   The workers finish on their own, because the channel is buffered, so the
   harm is work that outlives the call rather than a leaked goroutine.
3. [lfk internal/k8s/capture_supersede_test.go#L67](https://github.com/janosmiko/lfk/blob/ca9760842190011f31f9d2079425d3a313fdd4c2/internal/k8s/capture_supersede_test.go#L67):
   when `require.NoError` fails, `FailNow` ends the test before `close(done)`
   and `wg.Wait`, and sixteen churner goroutines spin on. This happens only on
   a test-failure path. No reusable rule applies: treating `FailNow` as an exit
   edge would be a library-name guess. It stays an accepted false negative.

## Conclusions

- For the resource and cancellation checks, the declined candidates show no
  measurable recall loss: 0 of 108 sampled were missed bugs. Their losses, if
  any, are candidates the obligation finders never register, which only the
  stage 3 fix-commit replay can see.
- Channel hand-offs of resources are too rare in this corpus to justify an
  ownership model for them.
- Collections, feasibility, and the other undecided boundaries in
  resourcelifetime affect proof completeness and trace clarity, not reported
  bugs, on this sample.
- The only structural recall gap found is goroutineownership's early exit
  from a counted receive loop: 2 of 36 goroutine samples, in one repository.
  It is worth one fixture pair and a check of its precision before any change.
- The two tracing gaps should be closed before the next recall audit of
  cancellation or goroutine ownership.
