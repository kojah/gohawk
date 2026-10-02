# Imported concurrency declaration isolation

Beads `gohawk-dho.44.11.5.25.6` fixes the imported metadata boundary identified
by the [publication follow-up](concurrency-publication-followup-2026-10-02.md).
`Declaration` previously detached linear effect fields and worker effects, but
returned the cached alternatives and their nested slices directly. A caller
could therefore change later declaration results by mutating an alternative.
Those copy loops also ignored the supplied search allowance after lookup.

One body-copy path now owns linear and alternative effect, field, cancellation
and worker metadata. A bounded slice copy charges each visited element before
copying, without allocating from an unvisited collection length. It also copies
alternative conditions and returned metadata. Strings and scalars are immutable
values. Nil and empty slices retain their shape; absent and incompatible facts
stay unavailable. Any cutoff returns no fact and leaves the cache unchanged.
Schema version 9 and formal-parameter interpretation remain unchanged.

Actual bodyless imported SSA declarations exercise mutation isolation of outer
alternatives, effects, fields, methods, cancellation requirements, worker
prefixes/effects, conditions and returned constants. Every cutoff is paired with
a child/parent/fresh control. A nested field control gives lookup and its effect
enough allowance while forcing the field copy itself to cut off. The existing
broker parity test checks real local/exported/imported declaration access and
binding. The imported `branchdep.Pick` fact is also mutated before its caller
summary is cached, preserving later declaration results and path binding;
injected metadata in the mutation tests isolates copy ownership rather
than claiming new inference semantics.

Graph discovery tools were unavailable; inspection used scoped source fallback.
Package setup, heap/type internals, string/constant formatting and fact codec
costs remain separate. This corrects cache ownership, not a demonstrated
production FP; the queue remains 11 unresolved sites, with frozen labels intact.
No local race test or full precision-regression audit is part of this iteration.


## Validation

Focused declaration, broker parity and real imported-alternative tests pass:
`.build/goal-declaration-copy-reviewed-focused.log` and
`.build/goal-declaration-copy-real-import.log`. Restoring the old declaration
copy reproduces nested-alternative cache corruption; bypassing only field-copy
charges retains an effect under cutoff. Both source overlays exit 1, without
editing the checkout. Receipts:
`.build/goal-declaration-copy-{mutation,field}-counterfactual.log`.

The immutable binary `.build/goal-declaration-copy-reviewed` implements parent
`91e8d67` plus these production changes; SHA-256:
`01cec016842816fdcc01eb9c5c3625838fd9f8c20f4011208b9c2fb6f040fd82`.
The pinned clean stargz checkout at
`624678b4e421947534cbf0618f9609853cccee0f` was scanned with
`-enable-all -json ./store`, `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, and
`GOWORK=off`. It exits 3 with empty stderr, retaining the reviewed goroutine TP
at `manager.go:193:2` and return witness `217:2`. JSON is byte-identical to the
publication parent control. Receipts:
`.build/goal-declaration-copy-stargz.{json,err}`. No candidate tests, generators
or applications ran.

The final canonical gate passes (`make verify VERIFY_TIMINGS=1`,
`.build/goal-declaration-copy-verify-final.log`): generation, module checks, vet,
formatting, deadcode, lint, local dogfood and ordinary tests. The first gate's
two test-formatting errors were corrected with the canonical formatter before
final validation.
