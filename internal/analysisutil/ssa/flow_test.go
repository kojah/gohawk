package ssautil

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func TestSourceSSAFunctionsRejectsUnexpectedPrerequisiteResult(t *testing.T) {
	pass := &analysis.Pass{
		ResultOf: map[*analysis.Analyzer]any{
			buildssa.Analyzer: struct{}{},
		},
	}

	functions, err := SourceSSAFunctions(pass)
	if err == nil {
		t.Fatal("SourceSSAFunctions() error = nil, want unexpected buildssa result error")
	}
	if functions != nil {
		t.Fatalf("SourceSSAFunctions() functions = %v, want nil", functions)
	}
}

func TestStaticCallsiteIndexes(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

func callee() {}
func caller() {
	callee()
	defer callee()
	go callee()
}
`)
	callee := pkg.Func("callee")
	functions := []*ssa.Function{pkg.Func("caller"), callee}
	if got := len(StaticCallsites(functions)[callee]); got != 3 {
		t.Fatalf("StaticCallsites() count = %d, want 3", got)
	}
	if got := len(StaticCalls(functions)[callee]); got != 1 {
		t.Fatalf("StaticCalls() count = %d, want 1", got)
	}
}

func TestBlockReachable(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

func branch(flag bool) int {
	if flag {
		return 1
	}
	return 2
}
`)
	function := pkg.Func("branch")
	entry := function.Blocks[0]
	left := entry.Succs[0]
	right := entry.Succs[1]
	if !BlockReachable(entry, left) || !BlockReachable(entry, right) {
		t.Fatal("BlockReachable() did not find an entry successor")
	}
	if BlockReachable(left, right) {
		t.Fatal("BlockReachable() connected disjoint return branches")
	}
	if BlockReachable(nil, right) {
		t.Fatal("BlockReachable() accepted a nil source")
	}
}

func TestClosureOwnershipAndTransfer(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type holder struct { callback func() }
type callback func()

func deferred(target func()) {
	defer func() { target() }()
}

func launched(value *int) {
	go func() { _ = *value }()
}

func returned(value *int) func() {
	return func() { _ = *value }
}

func stored(value *int, target *holder) {
	target.callback = func() { _ = *value }
}

func boxed(value *int) any {
	return func() { _ = *value }
}

func converted(value *int) callback {
	return callback(func() { _ = *value })
}
`)

	deferred := pkg.Func("deferred")
	deferInstruction := findSSAInstruction(t, deferred, func(instruction ssa.Instruction) bool {
		_, ok := instruction.(*ssa.Defer)
		return ok
	})
	if !DeferredClosureCallsValue(deferInstruction, deferred.Params[0]) {
		t.Error("DeferredClosureCallsValue did not recognize a captured callback")
	}
	if DeferredClosureCallsValue(findMakeClosure(t, deferred), deferred.Params[0]) {
		t.Error("DeferredClosureCallsValue accepted a closure creation that was not deferred")
	}

	launched := pkg.Func("launched")
	goInstruction := findSSAInstruction(t, launched, func(instruction ssa.Instruction) bool {
		_, ok := instruction.(*ssa.Go)
		return ok
	})
	if !ClosureOwnsValue(goInstruction, launched.Params[0]) {
		t.Error("ClosureOwnsValue did not recognize a goroutine's captured value")
	}
	if ClosureOwnsValue(findMakeClosure(t, launched), launched.Params[0]) {
		t.Error("ClosureOwnsValue accepted a closure that was not launched")
	}

	for _, name := range []string{"returned", "stored", "boxed", "converted"} {
		function := pkg.Func(name)
		if !ClosureCapturesValue(findMakeClosure(t, function), function.Params[0]) {
			t.Errorf("ClosureCapturesValue did not recognize the %s closure transfer", name)
		}
	}
}

func TestExternallyOwnedValue(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

var global *int

type box struct { value *int }
type pointer *int

func local(parameter *int) {
	value := new(int)
	_, _ = parameter, value
}

func derived(owner *box, values []*int, choose bool) *int {
	if choose {
		return owner.value
	}
	return values[0]
}

func converted(value *int) pointer { return pointer(value) }
func boxed(value *int) any { return value }
func copied(value *int) **int {
	result := new(*int)
	*result = value
	return result
}
`)
	function := pkg.Func("local")
	if !ExternallyOwnedValue(function.Params[0]) {
		t.Error("parameter should be externally owned")
	}
	global, ok := pkg.Members["global"].(*ssa.Global)
	if !ok || !ExternallyOwnedValue(global) {
		t.Error("package global should be externally owned")
	}
	allocation := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
		_, ok := instruction.(*ssa.Alloc)
		return ok
	}).(*ssa.Alloc)
	if ExternallyOwnedValue(allocation) {
		t.Error("unescaped local allocation should not be externally owned")
	}

	for _, test := range []struct {
		function string
		matches  func(ssa.Instruction) bool
	}{
		{function: "derived", matches: func(instruction ssa.Instruction) bool { _, ok := instruction.(*ssa.FieldAddr); return ok }},
		{function: "derived", matches: func(instruction ssa.Instruction) bool { _, ok := instruction.(*ssa.IndexAddr); return ok }},
		{function: "converted", matches: func(instruction ssa.Instruction) bool { _, ok := instruction.(*ssa.ChangeType); return ok }},
		{function: "boxed", matches: func(instruction ssa.Instruction) bool { _, ok := instruction.(*ssa.MakeInterface); return ok }},
		{function: "copied", matches: func(instruction ssa.Instruction) bool { _, ok := instruction.(*ssa.Alloc); return ok }},
	} {
		instruction := findSSAInstruction(t, pkg.Func(test.function), test.matches)
		value, ok := instruction.(ssa.Value)
		if !ok || !ExternallyOwnedValue(value) {
			t.Errorf("%s value %T should retain external ownership", test.function, instruction)
		}
	}
}

func TestBranchBool(t *testing.T) {
	for _, test := range []struct {
		name  string
		value bool
	}{
		{name: "true", value: true},
		{name: "false", value: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, known := branchBool(ssa.NewConst(constant.MakeBool(test.value), types.Typ[types.Bool]), nil, nil)
			if !known || value != test.value {
				t.Fatalf("branchBool = (%t, %t), want (%t, true)", value, known, test.value)
			}
		})
	}
	if value, known := branchBool(nil, nil, nil); known || value {
		t.Fatalf("branchBool(nil) = (%t, %t), want (false, false)", value, known)
	}

	pkg := buildTestSSA(t, `
package ssaflowtest

func firstIteration() {
	first := true
	for first {
		first = false
	}
}
`)
	function := pkg.Func("firstIteration")
	var recognized int
	for _, block := range function.Blocks {
		if len(block.Instrs) == 0 {
			continue
		}
		branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
		if !ok {
			continue
		}
		if _, ok := branch.Cond.(*ssa.Phi); !ok {
			continue
		}
		for _, predecessor := range block.Preds {
			if _, known := branchBool(branch.Cond, block, predecessor); known {
				recognized++
			}
		}
	}
	if recognized != 2 {
		t.Fatalf("recognized %d predecessor-selected branch values, want 2", recognized)
	}
}

func buildTestSSA(t *testing.T, source string) *ssa.Package {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "ssaflow.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssautil.BuildPackage(
		&types.Config{Importer: importer.Default()},
		fset,
		types.NewPackage("example.com/ssaflowtest", "ssaflowtest"),
		[]*ast.File{file},
		ssa.SanityCheckFunctions,
	)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func findMakeClosure(t *testing.T, function *ssa.Function) *ssa.MakeClosure {
	t.Helper()
	return findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
		_, ok := instruction.(*ssa.MakeClosure)
		return ok
	}).(*ssa.MakeClosure)
}

func findSSAInstruction(t *testing.T, function *ssa.Function, matches func(ssa.Instruction) bool) ssa.Instruction {
	t.Helper()
	if function == nil {
		t.Fatal("SSA function is nil")
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if matches(instruction) {
				return instruction
			}
		}
	}
	t.Fatalf("matching instruction not found in %s", function.Name())
	return nil
}
