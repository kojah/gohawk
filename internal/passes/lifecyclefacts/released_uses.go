package lifecyclefacts

import (
	"fmt"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/resourcemodel"
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
// The proof is deliberately narrow: the release dominates the use on the
// allowed paths, and a release on a branch the function decides by its own
// data does not count, because the use is then not wrong on every path. A
// release is a cleanup call on the exact parameter, or a helper the completion
// engine proves calls one on it on every return, under the constants the
// case fixes. A use is any other method called on it, or a helper whose
// summary requires one on it on every path. Any other call that receives the
// parameter could reopen, reset, or replace it, so it cancels the claim.
//
// A function that forwards its own Boolean parameter to a callee with a
// latent released use has that use too, under its own condition, so the
// defect surfaces at the call that finally fixes the flag. A callee's latent
// use that the call's literals trigger by themselves is that call's own
// defect, reported there, and is not composed.

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

// releasedUseSearch carries what the proof reads beyond the body: callee
// facts, and the released uses and heap requirements of this package's
// unexported helpers, which have no fact. The memo owns the recursion guard
// for helpers that forward to each other.
type releasedUseSearch struct {
	facts func(ssa.Instruction) (Fact, bool)
	// budget bounds the helper proofs of the body being proved; a body that
	// exhausts it claims only what it proved before.
	budget *ssaflow.SearchBudget
	memo   *ssaflow.CallGraphMemo[*ssa.Function, []ReleasedUse]
	heaps  map[*ssa.Function]*heapmodel.HeapSummary
}

func newReleasedUseSearch(facts func(ssa.Instruction) (Fact, bool)) *releasedUseSearch {
	return &releasedUseSearch{
		facts: facts,
		memo:  ssaflow.NewCallGraphMemo[*ssa.Function, []ReleasedUse](),
		heaps: map[*ssa.Function]*heapmodel.HeapSummary{},
	}
}

// releasedUseProofs proves the function's released uses, most general
// condition first; a use implied by a proven one under fewer assumptions is
// not repeated.
func (search *releasedUseSearch) releasedUseProofs(function *ssa.Function) []ReleasedUseProof {
	// Only a parameter whose type has a cleanup method can be released, so a
	// body without one has nothing to prove and costs nothing.
	if function == nil || len(function.Blocks) == 0 || !slices.ContainsFunc(function.Params, releasable) {
		return nil
	}
	outer := search.budget
	search.budget = ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	defer func() { search.budget = outer }()
	assignments := append([]ssaflow.CallCondition{{}}, argumentAssignments(guardingParameters(function))...)
	var proofs []ReleasedUseProof
	for _, condition := range assignments {
		constants, ok := condition.Bindings(function)
		if !ok {
			continue
		}
		blocks := ssaflow.ReachableBlocksAssuming(function, constants)
		for index, parameter := range function.Params {
			if index >= 64 || len(proofs) >= maxReleasedUses {
				break
			}
			if releasable(parameter) {
				proofs = search.parameterReleasedUses(proofs, parameter, index, condition, blocks, constants)
			}
		}
		if !condition.Unconditional() {
			proofs = search.forwardedReleasedUses(proofs, function, condition, blocks, constants)
		}
	}
	return proofs
}

// parameterReleasedUses appends the released uses of one parameter under one
// condition to proofs, up to the export bound.
func (search *releasedUseSearch) parameterReleasedUses(
	proofs []ReleasedUseProof, parameter *ssa.Parameter, index int, condition ssaflow.CallCondition,
	blocks []*ssa.BasicBlock, constants ssaflow.FixedValues,
) []ReleasedUseProof {
	releases, uses := search.parameterCalls(blocks, parameter, constants)
	for _, use := range uses {
		for _, release := range releases {
			if len(proofs) == maxReleasedUses {
				return proofs
			}
			if release.call == use.call {
				continue
			}
			candidate := ReleasedUse{Condition: condition, Parameter: index, Release: release.method, Use: use.method}
			release, use := release.call, use.call
			if releasedUseImplied(candidate, proofs) || !ssaflow.InstructionDominatesAssuming(release, use, constants) ||
				touchedBetween(parameter, release, use, constants) {
				continue
			}
			proofs = append(proofs, ReleasedUseProof{ReleasedUse: candidate, ReleaseCall: release, UseCall: use})
		}
	}
	return proofs
}

// releasable reports whether the parameter's type has a cleanup method.
func releasable(parameter *ssa.Parameter) bool {
	return len(releaseMethods(parameter.Type())) != 0
}

// releaseMethods lists the cleanup methods the type has; a callback has none.
func releaseMethods(value types.Type) []string {
	if _, callback := value.Underlying().(*types.Signature); callback {
		return nil
	}
	return conditionalMethods(value)
}

// parameterCall is a call that releases or uses the parameter, with the
// method it calls on it, directly or inside a helper.
type parameterCall struct {
	call   *ssa.Call
	method string
}

// parameterCalls classifies the plain calls in blocks that receive the exact
// parameter: cleanup methods called on it, or proven of a helper, as
// releases; other methods called on it, or required of a helper, as uses.
// Deferred calls run at return and precede nothing.
func (search *releasedUseSearch) parameterCalls(
	blocks []*ssa.BasicBlock, parameter *ssa.Parameter, constants ssaflow.FixedValues,
) (releases, uses []parameterCall) {
	for _, block := range blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			if ssaflow.CallReceiver(call.Common()) == parameter {
				name := ssaflow.CallName(call.Common())
				if slices.Contains(cleanupMethods, name) {
					releases = append(releases, parameterCall{call: call, method: name})
				} else {
					uses = append(uses, parameterCall{call: call, method: name})
				}
				continue
			}
			index := slices.Index(call.Common().Args, ssa.Value(parameter))
			if index < 0 || call.Common().IsInvoke() || call.Common().StaticCallee() == nil {
				continue
			}
			if method, ok := search.helperReleases(call, parameter, constants); ok {
				releases = append(releases, parameterCall{call: call, method: method})
				continue
			}
			for _, method := range search.helperRequires(call, index) {
				if !slices.Contains(cleanupMethods, method) {
					uses = append(uses, parameterCall{call: call, method: method})
				}
			}
		}
	}
	return releases, uses
}

// helperReleases reports the cleanup method the helper call is proven to call
// on the exact parameter on every normal return, under the constants bound in
// the calling body.
func (search *releasedUseSearch) helperReleases(call *ssa.Call, parameter *ssa.Parameter, constants ssaflow.FixedValues) (string, bool) {
	for _, method := range releaseMethods(parameter.Type()) {
		if search.budget.Exhausted() {
			return "", false
		}
		proof := lifecycle.ProveCompletion(lifecycle.CompletionRequest{
			Instruction: call, Target: parameter, Methods: []string{method}, ExactTarget: true, Constants: constants, Budget: search.budget,
			Summarized: conditionalLookup(search.facts, search.budget, nil), CallContract: resourcemodel.ConditionalReleases(search.budget),
		})
		if proof.Proven() && !search.budget.Exhausted() {
			return method, true
		}
	}
	return "", false
}

// helperRequires lists the methods the helper's summary requires on the
// argument at index on every path, read from its fact, or from its heap
// projection when it is an unexported helper of this package.
func (search *releasedUseSearch) helperRequires(call *ssa.Call, index int) []string {
	var heap *heapmodel.HeapSummary
	if fact, ok := search.facts(call); ok {
		heap = fact.Heap
	} else if callee := call.Common().StaticCallee(); len(callee.Blocks) != 0 && callee.Pkg == call.Parent().Pkg {
		if _, ok := search.heaps[callee]; !ok {
			search.heaps[callee] = projectHeap(callee)
		}
		heap = search.heaps[callee]
	}
	if heap == nil {
		return nil
	}
	var methods []string
	for _, requirement := range heap.Requires {
		if requirement.Kind == heapmodel.HeapRequiresMethod && requirement.Slot.Root.Kind == heapmodel.HeapParameter &&
			requirement.Slot.Root.Index == index && requirement.Slot.Path == "" {
			methods = append(methods, requirement.Method)
		}
	}
	return methods
}

// forwardedReleasedUses composes the callees' latent released uses that the
// function's own condition triggers: the call supplies constants that match
// the callee's condition only with the function's bound parameters, and it
// passes the function's own parameter where the callee uses it.
func (search *releasedUseSearch) forwardedReleasedUses(
	proofs []ReleasedUseProof, function *ssa.Function, condition ssaflow.CallCondition,
	blocks []*ssa.BasicBlock, constants ssaflow.FixedValues,
) []ReleasedUseProof {
	for _, block := range blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok {
				continue
			}
			bound := ssaflow.SuppliedCondition(call.Common(), constants)
			literal := ssaflow.SuppliedCondition(call.Common(), nil)
			for _, latent := range search.calleeReleasedUses(call) {
				if latent.Condition.Unconditional() || !latent.Condition.Matches(bound) || latent.Condition.Matches(literal) ||
					latent.Parameter >= len(call.Common().Args) {
					continue
				}
				parameter, ok := call.Common().Args[latent.Parameter].(*ssa.Parameter)
				index := slices.Index(function.Params, parameter)
				if !ok || index < 0 || index >= 64 || len(proofs) == maxReleasedUses {
					continue
				}
				candidate := ReleasedUse{Condition: condition, Parameter: index, Release: latent.Release, Use: latent.Use}
				if !releasedUseImplied(candidate, proofs) {
					proofs = append(proofs, ReleasedUseProof{ReleasedUse: candidate, ReleaseCall: call, UseCall: call})
				}
			}
		}
	}
	return proofs
}

// calleeReleasedUses returns the released uses of the call's static callee:
// its summary's, or, for an unexported helper of this package, the same
// proof run on its body. A helper already on the current path answers
// nothing, which the memo does not retain.
func (search *releasedUseSearch) calleeReleasedUses(call *ssa.Call) []ReleasedUse {
	if fact, ok := search.facts(call); ok {
		return fact.ReleasedUses
	}
	callee := call.Common().StaticCallee()
	if call.Common().IsInvoke() || callee == nil || len(callee.Blocks) == 0 || callee.Pkg != call.Parent().Pkg {
		return nil
	}
	return search.memo.Summarize(callee, callee, nil, func() []ReleasedUse {
		return releasedUses(search.releasedUseProofs(callee))
	}, func(ssaflow.SummaryUnavailable, []ReleasedUse) []ReleasedUse { return nil })
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
func touchedBetween(parameter *ssa.Parameter, release, use *ssa.Call, constants ssaflow.FixedValues) bool {
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
		lines = append(lines, fmt.Sprintf("parameter %d calls %s after %s when %s", use.Parameter, use.Use, use.Release, use.Condition))
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

// releasedUseSearch returns the evidence's search, built on first use.
func (evidence *LifecycleEvidence) releasedUseSearch() *releasedUseSearch {
	if evidence.releasedUses == nil {
		evidence.releasedUses = newReleasedUseSearch(func(instruction ssa.Instruction) (Fact, bool) {
			return factFor(evidence.pass, instruction)
		})
	}
	return evidence.releasedUses
}

// ManifestReleasedUses returns the released uses of a function in this
// package that hold with no condition, with the calls that establish them.
func (evidence *LifecycleEvidence) ManifestReleasedUses(function *ssa.Function) []ReleasedUseProof {
	var manifest []ReleasedUseProof
	for _, proof := range evidence.releasedUseSearch().releasedUseProofs(function) {
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
	query := suppliedCondition(instruction, nil)
	if query.Unconditional() {
		return nil
	}
	var triggered []ReleasedUse
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return nil
	}
	for _, use := range evidence.releasedUseSearch().calleeReleasedUses(call) {
		if !use.Condition.Unconditional() && use.Condition.Matches(query) {
			triggered = append(triggered, use)
		}
	}
	return triggered
}
