# Completion binding metadata consistency

Bead: `gohawk-dho.23.21.6`. Parent: `fbd1cd0`.

The local completion mapping already uses `ssaflow.CallBindingsWithin`.
Callback environment construction still materialized all arguments/captures
before charging each pair; enclosing read-only helper traversal enumerated
bindings without any metadata charge. Both now use the existing lazy driver.
There is no additional pairing algorithm or new proof model.

Callback metadata cutoff discards the entire staged environment, marks the
completion search incomplete and invalidates its current memo answer. Arguments
retain the caller environment, captures retain the lexical environment, and
both keep the invocation's observation point. Enclosing read-only traversal
rejects an incomplete helper binding census and invalidates the enclosing memo
answer; inspecting only a prefix cannot prove that no later binding can mutate
the aggregate. Default closure queries that do not take a budget retain their
existing materialized bindings and policy. This review is bounded to these two
explicit-budget consumers, not a repository-wide boundedness claim.

Actual SSA controls cover an argument plus two lexical captures, incomplete
and exact complete allowances, distinct environments and observation times,
completion memo retry after cutoff, read-only metadata cutoff, a fresh child
allowance and exhausted shared pool. Both controls fail assertions against
parent source overlays, not compilation: the parent publishes a partial
callback environment and credits a partial read-only census. Current focused
completion and enclosing callback tests pass.

Graph/index/coverage tools were unavailable. Exact source, the binding callers,
shared iterator contract and compiled SSA provide scoped evidence. No absence
of differently structured duplication or overall consolidation is claimed.
No production FP correction is claimed; Openase and Ferro previously silent
candidates still need protocol/state evidence. No full precision replay or
local race test is part of this iteration.

Final immutable binary SHA-256:
`d83fe660eb8fc23884c7d7c36ec12fca31b716efe1ea71a6cc8c9561a8e4f347`.
Parent SHA-256:
`5eb92345e8f2735e05dab9db9482098142e55614aadb17622f22bcb5516109e9`.

Local evidence: `.build/goal-completion-metadata-{focused,parent-controls}.log`;
`.build/goal-completion-metadata-final-scans/{scans,comparison}.json`;
`.build/goal-completion-metadata-final-verify.log`.

Fourteen terminal all-check scans exit 0 with empty stderr. Complete merged
parent/current diagnostic payloads agree in all seven affected scopes:

| Scope | Findings, unchanged |
| --- | ---: |
| cancel-fixture | 40 |
| lock-fixture | 116 |
| resource-fixture | 318 |
| process-fixture | 41 |
| goroutine-fixture | 128 |
| openase | 0 |
| ferro | 1 |

Openase pin: `e530faf137e764337d5beaaf68af3be159eb17aa`. Ferro pin:
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`. Candidate checkouts were clean
and module scans read-only, with CGO disabled. These comparisons preserve
existing reports and silence; they do not credit either candidate as corrected.

Final `make verify VERIFY_TIMINGS=1` exits 0 with all eight gates passing:
generation, module verification, vet, format, lint, deadcode, self-dogfood
(77 seconds) and ordinary tests (122 seconds). The canonical binary matches
the immutable reviewed binary. Final architecture tests after the audit and
completion-matrix updates pass. Unrelated staged deletion, Beads interaction
log and working-tree artifacts remain outside the focused commit.
