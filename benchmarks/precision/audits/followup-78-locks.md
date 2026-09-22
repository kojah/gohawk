# Follow-up of the 25 remaining lockorder false positives

Implementation: `d21bfcd`.

All 25 lock sites remaining in `followup-337-results.tsv` were reassessed. The
original reviews remain unchanged; `followup-78-locks.tsv` records this pass.

Result: **2 verified absent, 23 retained-needs-proof**. All 20 package scopes
loaded under both baseline and corrected binaries. The replay also includes
two already-corrected uTLS sites, so its raw site count is 27, not 25. Four
originally reviewed true-positive diagnostics present in these scopes remained
present. No diagnostics were added; the only removals are the two surf sites.

## Changes and their boundaries

### Incoming-edge branch feasibility

Surf's `DialContext` carries a local `unlocked` flag through the merge after
the cached-connection branch. The old lock walk forgot which predecessor
entered that merge and explored a flag value belonging to the other path.
That invented a still-held lock at both final returns.

The lock state now preserves its immediate predecessor and asks the existing
shared `ssaflow.FeasibleSuccessors` query which branches are possible. This is
not a condition solver: no Boolean facts are retained across blocks, no loop
counts are inferred, and no new SSA traversal is implemented. It adds at most
the incoming-edge alternatives to each existing state. The same mechanism
already serves other flow clients.

A minimized accepted fixture failed before the implementation. Wrong release
flags and nonconstant incoming values remain diagnostic. Structured trace
coverage checks the `predecessor-constant-branch-infeasible` evidence. Both
original surf reports reproduce on baseline and disappear on the corrected
explicit-check replay.

### Locally allocated mutex identity

A second conservative boundary retains the instance identity of a mutex that
shared storage proves is a particular local allocation, instead of replacing
that evidence with the declaration class of a field holding it. A loop's
allocation instruction can represent different runtime instances, so it does
not connect class-order edges between iterations. Callee class witnesses also
decline such local allocations.

This can miss cross-function ordering after a local mutex is published. It
does not prove that publication is safe. Fixtures retain inversions involving
the same exact local object and the existing package-global controls; distinct
instances and repeated allocations are accepted.

This boundary is **not recorded as fixing file.d**. Its actual constructor
passes and stores the owner through intervening helpers before acquiring the
field mutex. Shared storage cannot then prove that the current field still
contains the allocation. Historical containment is not substituted for exact
identity, and the original report remains present.

## Reassessed gaps

The remaining sites require evidence not established by either correction:

- Conditional held-on-return contracts: gmqtt and fortio deliberately return
  a held lock for a particular result. Fortio's false Boolean means the caller
  receives the lock, illustrating why a generic success-name convention is
  insufficient.
- More complex guard relations: bazel-remote carries its Boolean across
  several blocks; geesefs repeats a compound disjunction; gonc rereads mutable
  receiver state; uhaha correlates a sentinel with nested loops; OpenSurge
  combines a string prefix and helper switch. The predecessor-only change
  does not claim to solve these relations.
- Closed domains or infeasible errors: ContainerSSH needs its private state
  transition domain; auth needs the success contract of parsing its exact
  constant template; gophercloud needs the retry/callback invocation bound.
  No test, branch-name, state-number, or iteration exemption was introduced.
- Imported writer ownership: gocryptfs's countingMutex wrapper acquires a
  different exclusive mutex while fdLock is held for reading. Its atomic
  bookkeeping prevents a complete imported sequence. A deferred release of
  that different mutex was not promoted to a proven-held writer guard.
- Publication and concurrency context: gauge initialization, file.d's new
  Job, opcua's new channel instance, uTLS's QUIC serialization, yamux's joined
  receiver loop, dtls's separate client/server objects, and WireGuard's caller
  guards/per-peer identities need context beyond declaration-class cycles.

These are reviewed evidence gaps, not claims that the code is defective or
that the questions are impossible to answer. No check was retired, disabled,
demoted, or given a project-specific exemption.

## Validation and provenance

Baseline source: `56a8966`; immutable binary
`.build/gohawk-resource-tightening-v2`, SHA256
`03d703b2724c0f9f9a554417e6802ec3861287b39792eb62c8092c5939ff0f0a`.

Corrected worktree binary: `.build/gohawk-followup78-locks-v3`, SHA256
`b1e2a94173b6034be3d177524b4d3fb21c0fe481440f3a7c034a8e5676e5579d`.
It contains the lock changes reviewed here; its uncommitted provenance is
explicit rather than stamped as the clean baseline commit. The root task owns
the final focused commit and repository-wide gate. After that binary was built,
targeted lint required moving the identical branch-trace helper into `state.go`
and extracting the trace assertion; neither changed the lock proof.

Receipts: `.build/followup337-locks-followup78-baseline/` and
`.build/followup337-locks-followup78-v3/`. Each asserts the original full
repository pin, selects the exact reviewed check IDs with `-enable-checks`,
includes test files statically, records binary checksum, command, successful
load status, and complete diagnostics. Scans were sequential, with
`CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, and readonly modules. No
candidate tests or applications ran.

The canonical direct CLI profile independently confirms both corrected sites
and all four true-positive controls with both immutable binaries:
`gohawk -enable-all -gohawk-include-tests -json .`. All eight invocations loaded
successfully. Both surf sites reproduce on baseline and disappear on v3;
ContainerSSH `server.go:246:3` and `:494:3`, WireGuard
`noise-protocol.go:542:3`, and uTLS `common.go:1075:4` remain reported by both.
These receipts are in `.build/followup78-locks-canonical/`; they prevent
check-selection differences from being counted as fixes.

Static SSA evidence includes bazel-remote `availableOrTryProxy`, file.d
`addJob`, and gocryptfs `Write` in `.build/followup78-locks-*-ssa*.txt`.
The enetx CLI function dump did not expose the relevant private receiver
method; its source, minimized failing fixture, and pinned replay establish
that correction instead. A gocryptfs dump attempted with cgo first failed for
missing libcrypto; the successful cgo-disabled dump supersedes it.

Focused validation passed:

- `go test ./internal/analyzers/concurrency/lockorder -count=1`
- `go test -race ./internal/analyzers/concurrency/lockorder -count=1`
- `go test ./internal/architecture -count=1`
- Targeted canonical golangci-lint run: zero issues after the trace-only extraction.
- Explicit-check baseline and corrected replay: 25/25 baseline sites present,
  2 corrected sites absent, 4/4 available true-positive controls retained.
- Direct CLI all-check confirmation: both corrections and all four controls
  match the explicit-check result, with no loader failures.

Tier2 graph verification used project `home-james-scratch-gohawk`; relevant
source paths had no recorded coverage issue, with direct source verification
after edits. External pinned checkouts were read directly and not indexed.
