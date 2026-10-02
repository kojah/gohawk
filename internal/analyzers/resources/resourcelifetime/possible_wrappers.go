package resourcelifetime

import (
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Possible wrapper evidence owns constructor chains and their publication
// boundaries. Structural visits share one allowance; call effects and alias,
// graph and type internals remain independent. A wrapper only makes ownership
// uncertain and never proves cleanup or exact resource identity.

// proveWrapperStoredOnForeignOwnerWithin reports whether a store hands a constructor chain
// over the resource to an object this function did not allocate: a field or
// element of a parameter, a global, or an object a call returned. The resource
// then lives as long as that object, so its release is no longer this
// function's to prove. A logger routed into a server's error log is the
// common case. A chain stored into a local allocation stays owed here.
// https://github.com/1parado/grok-build-switch/blob/c1ee703bf6000abd92d8d29ca94a4ed59cb510f2/main.go#L173-L176
func (analysis *resourceAnalysis) proveWrapperStoredOnForeignOwnerWithin(store *ssa.Store, budget *ssaflow.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if heapmodel.MayAlias(store.Val, analysis.resource) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	wrapper := analysis.provePossibleWrapperWithin(store.Val, maxWrapperChain, true, budget)
	if wrapper.State != ssaflow.EvidenceProven {
		return wrapper
	}
	root := store.Addr
	for {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		switch address := root.(type) {
		case *ssa.FieldAddr:
			root = address.X
		case *ssa.IndexAddr:
			root = address.X
		case *ssa.Alloc:
			return carriedValueProof(false, resourceReasonUntouched, budget)
		default:
			return carriedValueProof(true, resourceReasonWrapperStoredOnForeignOwner, budget)
		}
	}
}

// proveChainKeptWithin reports whether a callee proven to store its argument
// receives a chain of constructors that holds the resource. A logger over a
// file is the common case: slog.SetDefault stores the logger that keeps the
// handler that keeps the MultiWriter that keeps the file, so the file now
// belongs to the process. Unlike a single wrapper, a chain counts only at a
// callee proven to store it: a method call on the logger with unknown effects,
// such as Info, does not publish the file, and a chain the function merely
// uses still leaves it owed.
func (analysis *resourceAnalysis) proveChainKeptWithin(instruction ssa.Instruction, common *ssa.CallCommon, budget *ssaflow.SearchBudget) resourceProof {
	for index, argument := range common.Args {
		if !budget.Spend() {
			return aggregateEscapeProof(false, budget)
		}
		wrapper := analysis.provePossibleWrapperWithin(argument, maxWrapperChain, false, budget)
		if wrapper.State == ssaflow.EvidenceUnknown {
			return wrapper
		}
		if !wrapper.Proven() {
			continue
		}
		// Only the strict stored claim proves publication; may-retain effects also
		// describe an opaque interface call on a borrowed logger handler.
		if !budget.Spend() {
			return aggregateEscapeProof(false, budget)
		}
		stored, _ := analysis.evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimStores)
		if stored {
			return aggregateEscapeProof(true, budget)
		}
	}
	return aggregateEscapeProof(false, budget)
}

// maxWrapperChain bounds how many constructors a wrapper chain may nest.
const maxWrapperChain = 4

// provePossibleWrapperWithin reports whether value is a call result that may keep the
// resource: an argument holds it inside an aggregate, or, below the outermost
// call when depth allows a chain, the argument is the resource itself or
// another such wrapper. Every step needs its own callee to keep the argument,
// or to be unknown. A wrapper may receive the resource inside a variadic
// aggregate instead of as a direct operand. This is only an
// opaque-consumption query; it never establishes exact identity or that the
// wrapper owns cleanup.
// https://github.com/Mmx233/BitSrunLoginGo/blob/a744f312b3835f329eb98e45c8d19bc2a5b7d4c0/internal/config/log.go#L58-L59
// https://github.com/inkdust2021/VibeGuard/blob/12a46784a7ebca95f7765178edd8344a974da849/internal/log/log.go#L42-L47
func (analysis *resourceAnalysis) provePossibleWrapperWithin(value ssa.Value, depth int, direct bool, budget *ssaflow.SearchBudget) resourceProof {
	call, ok := unwrapWrapperWithin(value, budget).(*ssa.Call)
	if !ok {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if _, scalar := call.Type().Underlying().(*types.Basic); scalar || syntax.IsErrorType(call.Type()) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if depth > 0 {
		appended := analysis.proveAppendedWrapperWithin(call, depth, budget)
		if appended.State != ssaflow.EvidenceDisproven {
			return appended
		}
	}
	for _, argument := range call.Common().Args {
		held := analysis.proveWrapperArgumentWithin(argument, depth, direct, budget)
		if held.State == ssaflow.EvidenceUnknown {
			return held
		}
		if !held.Proven() {
			continue
		}
		// A visible transformation that does not retain its input is not a
		// wrapper. Missing effects are unknown, never a purity claim.
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		effects := analysis.evidence.CallEffects(call, argument)
		if !effects.Proven() || effects.Effects&ssaflow.EffectRetain != 0 {
			return carriedValueProof(true, resourceReasonWrapperMayCarry, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// Append's explicit values live in a compiler-built variadic array. Follow
// their wrapper chains when this slice is published, without treating an
// append to a discarded local collection as a handoff. Spread slices remain
// outside this bounded query.
// https://github.com/twmb/kcl/blob/5290cb05bcc421a239e327ba11408bc4e27bd2dd/client/client.go#L1445-L1456
func (analysis *resourceAnalysis) proveAppendedWrapperWithin(call *ssa.Call, depth int, budget *ssaflow.SearchBudget) resourceProof {
	values, explicit := ssaflow.AppendedValuesWithin(call, budget)
	if resourceFlowExhausted(budget) || !explicit {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	for _, value := range values {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		wrapper := analysis.provePossibleWrapperWithin(value, depth-1, true, budget)
		if wrapper.State != ssaflow.EvidenceDisproven {
			return wrapper
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// The outermost constructor excludes a direct resource unless its caller
// explicitly selects direct mode. Inner steps may accept it, with the original
// four-step cap. Derived stored values keep their broader nested policy.
func (analysis *resourceAnalysis) proveWrapperArgumentWithin(argument ssa.Value, depth int, direct bool, budget *ssaflow.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	exact := heapmodel.MayAlias(argument, analysis.resource)
	if !exact {
		nested := analysis.proveNestedCarryWithin(argument, budget)
		if nested.State != ssaflow.EvidenceDisproven {
			return nested
		}
	}
	if direct && exact {
		return carriedValueProof(true, resourceReasonWrapperMayCarry, budget)
	}
	if depth > 0 {
		return analysis.provePossibleWrapperWithin(argument, depth-1, true, budget)
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// unwrapWrapperWithin peels the interface conversions a wrapper passes through on
// its way to the next constructor, such as a handler boxed as slog.Handler.
func unwrapWrapperWithin(value ssa.Value, budget *ssaflow.SearchBudget) ssa.Value {
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentMakeInterface
	for {
		if !budget.Spend() {
			return nil
		}
		inner, ok := ssaflow.UnwrapTransparentValue(value, forms)
		if !ok {
			return value
		}
		value = inner
	}
}
