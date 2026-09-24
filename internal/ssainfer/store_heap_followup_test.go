package ssainfer

import (
	"strconv"
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Scalar replacement now goes through the same storage query as fields and
// arrays, including its address escape and observation-time checks.
func TestHeapSmokeExistingStoreOverlap(t *testing.T) {
	pkg := buildHeapSmoke(t)
	call := heapObservation(t, pkg.Func("replacement"))
	load, ok := call.Common().Args[0].(*ssa.UnOp)
	if !ok {
		t.Fatal("replacement did not produce a load")
	}
	stored := heapmodel.NewStorage(ssaflow.NewSearchBudget(1000)).Content(load.X, call)
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
				Instruction: instruction, Target: target, Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(1000),
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
			if got := heapmodel.NewStorage(ssaflow.NewSearchBudget(1000)).Projection(value, root, call).Proven(); got != test.stable {
				t.Fatalf("existing projection=%t, want %t", got, test.stable)
			}
			if available, reason := smokeHeapMatch(call, value, value, 256); available || reason != "unsupported-effect" {
				t.Fatalf("prototype availability=%t, reason=%s", available, reason)
			}
		})
	}
}
