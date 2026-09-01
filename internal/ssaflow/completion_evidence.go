package ssaflow

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// CompletionMode selects lifecycle-completion relationships accepted by an
// analyzer. Modes are policy: callers should enable only the relationships
// that settle their particular ownership obligation.
type CompletionMode uint16

const (
	CompletionDeferred CompletionMode = 1 << iota
	CompletionInClosure
	CompletionBeforeBranch
	CompletionByHelper
	CompletionInStartedClosure
	CompletionByStartedHelper
	CompletionInCalledClosureBeforeBranch
	CompletionByDeferredHelperCallback
	CompletionByDeferredArgument
)

// CompletionRequest describes lifecycle completion to prove for one or more
// equivalent cleanup methods.
type CompletionRequest struct {
	Instruction ssa.Instruction
	Target      ssa.Value
	Methods     []string
	Modes       CompletionMode
}

// ProveCompletion returns the first concrete relationship, in precision-first
// order, that proves the requested lifecycle completion.
func ProveCompletion(request CompletionRequest) CompletionProof {
	var query EvidenceQuery
	return query.Completion(request)
}

// Completion proves and memoizes a lifecycle-completion request.
func (query *EvidenceQuery) Completion(request CompletionRequest) CompletionProof {
	key := completionEvidenceKey{
		instruction: request.Instruction,
		target:      request.Target,
		methods:     completionMethodsKey(request.Methods),
		modes:       request.Modes,
	}
	if proof, ok := query.completions[key]; ok {
		return proof
	}
	proof := proveCompletion(request)
	if query.completions == nil {
		query.completions = make(map[completionEvidenceKey]CompletionProof)
	}
	query.completions[key] = proof
	return proof
}

func completionMethodsKey(methods []string) string {
	if len(methods) == 1 {
		return methods[0]
	}
	return strings.Join(methods, "\x00")
}

func proveCompletion(request CompletionRequest) CompletionProof {
	if request.Instruction == nil || request.Target == nil || len(request.Methods) == 0 || request.Modes == 0 {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	for _, method := range request.Methods {
		if proof := proveMethodCompletion(request, method); proof.Proven() {
			return proof
		}
	}
	if completionEvidenceUnavailable(request) {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	return CompletionProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}}
}

func proveMethodCompletion(request CompletionRequest, method string) CompletionProof {
	if request.Modes&CompletionDeferred != 0 && DeferredClosureCalls(request.Instruction, method, request.Target) {
		return completionProof(EvidenceDeferredCompletion, method)
	}
	if request.Modes&CompletionByDeferredArgument != 0 &&
		DeferredClosureCallsMethodOnDerivedArgumentOnEveryReturn(request.Instruction, method, request.Target) {
		return completionProof(EvidenceDeferredArgumentCompletion, method)
	}
	if request.Modes&CompletionByDeferredHelperCallback != 0 &&
		DeferredHelperInvokesBoundMethodOnEveryReturn(request.Instruction, method, request.Target) {
		return completionProof(EvidenceDeferredHelperCallback, method)
	}
	if proof := proveBeforeBranchCompletion(request, method); proof.Proven() {
		return proof
	}
	if request.Modes&CompletionByHelper != 0 && CallCallsMethodOnArgumentOnEveryReturn(request.Instruction, method, request.Target) {
		return completionProof(EvidenceHelperCompletion, method)
	}
	if request.Modes&CompletionInStartedClosure != 0 && StartedClosureCallsMethodOnEveryReturn(request.Instruction, method, request.Target) {
		return completionProof(EvidenceStartedCompletion, method)
	}
	if request.Modes&CompletionByStartedHelper != 0 && StartedClosureCallsMethodViaHelper(request.Instruction, method, request.Target) {
		return completionProof(EvidenceStartedHelperCompletion, method)
	}
	if request.Modes&CompletionInClosure != 0 && ClosureCallsMethod(request.Instruction, method, request.Target) {
		return completionProof(EvidenceClosureCompletion, method)
	}
	return CompletionProof{}
}

func proveBeforeBranchCompletion(request CompletionRequest, method string) CompletionProof {
	if request.Modes&CompletionBeforeBranch != 0 && ClosureCallsMethodBeforeBranch(request.Instruction, method, request.Target) {
		return completionProof(EvidenceCompletionBeforeBranch, method)
	}
	if request.Modes&CompletionInCalledClosureBeforeBranch != 0 &&
		CalledClosureCallsMethodBeforeBranch(request.Instruction, method, request.Target) {
		return completionProof(EvidenceCalledCompletionBeforeBranch, method)
	}
	return CompletionProof{}
}

func completionProof(reason EvidenceReason, method string) CompletionProof {
	return CompletionProof{Proof{State: EvidenceProven, Reason: reason, Method: method, Provenance: EvidenceFromLocalSSA}}
}

func completionEvidenceUnavailable(request CompletionRequest) bool {
	common, _, function := calledFunction(request.Instruction)
	if request.Modes&(CompletionDeferred|CompletionInClosure|CompletionBeforeBranch|CompletionInStartedClosure|CompletionByStartedHelper|
		CompletionInCalledClosureBeforeBranch|CompletionByDeferredHelperCallback|CompletionByDeferredArgument) != 0 &&
		(function == nil || len(function.Blocks) == 0) {
		return true
	}
	return request.Modes&CompletionByHelper != 0 && (common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0)
}
