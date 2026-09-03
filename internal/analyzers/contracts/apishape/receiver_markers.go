package apishape

import (
	"go/ast"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

// This file owns the narrow proof that a method exists only to mark an
// interface implementation. It stops before methods with behavior, data flow,
// or a signature that callers can use as an ordinary operation.

const emptyInterfaceMarkerReason = "empty-interface-marker"

type receiverFormProof struct {
	contributes bool
	reason      string
}

func proveReceiverFormEvidence(pass *analysis.Pass, declaration *ast.FuncDecl, interfaces []*types.Interface) receiverFormProof {
	if !emptyValueReceiverMethod(pass, declaration) {
		return receiverFormProof{contributes: true}
	}
	function := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	receiver := function.Signature().Recv().Type()
	for _, iface := range interfaces {
		method := apiShapeInterfaceMethod(iface, function.Name())
		if method == nil || method.Pkg() != pass.Pkg || method.Id() != function.Id() {
			continue
		}
		if types.Implements(types.NewPointer(receiver), iface) {
			// Memefish uses empty value-receiver methods to seal AST sum-type
			// interfaces while all state-observing Node methods use pointers. The
			// marker has no receiver semantics to make inconsistent, and *T's exact
			// interface implementation proves its protocol role.
			// https://github.com/cloudspannerecosystem/memefish/blob/e6a536f2179df084186a61698deafcdf27a686ea/ast/ast.go#L60-L78
			return receiverFormProof{reason: emptyInterfaceMarkerReason}
		}
	}
	return receiverFormProof{contributes: true}
}

func emptyValueReceiverMethod(pass *analysis.Pass, declaration *ast.FuncDecl) bool {
	if declaration.Name.IsExported() || declaration.Body == nil || len(declaration.Body.List) != 0 {
		return false
	}
	function, ok := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if !ok {
		return false
	}
	signature := function.Signature()
	if signature.Params().Len() != 0 || signature.Results().Len() != 0 {
		return false
	}
	_, pointer := signature.Recv().Type().(*types.Pointer)
	return !pointer
}

func traceReceiverFormDecision(pass *analysis.Pass, declaration *ast.FuncDecl, proof receiverFormProof) {
	checkID := string(check.APIMixedReceivers)
	if !trace.Enabled("apishape", checkID) {
		return
	}
	receiver, _ := receiverName(declaration.Recv.List[0].Type)
	trace.For(pass, "apishape", checkID, declaration.Name.Pos()).Evidence(trace.Step{
		Reason:   proof.reason,
		Outcome:  trace.OutcomeAccepted,
		Pos:      declaration.Name.Pos(),
		Function: declaration.Name.Name,
		Details: map[string]string{
			"receiver": receiver,
		},
	})
}
