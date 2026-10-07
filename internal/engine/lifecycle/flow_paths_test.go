package lifecycle

import (
	"go/constant"
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func TestSourceSSAFunctionsRejectsUnexpectedPrerequisiteResult(t *testing.T) {
	pass := &analysis.Pass{
		ResultOf: map[*analysis.Analyzer]any{
			buildssa.Analyzer: struct{}{},
		},
	}

	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err == nil {
		t.Fatal("SourceSSAFunctions() error = nil, want unexpected buildssa result error")
	}
	if functions != nil {
		t.Fatalf("SourceSSAFunctions() functions = %v, want nil", functions)
	}
}

func TestSameValueAcrossChannelDirections(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

func receive(<-chan int) {}
func send(chan<- int) {}
func sendOther(chan<- int) {}

func compare(first, second chan int) {
	receive(first)
	send(first)
	sendOther(second)
}
`)
	function := pkg.Func("compare")
	callArgument := func(name string) ssa.Value {
		t.Helper()
		instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			return common != nil && common.StaticCallee() != nil && common.StaticCallee().Name() == name
		})
		return ssaflow.InstructionCall(instruction).Args[0]
	}

	receiveArgument := callArgument("receive")
	if sendArgument := callArgument("send"); !heapmodel.MayAlias(receiveArgument, sendArgument) {
		t.Error("MayAlias did not preserve identity across sibling channel direction conversions")
	}
	if otherArgument := callArgument("sendOther"); heapmodel.MayAlias(receiveArgument, otherArgument) {
		t.Error("MayAlias equated channel conversions with different sources")
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
	if !cfg.BlockReachable(entry, left) || !cfg.BlockReachable(entry, right) {
		t.Fatal("BlockReachable() did not find an entry successor")
	}
	if cfg.BlockReachable(left, right) {
		t.Fatal("BlockReachable() connected disjoint return branches")
	}
	if cfg.BlockReachable(nil, right) {
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
	if !ssaflow.ExternallyOwnedValue(function.Params[0]) {
		t.Error("parameter should be externally owned")
	}
	global, ok := pkg.Members["global"].(*ssa.Global)
	if !ok || !ssaflow.ExternallyOwnedValue(global) {
		t.Error("package global should be externally owned")
	}
	allocation := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
		_, ok := instruction.(*ssa.Alloc)
		return ok
	}).(*ssa.Alloc)
	if ssaflow.ExternallyOwnedValue(allocation) {
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
		if !ok || !ssaflow.ExternallyOwnedValue(value) {
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
			value, known := ssapath.BranchBoolWithin(ssa.NewConst(constant.MakeBool(test.value), types.Typ[types.Bool]), nil, nil, nil)
			if !known || value != test.value {
				t.Fatalf("BranchBool = (%t, %t), want (%t, true)", value, known, test.value)
			}
		})
	}
	if value, known := ssapath.BranchBoolWithin(nil, nil, nil, nil); known || value {
		t.Fatalf("BranchBoolWithin(nil) = (%t, %t), want (false, false)", value, known)
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
			if _, known := ssapath.BranchBoolWithin(branch.Cond, block, predecessor, nil); known {
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
	return ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", source)
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

func TestSelectedReceiveChannel(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
func mixed(receive <-chan int, send chan<- int) int {
	select {
	case <-receive: return 1
	case send <- 1: return 2
	default: return 3
	}
}
func ordinary(flag bool, channel <-chan int) int {
	if flag { return 1 }; return 2
}
func stringPair() (string, error) { return "", nil }
func stringComparison() int {
	value, _ := stringPair()
	if value == "x" { return 1 }; return 2
}
func boolPair() (bool, error) { return false, nil }
func boolComparison(flag bool) int {
	value, _ := boolPair()
	if value == flag { return 1 }; return 2
}
`)
	function := pkg.Func("mixed")
	var matched *ssa.BasicBlock
	for _, block := range function.Blocks {
		channel, ok := ssapath.SelectedReceiveChannel(block)
		if !ok {
			continue
		}
		if matched != nil || channel != function.Params[0] {
			t.Fatalf("unexpected selected receive in block %d: %v", block.Index, channel)
		}
		matched = block
	}
	if matched == nil {
		t.Fatal("missing receive case")
	}
	// A shared successor cannot borrow one predecessor's selected operation.
	matched.Preds = append(matched.Preds, function.Blocks[0])
	if _, ok := ssapath.SelectedReceiveChannel(matched); ok {
		t.Fatal("shared successor was treated as a selected receive")
	}
	if channel, ok := ssapath.SelectedReceiveOnEdge(matched.Preds[0], matched); !ok || channel != function.Params[0] {
		t.Fatal("exact selected edge lost its receive at a shared successor")
	}
	for _, name := range []string{"ordinary", "stringComparison", "boolComparison"} {
		for _, block := range pkg.Func(name).Blocks {
			if _, ok := ssapath.SelectedReceiveChannel(block); ok {
				t.Fatalf("%s branch was treated as a selected receive", name)
			}
		}
	}
	if _, ok := ssapath.SelectedReceiveChannel(nil); ok {
		t.Fatal("nil block was treated as a selected receive")
	}
}

func TestUnownedReturnWithSelectedEdges(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
func start() {}
func cleanup() {}
func opaque() {}
func shared(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: cleanup() }
}
func conditional(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: if flag { cleanup() } }
}
func unknown(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: opaque() }
}
func abandoned(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: }
}
func defaultArm(done, work <-chan int, flag bool) {
	start()
	select { case <-done: default: }
}
`)
	for _, test := range []struct {
		name                      string
		unowned, uncertainUnowned bool
	}{
		{"shared", false, false},
		{"conditional", true, true},
		{"unknown", true, false},
		{"abandoned", true, true},
		{"defaultArm", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			start := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				return ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "start"
			})
			exact := func(instruction ssa.Instruction) bool {
				return ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "cleanup"
			}
			uncertain := func(instruction ssa.Instruction) bool {
				return exact(instruction) || ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "opaque"
			}
			edge := func(from, to *ssa.BasicBlock) bool {
				channel, selected := ssapath.SelectedReceiveOnEdge(from, to)
				return selected && channel == function.Params[0]
			}
			if got := ssapath.UnownedReturn(ssapath.UnownedReturnQuery{After: start, Owns: exact, OwnsEdge: edge}) != nil; got != test.unowned {
				t.Errorf("after start = %v, want %v", got, test.unowned)
			}
			if got := ssapath.UnownedReturn(ssapath.UnownedReturnQuery{Entry: function, Owns: exact, OwnsEdge: edge}) != nil; got != test.unowned {
				t.Errorf("from entry = %v, want %v", got, test.unowned)
			}
			if got := ssapath.UnownedReturn(ssapath.UnownedReturnQuery{
				After:    start,
				Owns:     uncertain,
				OwnsEdge: edge,
				Assume:   ssapath.EntryAssumptions{NonNil: function.Params[0]},
			}) != nil; got != test.uncertainUnowned {
				t.Errorf("uncertain/non-nil = %v, want %v", got, test.uncertainUnowned)
			}
			if ssapath.UnownedReturn(ssapath.UnownedReturnQuery{After: start, Owns: uncertain}) == nil {
				t.Error("ordinary instruction-only query borrowed an edge action")
			}
		})
	}
}

// A stable guard contradicts on its other arm; a loaded guard's other arm is
// only uncertain; a store to the loaded cell forgets it; an unrelated branch
// is consistent.
func TestPathGuards(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type options struct{ enabled bool }

func stable(enabled bool, a *int) int {
	if enabled {
		if a != nil {
			return 1
		}
	}
	if enabled {
		return 2
	}
	return 3
}

func loaded(o *options) int {
	if o.enabled {
		o.enabled = false
	}
	if o.enabled {
		return 1
	}
	return 0
}
`)
	stableFn := pkg.Func("stable")
	branches := branchBlocks(stableFn)
	guards, contradiction := ssapath.PathGuards(nil).ExtendWithin(branches[0], branches[0].Succs[0], nil, nil)
	if contradiction != ssapath.GuardConsistent || len(guards) != 1 || !guards[0].Stable || !guards[0].Value {
		t.Fatalf("first branch: %+v %v", guards, contradiction)
	}
	last := branches[len(branches)-1]
	if _, contradiction := guards.ExtendWithin(last, last.Succs[1], nil, nil); contradiction != ssapath.GuardStableContradiction {
		t.Errorf("the other arm of a stable guard should contradict, got %v", contradiction)
	}
	if _, contradiction := guards.ExtendWithin(last, last.Succs[0], nil, nil); contradiction != ssapath.GuardConsistent {
		t.Errorf("the same arm of a stable guard is consistent, got %v", contradiction)
	}
	if _, contradiction := guards.ExtendWithin(branches[1], branches[1].Succs[1], nil, nil); contradiction != ssapath.GuardConsistent {
		t.Errorf("an unrelated branch is consistent, got %v", contradiction)
	}
	keepLoaded := func(guard ssapath.PathGuard) bool { return !guard.Stable }
	if kept, _ := ssapath.PathGuards(nil).ExtendWithin(branches[0], branches[0].Succs[0], keepLoaded, nil); len(kept) != 0 {
		t.Errorf("a filtered-out guard must not be remembered: %+v", kept)
	}

	loadedFn := pkg.Func("loaded")
	branches = branchBlocks(loadedFn)
	guards, _ = ssapath.PathGuards(nil).ExtendWithin(branches[0], branches[0].Succs[0], nil, nil)
	if len(guards) != 1 || guards[0].Stable {
		t.Fatalf("a field load is a loaded guard: %+v", guards)
	}
	if _, contradiction := guards.ExtendWithin(branches[1], branches[1].Succs[1], nil, nil); contradiction != ssapath.GuardLoadedContradiction {
		t.Errorf("the other arm of a loaded guard is uncertain, got %v", contradiction)
	}
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](loadedFn) {
		guards = guards.AfterWithin(store, nil)
	}
	if len(guards) != 0 {
		t.Errorf("a store to the guarded cell forgets the guard: %+v", guards)
	}
}

func branchBlocks(function *ssa.Function) []*ssa.BasicBlock {
	var blocks []*ssa.BasicBlock
	for _, block := range function.Blocks {
		if len(block.Instrs) > 0 {
			if _, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); ok {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks
}

func TestConstantHelperBranches(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
import "errors"
func nilError() error { return nil }
func tuple() (int, error) { return 42, nil }
func maybe(b bool) error { if b { return errors.New("failure") }; return nil }
func named() (err error) { defer func() { err = errors.New("failure") }(); return }
func yes() bool { return true }
func same(b bool) error { if b { println("one"); return nil }; println("two"); return nil }
func opaque() error
func recur() error { return recur() }
func direct() { if nilError() != nil { return }; println("reachable") }
func extracted() { _, err := tuple(); if err == nil { println("reachable") } }
func variable(b bool) { if maybe(b) == nil { println("unknown") } }
func deferred() { if named() == nil { println("unknown") } }
func indirect(fn func() error) { if fn() == nil { println("unknown") } }
func boolean() { if yes() { println("reachable") } }
func sameReturns(b bool) { if same(b) != nil { println("unreachable") } }
func unavailable() { if opaque() == nil { println("unknown") } }
func recursive() { if recur() == nil { println("unknown") } }
`)
	for _, test := range []struct {
		name string
		arm  int
	}{
		{"direct", 1},
		{"extracted", 0},
		{"variable", -1},
		{"deferred", -1},
		{"indirect", -1},
		{"boolean", 0},
		{"sameReturns", 1},
		{"unavailable", -1},
		{"recursive", -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, block := range pkg.Func(test.name).Blocks {
				if len(block.Instrs) == 0 {
					continue
				}
				if _, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); !ok {
					continue
				}
				got := ssapath.FeasibleSuccessorsWithin(block, nil, nil)
				if test.arm < 0 {
					if len(got) != 2 {
						t.Fatalf("opaque result pruned successors: %v", got)
					}
				} else if len(got) != 1 || got[0] != block.Succs[test.arm] {
					t.Fatalf("successors = %v, want arm %d", got, test.arm)
				}
				return
			}
			t.Fatal("missing branch")
		})
	}
}
