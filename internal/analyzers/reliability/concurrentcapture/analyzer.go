// Package concurrentcapture implements the concurrentcapture gohawk analyzer.
package concurrentcapture

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "concurrentcapture",
		Doc:      "checks locals mutated by goroutines launched repeatedly",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      runConcurrentCapture,
	}
}

func runConcurrentCapture(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.ForStmt)(nil), (*ast.RangeStmt)(nil)}, func(node ast.Node) {
		var body *ast.BlockStmt
		switch loop := node.(type) {
		case *ast.ForStmt:
			body = loop.Body
		case *ast.RangeStmt:
			body = loop.Body
		}
		if body == nil || loopJoinsEachIteration(body) {
			return
		}
		inspectRepeatedLaunches(pass, node, body)
	})
	return nil, nil
}

func inspectRepeatedLaunches(pass *analysis.Pass, loop ast.Node, body *ast.BlockStmt) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch candidate := node.(type) {
		case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
			return false
		case *ast.GoStmt:
			if closure := calledClosure(candidate.Call); closure != nil {
				reportCapturedMutations(pass, body, closure, varyingWorkerParameters(pass, loop, candidate.Call, closure))
			}
			return false
		case *ast.CallExpr:
			selector, ok := candidate.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Go" || len(candidate.Args) == 0 {
				return true
			}
			if closure, ok := candidate.Args[0].(*ast.FuncLit); ok {
				reportCapturedMutations(pass, body, closure, nil)
			}
			return false
		default:
			return true
		}
	})
}

func calledClosure(call *ast.CallExpr) *ast.FuncLit {
	if call == nil {
		return nil
	}
	closure, _ := call.Fun.(*ast.FuncLit)
	return closure
}

func reportCapturedMutations(pass *analysis.Pass, body *ast.BlockStmt, closure *ast.FuncLit, varying []types.Object) {
	// A lock anywhere in the launched closure is conservative synchronization
	// evidence. Without one, report only writes whose root is a local declared
	// before the loop body. A body-local variable belongs to one iteration;
	// capturing it does not prove sharing between workers. Competing accesses
	// within one iteration are outside this repeated-launch check's proof.
	// https://github.com/okteto/okteto/blob/ad42c0823762a2255d4b4ad2e53fb4ec190010e7/pkg/ssh/manager_test.go#L143-L215
	if closureUsesLock(closure) {
		return
	}
	reported := map[types.Object]bool{}
	ast.Inspect(closure.Body, func(node ast.Node) bool {
		if nested, ok := node.(*ast.FuncLit); ok && nested != closure {
			return false
		}
		var expressions []ast.Expr
		switch candidate := node.(type) {
		case *ast.AssignStmt:
			expressions = candidate.Lhs
		case *ast.IncDecStmt:
			expressions = []ast.Expr{candidate.X}
		default:
			return true
		}
		if mutationHasWorkerGuard(pass, closure, node, varying) || mutationHasChannelGuard(pass, closure, node) {
			return true
		}
		for _, expression := range expressions {
			identifier := mutatedRoot(pass, expression)
			if identifier == nil {
				continue
			}
			object, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
			if !ok || object.Parent() == pass.Pkg.Scope() || object.Pos() >= body.Pos() || reported[object] {
				continue
			}
			reported[object] = true
			check.Reportf(pass, check.ConcurrentCapture, identifier.Pos(), "captured local %s is mutated by goroutines launched repeatedly", identifier.Name)
		}
		return true
	})
}

// A guard using a worker's own argument can select distinct writers across
// iterations. Without evaluating those arguments, repeated launch is not proof
// of repeated writes to this local. This is unknown, not proof of disjointness.
// https://github.com/metatube-community/metatube-sdk-go/blob/19a92ad3263ab7b69636d26d7cc32f531c0a0804/provider/internal/imcmp/image.go#L39-L55
func mutationHasWorkerGuard(pass *analysis.Pass, closure *ast.FuncLit, mutation ast.Node, varying []types.Object) bool {
	guarded := false
	ast.Inspect(closure.Body, func(node ast.Node) bool {
		branch, ok := node.(*ast.IfStmt)
		if !ok || mutation.Pos() < branch.Body.Pos() || mutation.End() > branch.End() {
			return true
		}
		for _, parameter := range varying {
			guarded = guarded || syntax.ExpressionUsesObject(pass, branch.Cond, parameter)
		}
		return !guarded
	})
	return guarded
}

func varyingWorkerParameters(pass *analysis.Pass, loop ast.Node, call *ast.CallExpr, closure *ast.FuncLit) []types.Object {
	ranged, ok := loop.(*ast.RangeStmt)
	if !ok || ranged.Key == nil {
		return nil
	}
	key, ok := ranged.Key.(*ast.Ident)
	if !ok {
		return nil
	}
	var parameters []types.Object
	index := 0
	for _, field := range closure.Type.Params.List {
		for _, name := range field.Names {
			if index < len(call.Args) {
				argument, direct := syntax.Unparen(call.Args[index]).(*ast.Ident)
				if direct && pass.TypesInfo.ObjectOf(argument) == pass.TypesInfo.ObjectOf(key) {
					parameters = append(parameters, pass.TypesInfo.ObjectOf(name))
				}
			}
			index++
		}
	}
	return parameters
}

// A send before the write and receive after it, in the same containing block,
// can delimit a channel semaphore. Capacity and branch-dependent concurrency
// are outside this syntax check, so this is uncertainty rather than a claimed
// happens-before proof. Unrelated channels and already-ended regions do not count.
// https://github.com/alajmo/sake/blob/86986df901293db0f7d1e548ef34c849bb1f709d/core/run/table.go#L419-L439
func mutationHasChannelGuard(pass *analysis.Pass, closure *ast.FuncLit, mutation ast.Node) bool {
	guarded := false
	ast.Inspect(closure.Body, func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok || mutation.Pos() < block.Pos() || mutation.End() > block.End() {
			return true
		}
		pending := map[types.Object]bool{}
		for _, statement := range block.List {
			if send, ok := statement.(*ast.SendStmt); ok && send.End() < mutation.Pos() {
				if channel := channelObject(pass, send.Chan); channel != nil {
					pending[channel] = true
				}
			}
			expression, ok := statement.(*ast.ExprStmt)
			if !ok {
				continue
			}
			receive, ok := expression.X.(*ast.UnaryExpr)
			if !ok || receive.Op != token.ARROW {
				continue
			}
			channel := channelObject(pass, receive.X)
			if receive.Pos() > mutation.End() && pending[channel] {
				guarded = true
			}
			delete(pending, channel)
		}
		return !guarded
	})
	return guarded
}

func channelObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	identifier, ok := syntax.Unparen(expression).(*ast.Ident)
	if !ok {
		return nil
	}
	return pass.TypesInfo.ObjectOf(identifier)
}

func mutatedRoot(pass *analysis.Pass, expression ast.Expr) *ast.Ident {
	switch candidate := expression.(type) {
	case *ast.Ident:
		return candidate
	case *ast.IndexExpr:
		typeOf := pass.TypesInfo.TypeOf(candidate.X)
		if typeOf != nil {
			_, ok := typeOf.Underlying().(*types.Map)
			if !ok {
				return nil
			}
			return mutatedRoot(pass, candidate.X)
		}
		return nil
	case *ast.ParenExpr:
		return mutatedRoot(pass, candidate.X)
	default:
		return nil
	}
}

func closureUsesLock(closure *ast.FuncLit) bool {
	usesLock := false
	ast.Inspect(closure.Body, func(node ast.Node) bool {
		if nested, ok := node.(*ast.FuncLit); ok && nested != closure {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Lock" || selector.Sel.Name == "RLock") {
			usesLock = true
			return false
		}
		return true
	})
	return usesLock
}

func loopJoinsEachIteration(body *ast.BlockStmt) bool {
	joined := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
			return false
		}
		switch candidate := node.(type) {
		case *ast.UnaryExpr:
			joined = joined || candidate.Op == token.ARROW
		case *ast.CallExpr:
			selector, ok := candidate.Fun.(*ast.SelectorExpr)
			joined = joined || ok && (selector.Sel.Name == "Wait" || selector.Sel.Name == "Join")
		}
		return !joined
	})
	return joined
}
