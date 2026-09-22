# Follow-up337: lock, capture, channel, once, evaluation, and loop-lifetime sites

Scope: all46 assigned original false-positive sites, across33 package/analyzer
scopes: lockorder33, concurrentcapture5, deferinloop5, and one each of
channelsafety, evalorder, and oncepolicy. Every source site was reread at its
original pin. Original audit records were not changed. The companion
`followup337-locks.tsv` records each disposition and its receipt.

Final explicit-check result: **10 verified absent, 36 retained-needs-proof**.
All33 package/analyzer scopes loaded successfully (exit0, no loader errors).
The ten fixes comprise six callback handoffs, two deferred-mode sites, one
package initializer, and one collective resource lifetime. No original verdict
was relabelled to inflate this count.

## Changes and precision boundaries

- **Unlock callback handoff:** an exact Unlock/RUnlock callback passed to a
  consumer or stored in a receiver field makes held state unknown. This is not
  a completion or scheduling proof. Merely constructing an unhanded callback,
  or handing off a callback for a different mutex, retains recursive-acquire
  reporting. A callback handoff is not counted as a positive release witness:
  the initial implementation otherwise invented a new missing-release warning
  after a final private-gate acquisition in acme-dns. A new accepted fixture
  and a subsequent full-diagnostic replay cover that correction.
- **Deferred acquisition and modes:** registering `defer RLock` does not acquire
  reader state in the function body. A fresh exclusive acquisition clears stale
  reader mode after a helper/unknown release. The uTLS rotation sequence is the
  motivating case. This does not model arbitrary LIFO deferred lock protocols.
- **External writer guard:** a proven-held exclusive lock on a different owner
  makes read-lock-write guard inference unknown, just like one on the same
  owner. This accepts the false negative where that mutex is unrelated. The
  actual gocryptfs sites still need an imported held-writer summary for the
  countingMutex wrapper around embedded sync.RWMutex and atomic bookkeeping;
  the direct fixture improvement is not recorded as solving those sites.
- **Once initializer:** direct package-variable initializers already evaluate
  once. Stored function literals remain repeatable and diagnosed. This uses
  type-checked initialization metadata, not naming conventions.
- **Collective resource lifetime:** field/index storage is unknown iteration
  ownership. Dominating stores before a defer receive the same treatment as
  stores afterward. This covers pmtiles' append-before-defer resource collection.
  Sloth remains reported because a nested population loop has an unproven
  zero-iteration path; the change is not claimed to solve that site.

All changes reuse existing flow/identity/completion vocabulary. No new summary
family, state enumerator, framework contract, loop-count model, check retirement,
disablement, demotion, suppression, or candidate-code execution was introduced.
The existing large lock-state transfer remains cohesive around one state model;
the precision changes stay at its existing transfer decision points.

## Retained evidence families

The ledger distinguishes these questions rather than calling them unfixable:

- Conditional held-on-return contracts: gmqtt and fortio require result-to-lock
  relations, including a false Boolean meaning the caller receives the lock.
- Feasible-path correlation: surf and bazel-remote track Boolean ownership;
  geesefs repeats a compound pointer guard; gonc requires stable receiver state;
  uhaha correlates a sentinel with ownership; OpenSurge uses disjoint helper
  conditions. ContainerSSH additionally needs a closed private state domain.
- Library success and cardinality: auth's valid constant template cannot take
  its error branch; gophercloud's callback count is bounded by its retry
  implementation; clusternet's worker collections contain one element;
  nerdlog's rotation guard runs once; paperboy's table has one entry. No
  loop-count or framework-specific exception was added.
- Publication, participants, and identity: gauge initializes before launching
  watchers; file.d and opcua use fresh unpublished objects; WireGuard needs
  caller guards and per-peer lock identity; dtls uses distinct client/server
  instances; yamux joins recvLoop before reverse locking; uTLS' remaining order
  witness needs QUIC protocol context. BounceBack uses private completion gates.
  Exported helper behavior beyond reviewed in-repository callers is not claimed
  safe. These remain useful questions for future bounded shared evidence, not
  a reason to assume every type-level cycle is realizable.
- Keyed synchronization: driftctl uses LoadOrStore for the same key; resolving
  that key-to-mutex relationship through helpers is still missing.
- Intent and API data semantics: plow intentionally captures a recovered panic
  sentinel; a generic recover exemption would hide unintended failures.
  Nanobot relies on nonnil map identity and JSON object decoding rather than
  replacing the map header. Neither is solved by broad syntactic exemptions.

The checks retain useful bounded diagnostic subsets and their positive fixtures.
No retirement is proposed in this patch. Remaining sites are explicit
`retained-needs-proof`, not silently relabelled or described as fixed.

## Verification and provenance

Frozen baseline: source `c9b65a6`, executable
`.build/gohawk-followup337-baseline`, SHA256
`33743c44dfe1f68cbe8dc146989dbe76c0ab02bda55fa8f030a10e32861dfa02`.
Final corrected source: `5a56fb4`, executable
`.build/gohawk-followup337-final-contained`, SHA256
`3da9ce166dfd8dc72c3225dc1229298040ae27b04a394c49b2edcf7c3ca772a7`.
This includes the final function-signature prefilter before callback completion
search. Intermediate worktree binaries and receipts remain separate.

Final replay profile explicitly selects each reviewed check with
`-enable-checks`, additionally selecting its analyzer and test files, under
CGO_ENABLED=0, GOWORK=off, GOTOOLCHAIN=local, and readonly modules. Every receipt
asserts checkout HEAD against the original full pin and records binary hash,
command, package, exit status, loader errors, and complete diagnostics. Only
successful package loads count as absence. Replay concurrency is one.

Earlier v1-v4 receipts used analyzer-only selection and did not enable two
extended checks. They are preserved as intermediate artifacts, not final
verification. Baseline2 explicitly reproduces all12 initially targeted sites;
v5 receipts use the corrected profile. The final ledger uses
`followup337-locks-final-contained-5a56fb4` receipts only.

The final46 replay and `.build/followup337-locks-final-verify.py` both passed:
all46 original site identities are accounted for, all33 scopes loaded, all10
corrected FP sites are absent, the uTLS true-positive control remains present,
and no diagnostic was added compared with the explicit-check v5 replay.
The final receipts record source5a56fb4, the immutable checksum, full repository
pins, exact check categories, commands, load status, and complete diagnostics.

Focused tests passed for lockorder, oncepolicy, and deferinloop, including
failed-first minimized cases and retained diagnostic controls. The pinned
uTLS `common.go:1075:4` missing-release true positive is a real-source control.
Root owns repository-wide validation and final commits.

Graph evidence: Tier2 project `home-james-scratch-gohawk`, refreshed generation
2026-09-22T11:24:15Z, clean best-effort coverage for changed existing analyzer
files; source snippets and the walkLockOrder→transferOpaqueUnlocks caller
relationship were checked. External checkouts are excluded from the graph and
were reviewed directly. Actual SSA dumps were read for ctrld startup callbacks,
uTLS ticketKeys, gocryptfs Write, and pmtiles Merge before changing policy.
