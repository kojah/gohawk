package ssaflow

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestDefiniteIdentityRejectsPossibleAliases(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest
func chosen(a, b *int, pick bool) *int {
	v := a
	if pick { v = b }
	return v
}
func loaded(a, b *int) *int {
	v := a
	p := &v
	*p = b
	return *p
}
func alias(a *int) *int { return a }
func wrapped(a chan int) chan<- int { return a }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"chosen", false}, {"loaded", false}, {"alias", true}, {"wrapped", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			ret := findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
				_, ok := i.(*ssa.Return)
				return ok
			}).(*ssa.Return)
			value, target := ret.Results[0], fn.Params[0]
			if !SameValue(value, target) {
				t.Fatal("fixture must exercise the possible-identity matcher")
			}
			for _, pair := range [][2]ssa.Value{{value, target}, {target, value}} {
				if got := DefinitelySameValue(pair[0], pair[1]); got != test.want {
					t.Errorf("DefinitelySameValue(%s, %s) = %t, want %t", pair[0], pair[1], got, test.want)
				}
				proof := ProveIdentity(AccessPath{Value: pair[0]}, AccessPath{Value: pair[1]})
				if proof.Proven() != test.want {
					t.Errorf("identity proof = %#v, want proven %t", proof, test.want)
				}
			}
		})
	}
}

func TestDefiniteIdentityPhiAgreement(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest; func input(a, b *int) {}`)
	a, b := pkg.Func("input").Params[0], pkg.Func("input").Params[1]
	for _, test := range []struct {
		name string
		phi  *ssa.Phi
		want bool
	}{
		{"all agree", &ssa.Phi{Edges: []ssa.Value{a, a}}, true},
		{"mixed", &ssa.Phi{Edges: []ssa.Value{a, b}}, false},
		{"empty", &ssa.Phi{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := DefinitelySameValue(test.phi, a); got != test.want {
				t.Errorf("got %t, want %t", got, test.want)
			}
		})
	}
	cycle := &ssa.Phi{}
	cycle.Edges = []ssa.Value{a, cycle}
	if DefinitelySameValue(cycle, a) {
		t.Fatal("a cyclic phi must not establish identity")
	}
}

func TestCompletionDoesNotPromotePossibleIdentity(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func helper(r *resource) { r.Close() }
func chosen(a, b *resource, pick bool) {
	v := a
	if pick { v = b }
	helper(v)
}
func invoke(fn func()) { fn() }
func callback(a, b func(), pick bool) {
	fn := a
	if pick { fn = b }
	invoke(fn)
}
`)
	fn := pkg.Func("chosen")
	call := findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
		return CallName(InstructionCall(i)) == "helper"
	})
	proof := ProveCompletion(CompletionRequest{
		Instruction: call, Target: fn.Params[0], Methods: []string{"Close"}, Budget: NewSearchBudget(1000),
	})
	if proof.State != EvidenceUnknown {
		t.Errorf("mixed receiver completion = %#v, want unknown", proof)
	}
	fn = pkg.Func("callback")
	call = findSSAInstruction(t, fn, func(i ssa.Instruction) bool {
		return CallName(InstructionCall(i)) == "invoke"
	})
	if CallInvokesArgumentOnEveryReturn(call, fn.Params[0]) {
		t.Fatal("invoking a selected callback does not guarantee invoking parameter 0")
	}
}
