package resourcelifetime

import (
	"go/constant"
	"strings"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Header-only effects require every visible consumption of the exact writer
// to preserve empty response framing. Opaque, recursive or shortened helper
// evidence cannot prove this contract; alias internals retain separate costs.

type httpWriterEffects struct {
	writers   *ssaflow.CallGraphMemo[*ssa.Parameter, bool]
	overrides *ssaflow.FunctionSummaries[bool]
}

func newHTTPWriterEffects() *httpWriterEffects {
	effects := &httpWriterEffects{writers: ssaflow.NewCallGraphMemo[*ssa.Parameter, bool]()}
	effects.overrides = ssaflow.NewFunctionSummaries(effects.visibleOverrides, func(ssaflow.SummaryUnavailable) bool { return true })
	return effects
}

func (effects *httpWriterEffects) headerOnly(writer *ssa.Parameter, budget *proofs.SearchBudget) bool {
	return effects.writers.Summarize(writer, writer.Parent(), budget, func() bool {
		for instruction := range ssaflow.InstructionsWithin(writer.Parent(), budget) {
			for _, operand := range instruction.Operands(nil) {
				if !budget.Spend() {
					return false
				}
				if operand != nil && budget.Spend() && heapmodel.MayAlias(*operand, writer) && !effects.writerUse(instruction, writer, budget) {
					return false
				}
			}
		}
		return !resourceFlowExhausted(budget)
	}, func(ssaflow.SummaryUnavailable, bool) bool { return false })
}

func (effects *httpWriterEffects) writerUse(instruction ssa.Instruction, writer ssa.Value, budget *proofs.SearchBudget) bool {
	if _, ok := instruction.(*ssa.ChangeInterface); ok {
		return true
	}
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	common := call.Common()
	if ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("net/http", "SetCookie")) {
		return true
	}
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "net/http", Receiver: "ResponseWriter", Name: "Header",
	})) {
		return benignHTTPHeaders(call, budget)
	}
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "net/http", Receiver: "ResponseWriter", Name: "WriteHeader",
	})) {
		code, ok := common.Args[0].(*ssa.Const)
		if !ok || code.Value == nil || code.Value.Kind() != constant.Int {
			return false
		}
		n, exact := constant.Int64Val(code.Value)
		return exact && n >= 200 && n <= 599 && (n < 300 || n >= 400)
	}
	callee, closure := ssaflow.DirectCallee(common)
	// A visible helper must preserve the same restriction for every parameter
	// receiving the writer. Missing bodies or captured/dynamic dispatch are not
	// evidence of an empty effect set, so they cannot establish empty framing.
	found := false
	for _, binding := range ssaflow.CallBindings(common, callee, closure) {
		if !budget.Spend() {
			return false
		}
		if !budget.Spend() {
			return false
		}
		if !heapmodel.MayAlias(binding.Supplied, writer) {
			continue
		}
		parameter, ok := binding.Local.(*ssa.Parameter)
		if !ok || !effects.headerOnly(parameter, budget) {
			return false
		}
		found = true
	}
	return found
}

func benignHTTPHeaders(header *ssa.Call, budget *proofs.SearchBudget) bool {
	if header.Referrers() == nil {
		return false
	}
	for _, ref := range *header.Referrers() {
		if !budget.Spend() {
			return false
		}
		call, ok := ref.(*ssa.Call)
		if !ok || len(call.Common().Args) < 2 {
			return false
		}
		if !ssaflow.CallMatchesAnySymbol(call.Common(),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Set"}),
			syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "net/http", Receiver: "Header", Name: "Add"})) {
			return false
		}
		key := strings.ToLower(constantString(call.Common().Args[1]))
		switch key {
		case "", "content-length", "transfer-encoding", "trailer", "connection", "upgrade", "location":
			return false
		}
	}
	return true
}
