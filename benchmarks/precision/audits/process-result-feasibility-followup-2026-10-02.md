# Process result feasibility follow-up — 2026-10-02

Beads: `gohawk-dho.44.11.5.29`. Parent: `cadd4c8`.

## Decision and scope

Process ownership now requests the shared result component and supplies the
provider's feasible successors to both forms of its post-Start ownership walk.
`UnownedReturnQuery` forwards that hook to the existing obligation flow.
No independent result inference, wait classifier, traversal, or receiver-state
proof is introduced. The existing provider also supports proven result-pair
relations; unknown or interrupted inference retains ordinary successors.
Each feasibility question has the provider's existing summary allowance.
This is not a bound for the whole obligation query.

Graph tools were unavailable; material source review covered
`internal/analysis/summaries/results.go`, `internal/engine/ssaflow/flow_paths.go`,
`internal/engine/ssaflow/flow_successors.go`, and the process reporting decision.
This is a bounded review, not a complete architecture audit.

## Controls

`result_feasibility.go` accepts imported always-nil and always-true helpers,
a local always-false helper, and a successful command retained through a phi.
Variable errors, unresolved interface calls and a boxed typed-nil error remain
diagnostic. These result contracts say nothing about process ownership.

- Parent fixture run: `.build/goal-process-results-parent.log`, exit 1 on
  unexpected diagnostics for imported nil/true accepted cases.
- Focused process and ssaflow tests: `.build/goal-process-results-focused.log`,
  exit 0, including all existing controls.
- Counterfactual removing only shared hook forwarding:
  `.build/goal-process-results-no-hook.log`, exit 1 on three unexpected
  diagnostics, including the merged-command case. No load or compile failure.
- Actual merged-command SSA: `.build/goal-process-results-merged.ssa`,
  successful dump with no stderr. It contains a phi of the command and nil,
  followed by the command nil guard and the guaranteed helper result check.

## Pinned production check

Ferro pin: `d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`, package `./mcp`.
Immutable binary: `.build/goal-process-results-reviewed`, SHA-256
`1ddad82258c5d8c5264f322db9008433d6452e4280572346a504ea3f14e449b4`.
CGO disabled, readonly modules, workspace disabled, 180-second command limit.
Direct all-check JSON run exited 3 with diagnostics and empty stderr.
`.build/goal-process-results-ferro.json` is structurally identical to parent
`.build/goal-ferro-state.json`: processownership at stdio.go:154 and
historical goroutineownership at registry.go:760 remain. Formatting differs
by one byte between direct and vet driver output.

No production FP correction is credited. Ferro still needs constructor-dependent
receiver state; its Stdio.Start lacks an unconditional nil result guarantee.
Eleven recorded production FP sites and the Rune publication issue remain open.
No full precision replay was run.

## Canonical validation

`make verify VERIFY_TIMINGS=1` passed, including generation, module verification,
formatting, vet, deadcode, lint, ordinary tests, architecture checks and local
all-check dogfood. Receipt: `.build/goal-process-results-verify.log`.
The generated ssaflow reference includes the new optional successor hook.
The gate ran without a full precision regression replay.
