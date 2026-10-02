# Read-lock diagnostic reporting across paths

Beads `gohawk-dho.4.5`.

Pinned Skywalking cursor initialization has a short-circuit condition with two
predecessors entering the same write block. Actual SSA from
`.build/goal-skywalking-field.ssa` and the minimized fixture confirms this shape.
Path-specific branch evidence revisits the two writes, producing four
read-lock diagnostics for two locations.

The authoritative `proveReadLockWrite` remains unchanged. Every state still
computes and traces its proof. Reporting records proven instruction/lock pairs
in a function-owned map; an unknown state does not reserve a pair, and different
lock identities at the same write remain distinct. The existing function report
buffer commits only after complete analysis; interrupted prefixes are discarded.
The map is recreated for each walk, including recovery after a cutoff.

`readlockpaths` tests two writes reached through converging predecessors,
a writer-guarded path beside an unguarded path, and two read-lock identities
at one instruction. The parent fails with exactly two unexpected duplicate
diagnostics; the current entire lockorder package passes, including existing
cutoff-discard/fresh-query recovery tests. Actual fixture SSA is saved at
`.build/goal-readlock-paths.ssa`.

Six all-check scoped scans exit zero with empty stderr. Existing lock fixtures
retain all 108 diagnostics byte-for-byte. The new path fixture goes from seven
to five reports; pinned Skywalking goes from four to two. Both comparisons
preserve the complete set of distinct diagnostic payloads, including related
acquisition evidence. The [scan ledger](readlock-path-reporting-2026-10-02.tsv)
records binary hashes and individual receipts. Skywalking is pinned at
`e83d5925500a7e63dd55c080a9b1542d6cedaefb`, with a clean checkout and readonly
module loading; the scan is restricted to `./pkg/tools/buffer`.

This corrects duplicate presentation, not field association or concurrency
semantics. The two Skywalking production FP sites, eight other production FP
sites and Rune remain open. Common receiver identity does not establish which
fields a mutex protects; unlocked cursor writes do not prove confinement. No
field-name, framework or speculative participant exemption is introduced.

Graph tools are unavailable; exact source and compiled SSA provide the scoped
evidence. Receipts use `.build/goal-readlock-paths-*`. No full precision replay
or local race run is used. Final `make verify VERIFY_TIMINGS=1` passes all
eight canonical targets, and final architecture validation passes.
