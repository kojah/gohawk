---
title: Local storage model
description: Observation-time storage evidence shared by gohawk analyzers.
---

`heapmodel.Storage` answers a bounded question: what value occupies this local
location at this instruction? It is a demand-driven store query, not a
whole-program points-to solver or a second lifecycle engine.

## Evidence boundary

- Locations are local allocations plus exact field and constant-index paths.
- Equivalent field addresses share writes. The backwards query stops at the
  first write on each path, so an unconditional overwrite discards earlier
  disagreements. A join is known only when every incoming value agrees.
  A sole dominating initializer stays known across loops without other writes;
  other cyclic queries conservatively return unknown.
- Aggregate copies and saved loads retain their original observation time.
- Slices of local backing arrays can use constant bounds and indexes, including
  nested offsets and three-index bounds. Dynamic indexes or bounds, maps,
  pointer-cell escapes, and unknown mutations
  do not establish contents.
- Writes to independent fields and array elements do not invalidate each other.
  Whole-owner escapes still invalidate knowledge of every field.
- Conflicting branch writes remain unknown. There are no speculative alias sets
  or interprocedural heap summaries.
- Queries have a search budget. Exhaustion returns unknown, not inequality or
  evidence that cleanup is missing.

`Content` returns the value written before an observation. `Resolve` follows
local loads to their values. `Same` requires definite equality after resolution.
`StableContent` additionally checks later uses, for captured cells that will
be read after registration. Its caller must check the observation's own effects;
callback mapping already checks that the callee reads its supplied aggregate.
`Projection` proves an acquired owner's field has not been replaced or exposed.

## Call effects

`ssaflow.CallEffects` supplies bounded local effect evidence to storage and
lifecycle consumers. It separates reads, mutation, retention, asynchronous
exposure, and invocation of a supplied callback. `PreservesStorage` requires a
complete proof with no effect beyond reading. Missing bodies, dynamic dispatch,
recursive calls, unsupported uses, and exhausted budgets cannot prove safety.

Effects describe the supplied value and directly selected storage, not every
object reachable from it. Loading a pointer reads its slot; the pointee is not
the slot. Returning that loaded pointer therefore does not itself expose the
slot's address. Returning the address, storing it, or handing it to concurrent
work prevents preservation. Stores (including local spills) are conservative
retention boundaries, with subsequent effects unknown rather than guessed
through aliases. Observed flags remain visible on an incomplete proof, but
missing flags cannot establish the absence of an effect.

The query follows visible static helpers and joins possible effects across all
uses. It does not assert an effect happens on every return. Retention must not
be interpreted as ownership transfer, nor reading as cleanup. Imported lifecycle
facts do not prove read-only behavior: no cross-package effect fact is added.

`CallBindings` centralizes argument/capture mapping for completion and goroutine
analysis. Captures stay distinguishable from eager arguments; their timing and
ownership policies remain in their consumers. Callback aggregate and captured
address checks use the effect query rather than separate read-only scanners.

Return-to-argument relationships, conditional transfers, collection cleanup,
and general pointer analysis remain outside this increment.

The pure `DefinitelySameValue` and `ProveIdentity` primitives still answer
value identity and structural path correspondence, respectively. They do not
guess current contents from historical stores. Likewise, `StoredInto` and
possible-origin traversal remain useful for escape and containment questions;
their results must not be promoted to a guaranteed release.

## Migration inventory

| Consumer | Shared storage question |
|---|---|
| Resource lifetime | direct cleanup, stored fields, and SQL parent identity |
| Defer in loop | acquired resource and cleanup receiver identity |
| Channel safety | whether close and send use the same loaded channel |
| Goroutine ownership | stable captured channel used by a guarded join |
| Process ownership | exact command selected for Wait |
| Lock ordering | local receiver contents before structural lock naming |
| Error ownership | return-slot value at its load |
| Lifecycle facts | exact arguments, callback invocation, and unchanged returns |
| Shared completion | callback cells, fields, array elements, and enclosing captures |

Cancellation ownership and other completion consumers inherit the migration
through the shared completion machinery. Syntax checks, acquisition contracts,
field naming, escape policy, and possible-containment scans have not been
rewritten into storage queries: they answer different questions.

Removed mechanisms include the separate single-store load resolver,
latest-store selector, immutable callback cell/address selectors, deferred
return-slot scan, analyzer-local field-address matching, and SQL-parent cell
matching. The now-unused access-path evidence cache was removed as well.

## Regression evidence

The original eight independently classified resource probes now report all
four leaking cases and accept all four valid cases. The baseline reported
three of the valid cases. These synthetic probes establish capability, not
real-world bug prevalence or a recall percentage.

Permanent fixtures cover field and array cleanup, copied owners, snapshots
before replacement, stale cleanup, and channel sends after close. Storage unit
tests cover uncertainty from opaque mutation, branch writes, dynamic indexes,
closure mutation, and budget exhaustion. Existing callback regressions retain
the distinction between eager defer arguments and later reads of captures.
