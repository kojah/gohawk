# Initial mutex publication ordering uncertainty

Bead: `gohawk-dho.23.24`. Production issue: `gohawk-cnx`. Parent: `8569f1f`.

Rune publishes a fresh mutex into its iterator map while holding the map owner's
writer, then immediately locks the new mutex. Existing exclusivity correctly
rejects that exposed value. The bounded acquisition decision now leaves only
the ordering edge from a uniquely matching owner writer unknown. It does not
infer map guarding, participant confinement or exclusive ownership. Other held
edges and subsequent held-state bookkeeping are retained.

The same-block interval permits field addresses, loads, debug references and one
exact map-value publication. Calls, stores, prior acquisitions, another
publication and owner release interrupt it. Loaded gates, reader guards,
different owners, multiple matching writers and uncertain release do not qualify.
Exact owner identity and instruction positions use shared budgeted SSA queries.
Interrupted evidence cannot publish a matching guard identity.

This follows the failure ladder at the existing acquisition decision without a
new proof file, name exemption or check demotion. Readers that bypass the owner
writer can still make this initialization participate in a real cycle. The old
`lockorder/gate_mutex.go` diagnostic fixture is deleted and its coverage loss is
recorded in the new fixture header and design note. Publication remains exposed
for `ExclusiveAt`; the new result is uncertainty, never a safety guarantee.

Graph/index/coverage tools were unavailable; evidence is bounded exact source,
compiled SSA and scoped behavior. Broad semantic consolidation is unproven.

## Controls and scoped receipts

The parent fails the accepted initialization fixture by reporting its cycle.
The final fixture retains two cycles for a different owner and an intervening
exposure call. Actual SSA controls cover read guards, loaded gates, prior
acquisitions, direct exposure, duplicate publication, late acquisition of the
owner, release before the new acquisition, multiple matching writers, unrelated
held writers, uncertain release and exhausted/fresh allowances. Trace tests
assert the authoritative unknown decision and candidate association.

Parent SHA-256:
`9615a352193007fa4f9ed3a02389ce7cde95288b61b9e8cce483d4603cc7ac3d`.
Final immutable binary SHA-256:
`5eb92345e8f2735e05dab9db9482098142e55614aadb17622f22bcb5516109e9`.
Fourteen terminal all-check receipts exit 0 with empty stderr. Full merged
payload comparisons add no findings and remove exactly these locations:

| Scope | Parent | Current | Removed |
| --- | ---: | ---: | ---: |
| lock-fixture | 117 | 116 | 1 |
| private-fixture | 6 | 6 | 0 |
| rune | 1 | 0 | 1 |
| skywalking | 0 | 0 | 0 |
| fields-fixture | 4 | 4 | 0 |
| goiardi | 9 | 9 | 0 |
| publication-fixture | 3 | 2 | 1 |

The parent lock-fixture receipt was captured before deleting `gate_mutex.go`;
its removal is a documented coverage loss, not production correction credit.
All other parent/current scopes use the same fixture source or clean pinned
candidate checkouts. Rune pin: `3e2165f8983280542c985947378dfa740a397d03`
with CGO enabled. Skywalking pin: `e83d5925500a7e63dd55c080a9b1542d6cedaefb`.
goiardi pin: `937cae400a92d8036b88ae2f65d93506271c292e`.
Candidate scans use read-only module mode and do not run candidate tests.

The Rune trace at acquisition line 419 returns
`initial-publication-order-unknown`, with no candidate budget exhaustion; its
original report at line 422 is removed. The initial line-422 trace filter was
empty because it selected the reporting call rather than the acquisition.
That preliminary trace is not a correction receipt. Final traced/untraced
payload equality is checked separately.

Initial lint/canonical attempts caught complexity and tracing-test size issues.
They are superseded by final validation after the refactor. No full precision
replay or local race test is part of this iteration. Receipt paths:
`.build/goal-initial-publication-final-scans/{scans,comparison}.json`;
`.build/goal-initial-publication-{rune,fixture}-final.trace.jsonl`;
`.build/goal-initial-publication-final-verify.log`.

Final `make verify VERIFY_TIMINGS=1` exits 0 with all eight gates passing:
generation, module verification, vet, format, lint, deadcode, self-dogfood and
ordinary tests. Self-dogfood takes 2 seconds and tests 19 seconds. The canonical
binary hash matches the immutable reviewed binary. Final Rune and fixture
traces preserve complete untraced diagnostic payloads, contain the expected
unknown decision and exit 0 with empty stderr. Rune has no candidate budget
cutoff. The recorded residual queue is now five production locations: Openase
two, goiardi two and Ferro one; their missing state/protocol evidence remains
unresolved, and broader completion is unproven.
