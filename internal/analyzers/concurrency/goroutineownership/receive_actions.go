package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Receive evidence distinguishes exact completion handles from possible
// aggregate/alias matches. Selected arms settle only their own path; opaque
// identity remains unknown, and recorded edge reasons project the same labels.

// receivesFrom reports whether instruction receives from a channel accepted by
// matches, through a receive expression, a select case, or a channel range.
func receivesFrom(instruction ssa.Instruction, matches func(ssa.Value) bool) bool {
	return receivesFromWithin(instruction, matches, nil)
}

func receivesFromWithin(instruction ssa.Instruction, matches func(ssa.Value) bool, budget *ssaflow.SearchBudget) bool {
	switch typed := instruction.(type) {
	case *ssa.UnOp:
		return typed.Op == token.ARROW && matches(typed.X)
	case *ssa.Select:
		for _, state := range typed.States {
			if !budget.Spend() {
				return false
			}
			if state.Dir == types.RecvOnly && matches(state.Chan) {
				return true
			}
		}
	case *ssa.Range:
		_, channel := typed.X.Type().Underlying().(*types.Chan)
		return channel && matches(typed.X)
	}
	return false
}

// A mixed select does not settle every path. Its exact receive arm is credited
// on the edge, or at an unambiguous arm entry, keeping other cases open.
// https://github.com/containerd/stargz-snapshotter/blob/624678b4e421947534cbf0618f9609853cccee0f/store/manager.go#L193-L221
func guaranteedReceive(instruction ssa.Instruction, matches func(ssa.Value) bool) bool {
	if selectedReceiveAtEntry(instruction, matches) {
		return true
	}
	if choice, ok := instruction.(*ssa.Select); ok {
		return choice.Blocking && len(choice.States) > 0 && !slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
			return state.Dir != types.RecvOnly || !matches(state.Chan)
		})
	}
	return receivesFrom(instruction, matches)
}

func selectedReceiveAtEntry(instruction ssa.Instruction, matches func(ssa.Value) bool) bool {
	block := instruction.Block()
	if len(block.Instrs) == 0 || block.Instrs[0] != instruction {
		return false
	}
	channel, selected := ssaflow.SelectedReceiveChannel(block)
	return selected && matches(channel)
}

func (analysis *spawnAnalysis) selectedJoinEdge(from, to *ssa.BasicBlock) bool {
	channel, selected := ssaflow.SelectedReceiveOnEdge(from, to)
	joined := selected && analysis.isSignal(channel)
	if joined {
		analysis.recordEdge(from, to, reasonSelectedReceiveEdge, ssaflow.ObligationExact)
	}
	return joined
}

// recordEdge keeps the authoritative reason and action for the trace. Tracing
// projects the action instead of reconstructing proof strength from reason names.
func (analysis *spawnAnalysis) recordEdge(from, to *ssa.BasicBlock, reason goroutineOwnershipReason, action ssaflow.ObligationAction) {
	if !analysis.tracing {
		return
	}
	if analysis.edgeEvidence == nil {
		analysis.edgeEvidence = make(map[[2]int]joinEdgeEvidence)
	}
	analysis.edgeEvidence[[2]int{from.Index, to.Index}] = joinEdgeEvidence{reason: reason, action: action}
}

// selectSends reports whether a select statement offers a tracked value on a
// send case; like a plain send, the receiver may retain it.
func (analysis *spawnAnalysis) selectSends(instruction ssa.Instruction) bool {
	choice, ok := instruction.(*ssa.Select)
	if !ok {
		return false
	}
	return slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
		return state.Dir == types.SendOnly && analysis.consumes(state.Send)
	})
}

// isSignal requires exact channel identity. An aggregate root or a phi with
// one matching alternative explains possible completion, never an exact join.
func (analysis *spawnAnalysis) isSignal(value ssa.Value) bool {
	return analysis.isSignalWithin(value, analysis.budget())
}

// isSignalWithin shares exact channel identity between observations and relay
// discovery. Aggregate roots retain possible ownership, never an exact join.
func (analysis *spawnAnalysis) isSignalWithin(value ssa.Value, budget *ssaflow.SearchBudget) bool {
	if !ssaflow.ChannelType(value) {
		return false
	}
	storage := heapmodel.NewStorage(budget)
	for _, signal := range analysis.signals {
		if !budget.Spend() {
			return false
		}
		if ssaflow.ChannelType(signal) && storage.Same(value, signal).Proven() {
			return true
		}
	}
	return false
}

// possibleSignal retains the previous broad receive boundary for unknown
// ownership and guarded joins. Matching sibling fields/indexes does not show
// that the caller observed this worker's completion handle.
func (analysis *spawnAnalysis) possibleSignal(value ssa.Value) bool {
	if heapmodel.MayAliasAny(value, analysis.signals) {
		return true
	}
	root := aggregateRoot(value)
	return root != value && slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
		if !ssaflow.ChannelType(signal) {
			// Captured slice cells and their loaded slice share an aggregate root.
			// Index correlation stays unknown under countedJoin, not proven exact.
			// https://github.com/bazel-contrib/buildtools/blob/933e9bbe17f7619afaca1dd58ce22810042f1c13/buildifier/buildifier.go#L241-L268
			return heapmodel.MayAlias(ssaflow.CapturedBindingValue(root), ssaflow.CapturedBindingValue(aggregateRoot(signal)))
		}
		signalRoot := aggregateRoot(signal)
		return signalRoot != signal && heapmodel.MayAlias(root, signalRoot)
	})
}

// A worker-side field can resolve only to the constructor's aggregate result,
// while its caller receives the original channel passed to that constructor.
// Containment cannot identify the exact field, so this is unknown, not a join.
// https://github.com/lotusirous/go-concurrency-patterns/blob/791337ff11e69cd9d1a8587e53474a8469b3510f/17-ring-buffer-channel/main.go#L49-L62
func (analysis *spawnAnalysis) signalAggregateCarries(value ssa.Value) bool {
	return slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
		return !ssaflow.ChannelType(signal) && carries(ssaflow.NewReachingWalk(carryForms), signal, value)
	})
}
