// Package concurrentcapture implements the concurrentcapture gohawk analyzer.
package concurrentcapture

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"go/version"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Concurrency: true})

type captureEvidence struct {
	provider *summaries.Provider
	workers  map[*ast.FuncLit]*ssa.Function
}

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "concurrentcapture",
		Doc:      "checks locals mutated by goroutines launched repeatedly",
		Requires: summaryKnowledge.Requires(),
		Run:      runConcurrentCapture,
	}
}

func runConcurrentCapture(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	evidence := captureEvidence{provider: summaryKnowledge.Provider(pass), workers: make(map[*ast.FuncLit]*ssa.Function)}
	for _, function := range functions {
		if closure, ok := function.Syntax().(*ast.FuncLit); ok {
			evidence.workers[closure] = function
		}
	}
	for _, file := range pass.Files {
		if !syntax.AnalyzeFile(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			var body *ast.BlockStmt
			switch loop := node.(type) {
			case *ast.ForStmt:
				body = loop.Body
			case *ast.RangeStmt:
				body = loop.Body
			}
			if body != nil && !loopJoinsEachIteration(body) {
				inspectRepeatedLaunches(pass, evidence, node, body)
			}
			return true
		})
	}
	return nil, nil
}

func inspectRepeatedLaunches(pass *analysis.Pass, evidence captureEvidence, loop ast.Node, body *ast.BlockStmt) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch candidate := node.(type) {
		case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
			return false
		case *ast.GoStmt:
			if closure := calledClosure(candidate.Call); closure != nil {
				reportCapturedMutations(pass, evidence, loop, body, closure, varyingWorkerParameters(pass, loop, candidate.Call, closure))
			}
			return false
		case *ast.CallExpr:
			selector, ok := candidate.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Go" || len(candidate.Args) == 0 {
				return true
			}
			if closure, ok := candidate.Args[0].(*ast.FuncLit); ok {
				reportCapturedMutations(pass, evidence, loop, body, closure, nil)
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

func reportCapturedMutations(
	pass *analysis.Pass, evidence captureEvidence, loop ast.Node, body *ast.BlockStmt, closure *ast.FuncLit, varying []types.Object,
) {
	// A complete straight-line worker prefix asks the shared synchronization
	// region whether a lock is held at this mutation. Unsupported workers retain
	// the older conservative lock suppression. Report only writes whose root is a local declared
	// before the loop body. A body-local variable belongs to one iteration;
	// capturing it does not prove sharing between workers. Competing accesses
	// within one iteration are outside this repeated-launch check's proof.
	// https://github.com/okteto/okteto/blob/ad42c0823762a2255d4b4ad2e53fb4ec190010e7/pkg/ssh/manager_test.go#L143-L215
	fallbackLock := closureUsesLock(closure)
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
		for _, expression := range expressions {
			if rangeIterationLocal(pass, loop, expression) {
				continue
			}
			identifier := mutatedRoot(pass, expression)
			if identifier == nil {
				continue
			}
			object, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
			if !ok || object.Parent() == pass.Pkg.Scope() || object.Pos() >= body.Pos() || reported[object] {
				continue
			}
			probe := analysisTrace.For(pass, "concurrentcapture", string(check.ConcurrentCapture), identifier.Pos())
			probe.Candidate(analysisTrace.Step{Reason: reasonRepeatedWrite.String(), Outcome: analysisTrace.OutcomeObserved, Pos: identifier.Pos()})
			guard := evidence.lockGuard(closure, node)
			// A held lock is evidence of possible serialization, not proof that
			// every worker uses the same lock. Unknown effects retain the older
			// syntax fallback rather than claiming the write is unguarded.
			switch {
			case guard.known && guard.guarded:
				probe.Decision(analysisTrace.Step{Reason: guard.reason.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: identifier.Pos()})
				continue
			case !guard.known && fallbackLock:
				probe.Decision(analysisTrace.Step{Reason: reasonLockFallbackUnknown.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: identifier.Pos()})
				continue
			case mutationHasWorkerGuard(pass, closure, node, varying):
				probe.Decision(analysisTrace.Step{Reason: reasonWorkerGuardUnknown.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: identifier.Pos()})
				continue
			case mutationHasChannelGuard(pass, closure, node):
				probe.Decision(analysisTrace.Step{Reason: reasonChannelGuardUnknown.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: identifier.Pos()})
				continue
			}
			reported[object] = true
			probe.Decision(analysisTrace.Step{Reason: reasonUnguardedWrite.String(), Outcome: analysisTrace.OutcomeAccepted, Pos: identifier.Pos()})
			reportSharedCapture(pass, identifier, object, loop)
		}
		return true
	})
}

// Go 1.22 range declarations create a fresh variable each iteration. Direct
// writes to that variable do not establish shared storage between workers.
// Map-index writes still mutate the referenced map, and ordinary for headers
// can read the prior iteration's variable while a worker mutates it. Neither
// is covered here; nor is an outer range variable shared by an inner loop.
// https://github.com/tensorchord/envd/blob/c5e6fd54eb111453ced50fe6fac1c16a506d7d62/pkg/builder/build.go#L235-L270
func rangeIterationLocal(pass *analysis.Pass, loop ast.Node, expression ast.Expr) bool {
	ranged, ok := loop.(*ast.RangeStmt)
	if !ok || ranged.Tok != token.DEFINE {
		return false
	}
	identifier, direct := syntax.Unparen(expression).(*ast.Ident)
	if !direct {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	declared := false
	for _, variable := range []ast.Expr{ranged.Key, ranged.Value} {
		if name, ok := variable.(*ast.Ident); ok && object != nil && pass.TypesInfo.Defs[name] == object {
			declared = true
		}
	}
	if !declared {
		return false
	}
	for _, file := range pass.Files {
		if file.Pos() <= loop.Pos() && loop.End() <= file.End() {
			language := pass.TypesInfo.FileVersions[file]
			if language == "" {
				language = pass.Pkg.GoVersion()
			}
			return version.Compare(language, "go1.22") >= 0
		}
	}
	return false
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

// reportSharedCapture reports a write to a captured local, citing where the
// local is declared, once, outside the loop, and the loop that launches a
// goroutine writing it on every iteration.
func reportSharedCapture(pass *analysis.Pass, identifier *ast.Ident, object *types.Var, loop ast.Node) {
	source := syntax.SourceRange(pass, identifier.Pos())
	check.Report(pass, check.ConcurrentCapture, analysis.Diagnostic{
		Pos:     source.Pos(),
		End:     source.End(),
		Message: fmt.Sprintf("captured local %s is mutated by goroutines launched repeatedly", identifier.Name),
		Related: []analysis.RelatedInformation{
			{Pos: object.Pos(), End: object.Pos() + token.Pos(len(object.Name())), Message: "`" + object.Name() + "` is declared once, before the loop"},
			check.KeywordEvidence(loop.Pos(), "for", "each iteration starts another goroutine that writes it"),
		},
	})
}
