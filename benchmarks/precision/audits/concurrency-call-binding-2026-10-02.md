# Concurrency call binding — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.24.1`. Parent: `b02b611`.

## Domain-owned binding

`Provider.ConcurrencyAtCall` copied and bound imported formal declarations
before falling back to the concurrency engine. `Engine.AtCall` already owns
imported lookup and binding, including dispatch, exact argument substitution,
field materialization and cutoff provenance. The broker now delegates every
selected concurrency call to that one domain boundary. Component availability
remains separate from summary completeness.

Removing the duplicate path exposes an unused formal concurrency view. The
dead-code gate rejects `Function.Concurrency`; source search confirms that its
remaining uses are tests, with `Engine.Declaration` and its declaration-copy
helpers serving only that facade. The unused view, exported binding adapter,
declaration lookup/copy engine and their API-only tests are removed. Result and
lifecycle formal views remain used. Publication, Fact encoding/version and the
domain's immutable imported cache remain unchanged. Tests now mutate actual
bound linear/alternative effects and verify fresh queries, rather than keeping
a production copy API solely to test it.

Graph tools were unavailable. Source fallback reviewed the broker, domain
AtCall/query/materialization, instantiate/imported lookup/binding, declaration
copy path and its callers, plus the direct concurrentcapture consumer. The
repo-wide Go source search finds no remaining Declaration or BindDeclaration
call. This is not a completed review of every summary consumer.

## Consumer inventory

The initial selector-name search produced 26 candidates. A Go type-info scan of
the selected production `./internal/...` packages identifies 71 broker function
and method references in 28 analyzer files, including setup selections and
method references. It checks declaration identity, accounting for import aliases
and avoiding unrelated selectors with the same names. Tests, testdata and files
outside the current build configuration are outside that scan.

The [typed location ledger](summary-consumer-locations-2026-10-02.tsv) records
each source position and leaves dispositions pending under parent
`gohawk-dho.44.11.5.27.24`. Local artifacts are
`.build/goal-summary-consumer-inventory.go` and
`.build/goal-summary-consumer-inventory-final.json`. A locator inventory is not
proof of consumer policy cohesion or a whole-query cost bound.

## Validation

Real exported/imported SSA controls bind four ordered mutex effects through a
forwarding package and a local helper. They check exact caller arguments,
call-site provenance, missing declarations, cutoff across intermediate
allowances and fresh evidence after returned-result mutation. The controls pass
on the parent overlay and current; focused summaries, concurrencyfacts and
concurrentcapture suites pass. Imported alternative cache mutation is covered
at the actual AtCall boundary before the caller summary is cached.

Final focused receipts are `.build/goal-concurrency-binding-parent-final.log`
and `.build/goal-concurrency-binding-focused-final.log`. Three ignored overlays
remove imported lookup, substitute the first argument for every effect or
publish an interrupted prefix. All fail assertions without compilation failure
or timeout in `.build/goal-concurrency-binding-mutants-final/`.

The first gate fails on the newly unused formal view. The initial counterfactual
also reveals a test harness defect: Fatal in an analysis Run goroutine exits
without completing the driver's action. Its SIGQUIT stack shows the checker
waiting for that action. Those failed/stalled attempts are not passing evidence.
The corrected controls record errors and return normally; the final gate and
counterfactual receipts supersede the initial attempts.

The normalized complete-body scan covers 317 production files and 2,145
functions, with five previously dispositioned distinct-contract groups. It
cannot prove partial duplication absent. Final canonical validation passes all
eight local targets in `.build/goal-concurrency-binding-verify-final.log`,
including dead-code, lint, ordinary tests and self-dogfood. Generated helper
references remove the unused APIs.
Final architecture checks pass in
`.build/goal-concurrency-binding-architecture-final.log`.

## Scoped compatibility and limits

Parent `.build/goal-structural-same-reviewed`, SHA-256
`f3d921ca7141ce9e81124ddddb6fe43e64af86421f7ac0efa28f35c6782cb4c1`.
Current `.build/goal-concurrency-binding-final-reviewed`, SHA-256
`d6b5ca6ca77b46e646845d0a6ca49576fd3984668118796e6579eac0466b0e28`.

The [comparison ledger](concurrency-call-binding-2026-10-02.tsv) records five
parent/current scopes in `.build/goal-concurrency-binding-final/`. All ten scans
exit zero with empty stderr. All 123 lock/capture fixture diagnostics, four
pinned XD/goiardi production controls and the two duplicated SkyWalking FP sites
retain byte-identical JSON. Production scans use readonly modules, disabled
CGO/workspaces, 180-second timeouts and at most two workers. Resource and
goroutine fixture cohorts are not repeated for this concurrency broker change.
No full precision-regression replay or local race run is performed.

No production FP reduction is credited. Parent .24 still owns the finite
consumer-policy dispositions. Ten production FP sites plus Rune and the broader
architecture completion review remain open.
