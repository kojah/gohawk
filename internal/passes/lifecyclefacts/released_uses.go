package lifecyclefacts

import (
	"fmt"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A released use is a method called on a parameter after the function has
// already called a cleanup method on it: on every path to the use that the
// case's condition allows, the release came first, and nothing else touched
// the parameter in between. With an empty condition the function misuses
// whatever it is handed, so the use is wrong at its own position. With a
// condition on the function's Boolean parameters it is latent, in Infer
// Pulse's sense: wrong only at a call that supplies those constants, so only
// such a call is. The claim is structural. Which uses fail on a released
// value is the consuming analyzer's contract, not this package's.
//
// The proof is deliberately narrow: the release and the use are direct calls
// on the exact parameter, the release dominates the use on the allowed
// paths, and a release on a branch the function decides by its own data
// does not count, because the use is then not wrong on every path. A helper
// that releases or uses the parameter is not followed yet.

// maxReleasedUses bounds the released uses one function exports.
const maxReleasedUses = 8

// ReleasedUse records that the function calls Use on parameter Parameter
// after calling Release on it, on every path to that use the case Condition
// allows.
type ReleasedUse struct {
	Condition ssaflow.CallCondition
	Parameter int
	Release   string
	Use       string
}

// ReleasedUseProof is a released use with the calls that establish it.
type ReleasedUseProof struct {
	ReleasedUse
	ReleaseCall *ssa.Call
	UseCall     *ssa.Call
}

// releasedUseProofs proves the function's released uses, most general
// condition first; a use implied by a proven one under fewer assumptions is
// not repeated.
func releasedUseProofs(function *ssa.Function) []ReleasedUseProof {
	if function == nil || len(function.Blocks) == 0 {
		return nil
	}
	assignments := append([]ssaflow.ArgumentConstants{{}}, argumentAssignments(guardingParameters(function))...)
	var proofs []ReleasedUseProof
	for _, assignment := range assignments {
		constants, ok := assignment.Bindings(function)
		if !ok {
			continue
		}
		condition := ssaflow.CallCondition{Arguments: assignment}
		blocks := ssaflow.ReachableBlocksAssuming(function, constants)
		for index, parameter := range function.Params {
			if index >= 64 || len(proofs) >= maxReleasedUses {
				break
			}
			proofs = parameterReleasedUses(proofs, parameter, index, condition, blocks, constants)
		}
	}
	return proofs
}

// parameterReleasedUses appends the released uses of one parameter under one
// condition to proofs, up to the export bound.
func parameterReleasedUses(
	proofs []ReleasedUseProof, parameter *ssa.Parameter, index int, condition ssaflow.CallCondition,
	blocks []*ssa.BasicBlock, constants ssaflow.BooleanConstants,
) []ReleasedUseProof {
	releases, uses := parameterCalls(blocks, parameter)
	for _, use := range uses {
		for _, release := range releases {
			if len(proofs) == maxReleasedUses {
				return proofs
			}
			candidate := ReleasedUse{
				Condition: condition, Parameter: index,
				Release: ssaflow.CallName(release.Common()), Use: ssaflow.CallName(use.Common()),
			}
			if releasedUseImplied(candidate, proofs) || !ssaflow.InstructionDominatesAssuming(release, use, constants) ||
				touchedBetween(parameter, release, use, constants) {
				continue
			}
			proofs = append(proofs, ReleasedUseProof{ReleasedUse: candidate, ReleaseCall: release, UseCall: use})
		}
	}
	return proofs
}

// parameterCalls returns the plain calls in blocks made on the exact
// parameter: cleanup methods as releases, every other method as a use.
// Deferred calls run at return and precede nothing.
func parameterCalls(blocks []*ssa.BasicBlock, parameter *ssa.Parameter) (releases, uses []*ssa.Call) {
	for _, block := range blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok || ssaflow.CallReceiver(call.Common()) != parameter {
				continue
			}
			if slices.Contains(cleanupMethods, ssaflow.CallName(call.Common())) {
				releases = append(releases, call)
			} else {
				uses = append(uses, call)
			}
		}
	}
	return releases, uses
}

func releasedUseImplied(candidate ReleasedUse, proven []ReleasedUseProof) bool {
	for _, earlier := range proven {
		if earlier.Parameter == candidate.Parameter && earlier.Release == candidate.Release && earlier.Use == candidate.Use &&
			earlier.Condition.Matches(candidate.Condition) {
			return true
		}
	}
	return false
}

// touchedBetween reports whether some instruction other than the release
// uses the parameter on an allowed path from the release to the use. The
// walk stops at the use, so work after it, such as the rest of a loop body,
// is not between them.
func touchedBetween(parameter *ssa.Parameter, release, use *ssa.Call, constants ssaflow.BooleanConstants) bool {
	type position struct {
		block *ssa.BasicBlock
		index int
	}
	touched := false
	ssaflow.WalkStates([]position{{block: release.Block(), index: ssaflow.InstructionIndex(release) + 1}},
		func(state position) *ssa.BasicBlock { return state.block },
		func(state position) ([]position, bool) {
			for _, instruction := range state.block.Instrs[state.index:] {
				if instruction == use {
					return nil, true
				}
				if instruction != release && usesValue(instruction, parameter) {
					touched = true
					return nil, false
				}
			}
			var next []position
			for _, successor := range constants.Narrow(state.block.Succs, state.block) {
				next = append(next, position{block: successor})
			}
			return next, true
		})
	return touched
}

func usesValue(instruction ssa.Instruction, value ssa.Value) bool {
	if _, debug := instruction.(*ssa.DebugRef); debug {
		return false
	}
	for _, operand := range instruction.Operands(nil) {
		if operand != nil && *operand == value {
			return true
		}
	}
	return false
}

func (fact *Fact) releasedUseDescriptions() []string {
	var lines []string
	for _, use := range fact.ReleasedUses {
		line := fmt.Sprintf("released use: parameter %d calls %s after %s", use.Parameter, use.Use, use.Release)
		if arguments := use.Condition.Arguments; arguments.Bound != 0 {
			line += fmt.Sprintf(" when arguments %#x are %#x", arguments.Bound, arguments.Values)
		}
		lines = append(lines, line)
	}
	return lines
}

func releasedUses(proofs []ReleasedUseProof) []ReleasedUse {
	uses := make([]ReleasedUse, 0, len(proofs))
	for _, proof := range proofs {
		uses = append(uses, proof.ReleasedUse)
	}
	return uses
}

// calleeReleasedUses returns the released uses of the call's callee: its
// summary's, or, for a helper of this package that is not summarized, the
// same proof run on its body.
func (evidence *LifecycleEvidence) calleeReleasedUses(instruction ssa.Instruction) []ReleasedUse {
	if fact, ok := factFor(evidence.pass, instruction); ok {
		return fact.ReleasedUses
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return nil
	}
	callee := common.StaticCallee()
	if callee == nil || len(callee.Blocks) == 0 || callee.Pkg != instruction.Parent().Pkg {
		return nil
	}
	if uses, ok := evidence.localReleasedUses[callee]; ok {
		return uses
	}
	if evidence.localReleasedUses == nil {
		evidence.localReleasedUses = map[*ssa.Function][]ReleasedUse{}
	}
	uses := releasedUses(releasedUseProofs(callee))
	evidence.localReleasedUses[callee] = uses
	return uses
}

// ManifestReleasedUses returns the released uses of a function in this
// package that hold with no condition, with the calls that establish them.
func (evidence *LifecycleEvidence) ManifestReleasedUses(function *ssa.Function) []ReleasedUseProof {
	var manifest []ReleasedUseProof
	for _, proof := range releasedUseProofs(function) {
		if proof.Condition.Unconditional() {
			manifest = append(manifest, proof)
		}
	}
	return manifest
}

// ReleasedUsesAt returns the latent released uses a call triggers: those of
// its callee's summary whose condition the call's constant arguments satisfy.
// An unconditional released use is the callee's own defect and is not
// returned here.
func (evidence *LifecycleEvidence) ReleasedUsesAt(instruction ssa.Instruction) []ReleasedUse {
	query := ssaflow.CallCondition{Arguments: suppliedConstants(instruction, nil)}
	if query.Arguments.Bound == 0 {
		return nil
	}
	var triggered []ReleasedUse
	for _, use := range evidence.calleeReleasedUses(instruction) {
		if !use.Condition.Unconditional() && use.Condition.Matches(query) {
			triggered = append(triggered, use)
		}
	}
	return triggered
}
