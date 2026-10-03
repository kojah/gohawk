# Factory-origin query allowance

Bead `gohawk-dho.44.11.8`; parent source `5e74c02`.

## Query and decision ownership

Exact source review found explicit storage allowances without corresponding
reaching-value allowances in channel-factory and opaque-group origin queries.
Both now use the existing shared reaching fold with their supplied budget;
wrapper, phi and recursive operand visits share the storage allowance. Each
query returns a structured proof rather than treating interrupted search as
false. A single completion-handle decision maps incomplete origin evidence to
unknown with `factory-origin-budget-exhausted` and evidence phase
`factory-origin`, before later lifecycle explanations can name another cause.

The existing order is preserved: signal factory opacity precedes external
transfer; externally owned groups transfer before their factory query. Channel
factories retain possible ownership, while group factories are opaque only
without a resolved body. Field/tuple traversal, capture storage observations
and the two transparency policies retain their contracts. The cohesive origin
file owns this evidence and its decision adapter; existing rationale links move
with the implementation. No allowance, fact schema or reporting policy expands.

## Controls and verification

Ten actual SSA controls cover direct, wrapped, phi, field, opaque and fresh
origins. They log the SSA, preserve fresh polarity, require incomplete answers
for tiny allowances and sweep cutoff behavior. Caller integration checks
origin, caller-lifetime and relay cutoff phases, candidate attribution and
fresh recovery. Restoring the unbounded fold behavior fails the tiny-budget
assertions; ignoring cutoff in the decision adapter fails caller assertions.
Both counterfactuals compile and fail assertions, rather than setup.

Final `make verify VERIFY_TIMINGS=1` passes every gate: generation 1s, modules
0s, vet 1s, formatting 2s, dogfood 2s, deadcode 4s, lint 5s and ordinary tests
49s. Six read-only parent/current all-check invocations preserve complete
payloads: 128 goroutine/closure-choice fixture findings, zero Openase findings
at `e530faf137e764337d5beaaf68af3be159eb17aa` in internal/infra/hook, and the
one stargz store control finding at `624678b4e421947534cbf0618f9609853cccee0f`.
All exits are zero with empty stderr; pinned production checkouts are clean
with readonly module loading. The reviewed binary matches the canonical build:
`174e194f8ba6e72f2ab582bcb98551db2d629ef09d4aa4dc3eea5e535b297666`.
Ignored local receipts are under `.build/goal-factory-origin-*`.

## Limits

Graph/index tools were unavailable. Source candidate scans now cover 335
production files and 2,205 declarations, retaining five whole-body and 34
partial groups with unchanged path/symbol/token signatures. These are review
aids, not proof of absent semantic duplication. Selected query visits are
bounded; graph construction, alias/type internals, allocation, rendering and
standalone policy traversals retain separate cost owners. No whole-query
wall-clock bound, full precision replay, local race run or production FP
correction credit is claimed. Parent reconciliation and the five-site queue
remain open.
