# producerlifecycle design notes

The public page is [producerlifecycle](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Complete shared concurrency summaries expose sends and receives hidden in
helpers, including imported helpers. They feed the existing bounded count proof:
a local unbuffered channel, an observed receiver, and explicit excess sends.
Deferred receiving helpers count as receivers too. Opaque channel consumers,
possible drain loops, and additional receiving goroutines leave the result
unknown rather than being mistaken for absent receivers. Incompletely modeled
competing workers can therefore suppress a finding, unless shared call-effects
evidence proves that all uses of the channel are non-receiving. Buffered channels and
externally supplied channels remain outside this proof.

Loop-based send counts remain unknown: a repeated statement does not prove
multiple sends are feasible. This deliberately misses unbounded producer loops
until their excess production can be established without a cardinality guess.
A `go` statement inside a loop counts the same way: it starts as many
producers as the loop runs, which may be none or one, so its sends are an
unknown count even when the goroutine body sends once. Before this rule such a
producer was counted once and traced as within the receive count
(`producerPerItem` in `uncertain_counts.go`).
Callers that terminate the process instead of returning do not establish an
abandoned receiver lifecycle.

## Retired experimental checks

Four experimental checks were removed on 2026-09-27; the last revision
containing them is `73f25e27`. The experimental tier itself remains.

- `stopped-loop-send` reported a send to a struct-owned channel after the
  service loop serving it had returned, and `unclosed-range` a range over a
  channel whose producer could return an error without closing it. Neither
  made a report across the batch 56 to 61 audits, about 1,500 repositories.
- `unreceived-return` reported a one-shot worker blocked on an unbuffered send
  the launching function could return without receiving, and
  `unsignalled-receiver` a worker blocked on a receive the function could
  return without satisfying. In batch 61 the first made eight reports, all
  true positives, seven of them also reported by `goroutineownership/unjoined`;
  the second made none.

The channel census they shared was removed with them.
