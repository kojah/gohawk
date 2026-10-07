package lifecycle

import (
	"bytes"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

type heapSmokeCase struct {
	name      string
	body      string
	baseline  bool
	prototype bool
}

func heapSmokeCases() []heapSmokeCase {
	return []heapSmokeCase{
		{"direct", `observe(a, a)`, true, true},
		{"cell", `x := a; p := &x; observe(*p, a)`, true, true},
		{"replacement", `x := a; p := &x; *p = b; observe(*p, b)`, true, true},
		{"stale", `x := a; p := &x; *p = b; observe(*p, a)`, false, false},
		{"field", `var x box; x.value = a; observe(x.value, a)`, true, true},
		{"fieldAlias", `var x box; p := &x; x.value = a; p.value = b; observe(x.value, b)`, true, true},
		{"staleField", `var x box; p := &x; x.value = a; p.value = b; observe(x.value, a)`, false, false},
		{"nested", `var x outer; x.inner.value = a; observe(x.inner.value, a)`, true, true},
		{"array", `var x [2]*int; x[0] = a; x[1] = b; observe(x[0], a)`, true, true},
		{"otherIndex", `var x [2]*int; x[0] = a; x[1] = b; observe(x[1], a)`, false, false},
		{"pointerCell", `var x box; p := &x; pp := &p; (*pp).value = a; observe(x.value, a)`, true, true},
		{"snapshot", `var x box; x.value = a; old := x.value; x.value = b; observe(old, a)`, true, true},
		// The graph sees that opaque's empty body keeps nothing.
		{"escape", `var x box; x.value = a; opaque(&x); observe(x.value, a)`, true, false},
		{"dynamicIndex", `var x [2]*int; x[0] = a; x[1] = b; observe(x[idx], a)`, false, false},
		{"branch", `var x box; if pick { x.value = a } else { x.value = a }; observe(x.value, a)`, true, false},
		{"loop", `var x box; for pick { x.value = a }; observe(x.value, a)`, false, false},
		{"aggregateOverwrite", `var x box; x.value = a; x = box{}; observe(x.value, a)`, false, false},
		{"capture", `x := a; keep := func() { _ = x }; _ = keep; observe(x, a)`, true, false},
		{"slice", `x := []*int{a, b}; observe(x[0], a)`, true, false},
	}
}

func buildHeapSmoke(tb testing.TB) *ssa.Package {
	tb.Helper()
	var source strings.Builder
	source.WriteString(`package heapsmoke
type box struct { value *int }
type outer struct { inner box }
func observe(a, b *int) {}
func opaque(any) {}
`)
	for _, test := range heapSmokeCases() {
		source.WriteString("func " + test.name + "(a, b *int, pick bool, idx int) { " + test.body + " }\n")
	}
	return ssaflowtest.BuildPackage(tb, "example.com/heapsmoke", source.String())
}

func heapObservation(tb testing.TB, function *ssa.Function) *ssa.Call {
	tb.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if ok && ssaflow.CallName(call.Common()) == "observe" {
				return call
			}
		}
	}
	tb.Fatal("no observation")
	return nil
}

func TestHeapSmokeComparison(t *testing.T) {
	pkg := buildHeapSmoke(t)
	for _, test := range heapSmokeCases() {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var dump bytes.Buffer
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			call := heapObservation(t, fn)
			args := call.Common().Args
			baseline := heapmodel.NewStorage(proofs.NewSearchBudget(1000)).Same(args[0], args[1]).Proven()
			prototype, reason := smokeHeapIdentity(call, 256)
			t.Logf("baseline=%t prototype=%t reason=%s", baseline, prototype, reason)
			if baseline != test.baseline || prototype != test.prototype {
				t.Errorf("got baseline=%t prototype=%t; want baseline=%t prototype=%t", baseline, prototype, test.baseline, test.prototype)
			}
			if proven, _ := smokeHeapIdentity(call, 0); proven {
				t.Fatal("exhausted budget must not prove identity")
			}
		})
	}
}

func BenchmarkHeapSmoke(b *testing.B) {
	pkg := buildHeapSmoke(b)
	call := heapObservation(b, pkg.Func("fieldAlias"))
	args := call.Common().Args
	b.Run("existing", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			heapmodel.NewStorage(proofs.NewSearchBudget(1000)).Same(args[0], args[1])
		}
	})
	b.Run("prototype", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			smokeHeapIdentity(call, 256)
		}
	})
}

// Scalar replacement now goes through the same storage query as fields and
// arrays, including its address escape and observation-time checks.
func TestHeapSmokeExistingStoreOverlap(t *testing.T) {
	pkg := buildHeapSmoke(t)
	call := heapObservation(t, pkg.Func("replacement"))
	load, ok := call.Common().Args[0].(*ssa.UnOp)
	if !ok {
		t.Fatal("replacement did not produce a load")
	}
	stored := heapmodel.NewStorage(proofs.NewSearchBudget(1000)).Content(load.X, call)
	if !stored.Proven() || !heapmodel.DefinitelySameValue(stored.Value, call.Common().Args[1]) {
		t.Fatal("existing latest-store helper failed to resolve the replacement")
	}
}

func TestHeapSmokeCleanupShadow(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func initial(a, b *resource, choose bool) {
	r := a
	defer func() { r.Close() }()
}
func latest(a, b *resource, choose bool) {
	r := a
	r = b
	defer func() { r.Close() }()
}
func replacedLater(a, b *resource, choose bool) {
	r := a
	defer func() { r.Close() }()
	r = b
}
func branch(a, b *resource, choose bool) {
	r := a
	if choose { r = b }
	defer func() { r.Close() }()
}
func clearedAfterClose(a, b *resource, choose bool) {
	r := a
	defer func() { if r != nil { r.Close() } }()
	if choose { r.Close(); r = nil }
}
`)
	for _, test := range []struct {
		name     string
		target   int
		complete bool
	}{
		{"initial", 0, true},
		{"latest", 1, true},
		{"latest", 0, false},
		{"replacedLater", 0, false},
		{"branch", 0, false},
		// The defer alone is conditional; whole-function coverage also needs
		// the explicit normal-path Close. A heap snapshot cannot replace that.
		{"clearedAfterClose", 0, false},
	} {
		t.Run(test.name+strconv.Itoa(test.target), func(t *testing.T) {
			fn := pkg.Func(test.name)
			instruction := findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
				_, ok := i.(*ssa.Defer)
				return ok
			})
			target := fn.Params[test.target]
			proof := ProveCompletion(CompletionRequest{
				Instruction: instruction, Target: target, Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1000),
			})
			if proof.Proven() != test.complete {
				t.Fatalf("existing completion = %#v, want proven %t", proof, test.complete)
			}
			// Even the easiest possible query at this seam (target == target)
			// cannot get through the prototype's control-flow/capture boundaries.
			// This measures feasibility, not an alternative cleanup proof.
			proven, reason := smokeHeapMatch(instruction, target, target, 256)
			if proven {
				t.Fatal("prototype unexpectedly entered an unsupported cleanup context")
			}
			t.Logf("existing completion=%t; prototype unavailable: %s", proof.Proven(), reason)
		})
	}
}

func TestHeapSmokeProjectionShadow(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type resource struct{}
type box struct { body *resource }
func acquire() *box
func cleanup(*resource)
func stable() { value := acquire(); cleanup(value.body) }
func replaced() { value := acquire(); value.body = new(resource); cleanup(value.body) }
`)
	for _, test := range []struct {
		name   string
		stable bool
	}{
		{"stable", true}, {"replaced", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			root := findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
				return ssaflow.CallName(ssaflow.InstructionCall(i)) == "acquire"
			}).(*ssa.Call)
			call := findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
				return ssaflow.CallName(ssaflow.InstructionCall(i)) == "cleanup"
			}).(*ssa.Call)
			value := call.Common().Args[0]
			if got := heapmodel.NewStorage(proofs.NewSearchBudget(1000)).Projection(value, root, call).Proven(); got != test.stable {
				t.Fatalf("existing projection=%t, want %t", got, test.stable)
			}
			if available, reason := smokeHeapMatch(call, value, value, 256); available || reason != "unsupported-effect" {
				t.Fatalf("prototype availability=%t, reason=%s", available, reason)
			}
		})
	}
}

// Test-only feasibility spike, deliberately not an analyzer dependency. Model
// straight-line local storage with exact addresses and strong updates. Anything
// that could mutate unmodeled storage stops the query, rather than guessing.
type smokeLocation struct {
	root *ssa.Alloc
	path string
}

type smokeValue struct {
	leaf     ssa.Value
	location smokeLocation
}

type smokeHeap struct {
	storage map[smokeLocation]smokeValue
	values  map[ssa.Value]smokeValue
}

func smokeHeapIdentity(observation *ssa.Call, budget int) (bool, string) {
	args := observation.Common().Args
	return smokeHeapMatch(observation, args[0], args[1], budget)
}

func smokeHeapMatch(observation ssa.Instruction, leftValue, rightValue ssa.Value, budget int) (bool, string) {
	fn := observation.Parent()
	if len(fn.Blocks) != 1 {
		return false, "control-flow"
	}
	heap := smokeHeap{storage: make(map[smokeLocation]smokeValue), values: make(map[ssa.Value]smokeValue)}
	for _, parameter := range fn.Params {
		heap.values[parameter] = smokeValue{leaf: parameter}
	}
	for _, instruction := range fn.Blocks[0].Instrs {
		budget--
		if budget < 0 {
			return false, "budget"
		}
		if instruction == observation {
			left, leftOK := heap.values[leftValue]
			right, rightOK := heap.values[rightValue]
			return leftOK && rightOK && left == right, "observed"
		}
		if !heap.step(instruction) {
			return false, "unsupported-effect"
		}
	}
	return false, "no-observation"
}

func (heap *smokeHeap) step(instruction ssa.Instruction) bool {
	switch instruction := instruction.(type) {
	case *ssa.Alloc:
		heap.values[instruction] = smokeValue{location: smokeLocation{root: instruction}}
	case *ssa.FieldAddr:
		return heap.project(instruction, instruction.X, ".f"+strconv.Itoa(instruction.Field))
	case *ssa.IndexAddr:
		index, ok := ssaflow.ConstantIndex(instruction.Index)
		return ok && heap.project(instruction, instruction.X, ".i"+index)
	case *ssa.Store:
		switch instruction.Val.Type().Underlying().(type) {
		case *types.Struct, *types.Array:
			return false
		}
		address, ok := heap.values[instruction.Addr]
		value, known := heap.values[instruction.Val]
		if !ok || address.location.root == nil || !known {
			return false
		}
		// Only tracked scalar values and addresses enter storage. Aggregate
		// replacement is unsupported, so overlapping parent/child writes cannot
		// leave stale field entries in this deliberately small model.
		heap.storage[address.location] = value
	case *ssa.UnOp:
		address, ok := heap.values[instruction.X]
		if instruction.Op != token.MUL || !ok || address.location.root == nil {
			return false
		}
		value, known := heap.storage[address.location]
		if !known {
			return false
		}
		heap.values[instruction] = value
	default:
		// Calls, goroutines, closure capture, slices, maps, and aggregate
		// copying require effects this smoke test does not model.
		return false
	}
	return true
}

func (heap *smokeHeap) project(value, base ssa.Value, component string) bool {
	address, ok := heap.values[base]
	if !ok || address.location.root == nil {
		return false
	}
	address.location.path += component
	heap.values[value] = address
	return true
}
