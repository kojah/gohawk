// Package borrowedstorage implements the borrowedstorage gohawk analyzer.
package borrowedstorage

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

const analyzerName = "borrowedstorage"

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     analyzerName,
		Doc:      "checks borrowed mutable storage transferred to a second owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runBorrowedStorage,
	}
}

func runBorrowedStorage(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportBufferOwnershipConflicts(pass, function)
	}
	return nil, nil
}

type borrowedView struct {
	value  ssa.Value
	owner  ssa.Value
	method string
}

func reportBufferOwnershipConflicts(pass *analysis.Pass, function *ssa.Function) {
	views := borrowedBufferViews(function)
	if len(views) == 0 {
		return
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok || !ssaflow.CallMatchesSymbol(call.Common(), syntax.PackageFunction("bytes", "NewBuffer")) ||
				len(call.Common().Args) != 1 {
				continue
			}
			for _, view := range views {
				if !ssaflow.SameValue(call.Common().Args[0], view.value) {
					continue
				}
				reportBufferOwnershipDecision(pass, function, call, view)
				break
			}
		}
	}
}

func borrowedBufferViews(function *ssa.Function) []borrowedView {
	viewMethods := []syntax.Symbol{
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "bytes", Receiver: "Buffer", Name: "Bytes"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "bytes", Receiver: "Buffer", Name: "Next"}),
		syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "bytes", Receiver: "Buffer", Name: "AvailableBuffer"}),
	}
	var views []borrowedView
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if !ok || !ssaflow.CallMatchesAnySymbol(call.Common(), viewMethods...) {
				continue
			}
			owner := ssaflow.CallReceiver(call.Common())
			if owner != nil {
				views = append(views, borrowedView{value: call, owner: owner, method: ssaflow.CallName(call.Common())})
			}
		}
	}
	return views
}

func reportBufferOwnershipDecision(pass *analysis.Pass, function *ssa.Function, call *ssa.Call, view borrowedView) {
	sourceOutlivesCall := ssaflow.ExternallyOwnedValue(view.owner) || bufferOwnerEscapes(function, view.owner)
	newOwnerEscapes := bufferOwnerEscapes(function, call)
	reason := "overlapping-buffer-owners"
	outcome := analysisTrace.OutcomeRejected
	if !sourceOutlivesCall {
		reason = "source-owner-local"
		outcome = analysisTrace.OutcomeAccepted
	} else if !newOwnerEscapes {
		reason = "new-owner-local"
		outcome = analysisTrace.OutcomeAccepted
	}
	emitBufferOwnershipDecision(pass, function, call, view, reason, outcome)
	if outcome == analysisTrace.OutcomeAccepted {
		return
	}

	// bytes.NewBuffer takes ownership of its argument, but Buffer view methods
	// leave that same storage under the source buffer's control. Returning or
	// storing the new buffer therefore creates two mutable owners. This caused
	// Resty's saved request body to be overwritten after the original buffer was
	// reused: https://github.com/go-resty/resty/blob/0451c4c63033b1a330b890cd543c97501b8684b9/middleware.go#L514-L518
	check.Reportf(
		pass,
		check.BorrowedStorageOwner,
		call.Pos(),
		"bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first",
	)
}

func bufferOwnerEscapes(function *ssa.Function, owner ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if returned, ok := instruction.(*ssa.Return); ok && ssaflow.ReturnedValueOwnsValue(returned, owner) {
				return true
			}
			if ssaflow.StoresValueInEscapingField(instruction, owner) ||
				ssaflow.StoresValueInGlobal(instruction, owner) ||
				ssaflow.StoresValueInEnclosingScope(instruction, owner) {
				return true
			}
		}
	}
	return false
}

func emitBufferOwnershipDecision(
	pass *analysis.Pass,
	function *ssa.Function,
	call *ssa.Call,
	view borrowedView,
	reason string,
	outcome analysisTrace.Outcome,
) {
	checkID := string(check.BorrowedStorageOwner)
	analysisTrace.For(pass, analyzerName, checkID, call.Pos()).Decision(analysisTrace.Step{
		Reason:   reason,
		Outcome:  outcome,
		Pos:      call.Pos(),
		Function: function.String(),
		Details:  map[string]string{"source": "bytes.Buffer." + view.method, "owner": "bytes.NewBuffer"},
	})
}
