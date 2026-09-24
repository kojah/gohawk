---
title: "Heap evidence loss: pinned sample, 2026-09-24"
description: Pinned measurements of where cached heap evidence is lost.
# Internal measurement notes, kept off the published site.
draft: true
---

This measures evidence boundaries, **not missed bugs**. Both scoped scans
completed with exit status 0, empty stderr, and valid diagnostic JSON `{}`.
Only `resourcelifetime` and `nilargument` were enabled. Test source contributed
summaries, but reporting from tests was not enabled. These are package-family
samples, not whole-repository audits.

| Scope | Pinned revision | Wall time |
| --- | --- | --- |
| Moby `./daemon/logger/...` | `3f673306102e01c16b5e0ab2343588bfab2fc4e7` | 1m48.72s |
| Kubernetes `./pkg/controller/garbagecollector/...` | `e72c2715ade37738aa5c029e8de5285cbe1c9441` | 2m55.66s |

Binary SHA-256:
`8853fa0903ee89ed46219e98ae74fca86f56c8197e90a0db561e54079843028d`.
Moby used Go 1.26.6; Kubernetes used Go 1.26.0. Both used `CGO_ENABLED=0`,
`GOWORK=off`, `GOFLAGS=-p=2`, and `GOMAXPROCS=2`. Scans overlapped verification
and race tests; timings are not controlled performance comparisons.

## Counts

Only `lifecyclefacts` `function-summarized` events under the listed source
prefixes are included. Dependency summaries are excluded. Identical repeated
observations are deduplicated by candidate position and function identity.
The aggregation script rejects conflicting snapshots instead of selecting one.
Moby emitted 518 matching events for 428 functions; Kubernetes emitted 82 for
82. There were no conflicting snapshots.

| Evidence | Moby | Kubernetes |
| --- | ---: | ---: |
| Distinct summarized functions | 428 | 82 |
| Of those, in test source | 243 | 37 |
| Cached, completed graphs | 428 | 82 |
| Graph budget / fixpoint cutoffs | 0 / 0 | 0 / 0 |
| Summaries with truncated slots | 150 | 28 |
| Truncated summary slots | 543 | 153 |
| Distinct widening sites | 6 | 0 |
| Escape origins | 7,642 | 2,277 |
| Applied call summaries | 1,675 | 1,310 |
| Applied calls whose summaries were truncated | 773 | 451 |
| Calls without an available summary | 523 | 62 |
| Closure calls not substituted | 31 | 3 |
| Dynamic calls not substituted | 39 | 79 |
| Interface calls not substituted | 163 | 34 |
| Goroutine launches not substituted synchronously | 53 | 3 |

Call records count instructions, not fixpoint visits. Widening sites are
distinct `(abstract slot, instruction)` pairs. Escape origins are distinct
`(slot, escape kind)` pairs within a function, not resource leaks. Applied
truncated summaries can still establish useful effects for unaffected slots.
`calls-applied` and `calls-summary-applied` are aliases in the output: do not
add them together. The eight-call textual sample does not limit these counts.

## Concrete examples and priorities

- Moby's [pluginAdapter.Log](https://github.com/moby/moby/blob/3f673306102e01c16b5e0ab2343588bfab2fc4e7/daemon/logger/adapter.go#L36)
  conditionally returns a message to its pool from a deferred closure. The
  graph records that closure as unsubstituted and the encoder call as an
  interface call. Its summary truncates the message parameter and result.
  This is a concrete target for exact closure binding and conditional effects,
  not proof that either gap causes a diagnostic to be missed.
- Kubernetes's [GarbageCollector.Run](https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/controller/garbagecollector/garbagecollector.go#L132)
  records six interface calls, one deferred closure, and one dynamic call as
  unsubstituted, alongside eleven applied summaries, eight truncated. The
  source includes receiver-interface calls, deferred shutdown/join work, and
  a cancel function returned by `context.WithTimeout`. This motivates exact
  dispatch/binding and resource-scoped evidence, not pretending an arbitrary
  function value has a known target.
- Five of Moby's six widening sites were in test source. The remaining one
  was in [jsonfilelog.decoder.Decode](https://github.com/moby/moby/blob/3f673306102e01c16b5e0ab2343588bfab2fc4e7/daemon/logger/jsonfilelog/read.go#L65).
  The counters locate the function, not the exact widening instruction.
  Inspect its region dump before choosing a container-model extension.

For these samples, missing/substituted-but-truncated call effects are more
prominent than graph convergence limits. Prioritize exact closures/interface
targets (`y7x.1`), conditioned relationships (`y7x.2`), and scoping retained
evidence to the consuming resource (`pza`). Do not raise graph budgets based on
these results. A `no-summary` on a standard helper does not by itself mean
that helper needs a new contract; inspect what relevant state it can affect.

Function-level counts do not attribute every loss to an analyzer's final
unknown decision. Candidate-level attribution currently exists for
`nilargument`'s earlier-call evidence, but these scans emitted no such
diagnostic candidates. Full provenance from every resource query to every
summary truncation remains a limitation.

## Reproduce

At either pinned checkout, using the matching toolchain/environment above:

```sh
/path/to/gohawk -json -enable=resourcelifetime,nilargument \
  -gohawk-trace=lifecyclefacts,nilargument \
  -gohawk-trace-file=/path/to/trace.jsonl ./selected/package/...
```

From this checkout, use the absolute source prefix of that package family:

```sh
jq -s --arg root '/path/to/pinned/repository/selected/package/' \
  -f scripts/heap-loss-summary.jq /path/to/trace.jsonl
```

The local run's JSON, JSONL, stderr, timing files, and frozen binary are under
`.build/heap-loss-2026-09-24/`. Tracing reads cached graphs only and does not
build or republish them. A cache miss or in-progress build must remain separate
from a completed graph with zero recorded losses.

## Exact call binding follow-up

The same pinned scopes were replayed against baseline `5993aa8` and the
closure/interface binding implementation. Both profiles enabled only
`resourcelifetime` and `nilargument`, with the same toolchains and package
selection above. Every scan completed with exit status 0, empty stderr, and
diagnostic JSON `{}`. These remain evidence measurements, not new bug findings.

| Evidence | Moby baseline → updated | Kubernetes baseline → updated |
| --- | ---: | ---: |
| Distinct functions | 428 → 428 | 82 → 82 |
| Applied summaries | 1,675 → 1,684 | 1,310 → 1,311 |
| Unsubstituted closures | 31 → 13 | 3 → 2 |
| Unresolved interface calls | 163 → 157 | 34 → 33 |
| Missing summaries | 523 → 523 | 62 → 62 |
| Uncertain defer registrations | 0 → 15 | 0 → 1 |
| Truncated slots | 543 → 542 | 153 → 153 |
| Graph budget / fixpoint cutoffs | 0 / 0 → 0 / 0 | 0 / 0 → 0 / 0 |

The closure/interface reductions are **not** all newly applied summaries:
conditional or repeated defers now have an explicit uncertainty reason instead
of being treated as unconditional effects. Exact defers run in reverse
registration order. This correction intentionally declines some old evidence.
The net application gains above include test-source functions.

Production examples with newly applied closure effects include Moby's
`MessageQueue.Enqueue`, `LogFile.WriteLogEntry`, and `follow.Do`. Applying a
summary does not imply that its effects are complete. `pluginAdapter.Log`'s
conditional cleanup still has an error cell changed after defer registration;
that capture remains unsupported rather than becoming unconditional cleanup.

Binding requires exact cell contents and read-only use of the captured cell's
address. Ambiguous writes, deferred changes, exposed cell addresses, nested
pointer dereferences, unresolved dispatch, and recursion remain conservative.
Imported declarations bind by signature even without SSA parameters. Generic
calls retain their concrete instantiation before the registry's origin fallback;
a first replay caught and corrected a regression in that distinction.

Local verification passed, including all normal tests, architecture checks,
lint, vet, generated documentation, and self-analysis. A regression also ensures
that writes-only storage identity cannot reenter the heap graph while its lock
is held. Race testing is CI-only.

Updated scan wall times were 77.97s (Moby) and 125.64s (Kubernetes); baseline
times were 63.52s and 157.51s. Cache warming and concurrent verification differ,
so these single runs establish successful bounded execution, **not a speedup
or a regression estimate**.

Artifacts are under `.build/heap-binding-2026-09-24/`; final files use the
`corrected` suffix. Baseline binary SHA-256:
`e9fd25d8977c14ecd3f68f4ad3dce7c1c7681229dcdb74e63ae712df2b6e037f`.
Updated binary SHA-256:
`c0b2e0725b3429b45ecc613198b3f37ece2c8a872fbdeb7e567f0d1cfe68b7c6`.
Temporary repository clones are removed after the comparison.
