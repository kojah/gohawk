# Lifecycle checks recall audit, 2026-09-26

Where do `resourcelifetime/missing-release`, `cancellationownership/release`, and
`goroutineownership/unjoined` lose real bugs among the candidates they examine
and stay silent on? This audit covers all three stages of the
[recall audit](../../.agents/skills/gohawk-recall-audit/SKILL.md): a census of
decline reasons, a labelled sample of declined candidates, and a replay of
real leak fixes. Recall labels are exploratory and feed no gate.

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

## Stage 3: leak fixes

`scripts/mine-fix-commits.py` searched GitHub commit messages for leak
symptoms, never the missing call ("fix fd leak", "too many open files", "fix
goroutine leak", "leaking goroutine", "fix context leak", and similar), kept
commits that change Go, and replayed each check on the fix's parent over the
touched packages, with test files included, using the same binary as the
census. Forks and mirrors of one commit count once. Each commit was labelled
in-class, other-leak, or not-a-leak, and a report counts as a hit only when it
names the defect the fix removed. All labels are in
[`lifecycle-2026-09-26-fixes.tsv`](lifecycle-2026-09-26-fixes.tsv).

| Check | Commits analysed | Unbuildable | In class | Other leak | Not a leak | In class and reported |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `resourcelifetime/missing-release` | 15 | 2 | 6 | 7 | 2 | 1 |
| `cancellationownership/release` | 13 | 2 | 9 | 0 | 4 | 5 |
| `goroutineownership/unjoined` | 50 | 31 | 6 | 29 | 15 | 1 |

Every report on an in-class commit named the fixed defect; the reports on
other commits (bounce, zeropod, istio, zongzi, go-service-template-rest) were
at unrelated sites. The samples are small, so the rates are indicative:
cancellation 5 of 9, resource 1 of 6, goroutine 1 of 6.

Why each in-class miss was silent:

- Resource (5 misses), all unregistered acquisitions, none declined:
  `os.Pipe` is absent from the acquisition table
  ([flar](https://github.com/swelljoe/flar/commit/6bf46571087beaa9be5f873d313b108efff35835));
  `fs.FS.Open`, an interface method whose files do not always need closing
  ([satisficer](https://github.com/fivethirty/satisficer/commit/a1188bc20c5c35328ec2b6494238b3b84c41af57));
  `golang.org/x/crypto/ssh.Dial`, a third-party API outside the standard
  library policy
  ([juju](https://github.com/juju/juju/commit/b20e2e2be4ff5fa2f1befb6bc36aaafa9a695369));
  and resources returned by project helpers, which count as acquisitions only
  under the narrow trusted-result rule
  ([reostream](https://github.com/VoltTech21/reostream/commit/dd4d84568ec808209af8a88b18521336c5766ed1),
  [dcs](https://github.com/Debian/dcs/commit/f488687aaeea17f7d57ec6d5c67196b8707c6960)).
- Cancellation (4 misses): twice the cancel function is captured by a
  closure, so SSA stores it into a heap cell and that store is labelled
  `stored` (unknown). In grpc-go the deferred closure then cancels only when
  the named error is non-nil
  ([grpc-go](https://github.com/grpc/grpc-go/commit/db35da8bc5e8dcfcb57b94e9be0fba306710cc77),
  [go-test](https://github.com/mvrahden/go-test/commit/2033b10e82d90215936475d31f503a9e0525231b)).
  Twice the constructor was not registered: a project wrapper around
  `WithCancel`
  ([go-transaction-manager](https://github.com/avito-tech/go-transaction-manager/commit/3b4ecd00a188d6a47e9f6fdedb1b458dfbf9d013)),
  and a constructor in a package the replay did not trace
  ([hermesx](https://github.com/Colin4k1024/hermesx/commit/8f73c6ac5dec8c1a3f04da41808698b18061d60c)).
- Goroutine (5 misses): two were accepted as joined because a deferred
  `Close` of an owner the worker reads was taken as the join
  (`deferred-join-before-spawn`), while in Goauld's relay the other worker
  blocks on a channel send that closing cannot release
  ([go-subprocess](https://github.com/kohkimakimoto/go-subprocess/commit/f72ce54bcf833ccdac3736f1140e8445288208ef),
  [Goauld relay](https://github.com/Hazegard/Goauld/commit/b45ba31ba19e90fc2ea41dcf3d09864d9bc36786)).
  One is the counted receive loop that returns on the first error, the
  pattern stage 2 found in kubernaut, now in a second, unrelated repository
  ([ethermint](https://github.com/blockintelligence/ethermint/commit/12674a5e223e58fdd8bbf5a165aca6448937bba1)).
  One has no completion signal at all (`no-completion-obligation`,
  [Goauld execute](https://github.com/Hazegard/Goauld/commit/cf423ea758275051dde10f924fbe7bf87b11f3c9)),
  and one returns on a timeout before its signal
  ([m2node](https://github.com/clavin-dev/m2node/commit/edbd6d6ef0deac3099a9ce03d6ace69b18c0db35)).

The goroutine-leak commits were also replayed with every `producerlifecycle`
check (`abandoned-send`, `unclosed-range`, `stopped-loop-send`), which target
producers blocked on a send. It was silent on all 35 real leaks: its count
proof compares sends with receive sites, so a receiver that returns early on
one path, as the timeout `select` does, is within the count.

Prevalence: most real goroutine leaks fixed in this sample are not the
unjoined-return shape. Of 35 real goroutine leaks, 29 are goroutines blocked
forever on a channel or a long-lived owner's lifetime, which
`goroutineownership/unjoined` does not target.

## Conclusions

- For the resource and cancellation checks, the declined candidates show no
  measurable recall loss: 0 of 108 sampled were missed bugs. Stage 3 confirms
  where the losses are instead: resource leaks are missed because the
  acquisition is never registered (5 of 5 misses), not because a candidate is
  declined.
- Channel hand-offs of resources are too rare in this corpus to justify an
  ownership model for them.
- Collections, feasibility, and the other undecided boundaries in
  resourcelifetime affect proof completeness and trace clarity, not reported
  bugs, on this sample.
- goroutineownership's early exit from a counted receive loop now appears in
  two unrelated repositories (kubernaut in stage 2, ethermint in stage 3). It
  is worth a fixture pair and a precision check before any change.
- Adding `os.Pipe` to the acquisition table is a cheap, standard-library-only
  recall fix. The other unregistered acquisitions need a policy decision:
  `fs.FS.Open`, third-party constructors, and project helpers.
- Cancel functions captured by a closure are the main cancellation decline
  worth modelling: a cell written once with the cancel function, then read by
  closures, as the lifecycle facts already resolve for Boolean flags.
- The largest goroutine leak family, a goroutine blocked forever on a
  channel, is outside every current check. That is a prevalence finding about
  what to build next, not a recall loss of an existing check.
- The two tracing gaps have since been closed (`bdc50b1`): every lifecycle
  label now names its rule, and resourcelifetime labels the edges that make a
  path unknown.
