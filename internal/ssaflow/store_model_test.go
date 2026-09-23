package ssaflow

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"

	"golang.org/x/tools/go/ssa"
)

func TestStorageSnapshots(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"literal", `x := box{value:a}; observe(x.value, a)`, true},
		{"copy", `x := box{value:a}; y := x; x.value = b; observe(y.value, a)`, true},
		{"copyNotCurrent", `x := box{value:a}; y := x; x.value = b; observe(y.value, b)`, false},
		{"overwrite", `x := box{value:a}; x = box{value:b}; observe(x.value, b)`, true},
		{"overwriteNotOld", `x := box{value:a}; x = box{value:b}; observe(x.value, a)`, false},
		{"arrayCopy", `x := [2]*int{a,b}; y := x; x[0] = b; observe(y[0], a)`, true},
		{"opaqueSlot", `x := box{value:a}; opaque(&x.value); observe(x.value, a)`, false},
		{"opaqueRoot", `x := box{value:a}; opaque(&x); observe(x.value, a)`, false},
		{"opaqueAfterSnapshot", `x := box{value:a}; old := x.value; opaque(&x); observe(old, a)`, true},
		{"ambiguousWrite", `x := box{value:a}; if pick { x.value = b }; observe(x.value, a)`, false},
		{"dynamicWrite", `x := [2]*int{a,b}; x[idx] = b; observe(x[0], a)`, false},
		{"capturedMutation", `x := box{value:a}; f := func() { x.value = b }; f(); observe(x.value, a)`, false},
		{"cyclicCell", `var x any; x = &x; opaque(x); observe(a,b)`, false},
		{"overwriteAfterBranches", `var x box; if pick { x.value=a } else { x.value=b }; x.value=a; observe(x.value,a)`, true},
		{"agreeingBranches", `var x box; if pick { x.value=a } else { x.value=a }; observe(x.value,a)`, true},
		{"conflictingBranches", `var x box; if pick { x.value=a } else { x.value=b }; observe(x.value,a)`, false},
		{"missingBranchWrite", `var x box; if pick { x.value=a }; observe(x.value,a)`, false},
		{"independentField", `x:=box{value:a}; x.count=1; observe(x.value,a)`, true},
		{"independentBranchField", `x:=box{value:a}; if pick { x.count=1 } else { x.count=2 }; observe(x.value,a)`, true},
		{"unchangedAcrossLoop", `x:=box{value:a}; for pick { opaque(b) }; observe(x.value,a)`, true},
		{"unchangedInsideLoop", `x:=box{value:a}; for pick { observe(x.value,a) }`, true},
		{"changedInsideLoop", `x:=box{value:a}; for pick { x.value=b }; observe(x.value,a)`, false},
		{"fixedOffset", `x:=[3]*int{b,a,b}; s:=x[1:]; observe(s[0],a)`, true},
		{"nestedOffset", `x:=[3]*int{b,b,a}; s:=x[1:][1:]; observe(s[0],a)`, true},
		{"offsetWrongSlot", `x:=[3]*int{a,b,b}; s:=x[1:]; observe(s[0],a)`, false},
		{"offsetWrite", `x:=[3]*int{a,a,a}; s:=x[1:]; s[0]=b; observe(x[1],b)`, true},
		{"offsetSibling", `x:=[3]*int{a,a,a}; s:=x[1:]; s[0]=b; observe(x[0],a)`, true},
		{"offsetEscape", `x:=[3]*int{a,a,a}; s:=x[1:]; opaque(s); observe(x[1],a)`, false},
		{"dynamicOffset", `x:=[3]*int{a,a,a}; s:=x[idx:]; observe(s[0],a)`, false},
		{"fullSliceBounds", `x:=[3]*int{b,a,b}; s:=x[1:2:3]; observe(s[0],a)`, true},
		{"pastSliceLength", `x:=[3]*int{b,a,a}; s:=x[1:2:3]; observe(s[1],a)`, false},
		{"readOnlyCall", `x:=box{value:a}; read(&x); observe(x.value,a)`, true},
		{"readOnlyForwarding", `x:=box{value:a}; forward(&x); observe(x.value,a)`, true},
		{"mutatingCall", `x:=box{value:a}; mutate(&x,b); observe(x.value,a)`, false},
		// The callee's summary says it stored the address in a global and
		// wrote nothing through it, so the field still holds a.
		{"retainingCall", `x:=box{value:a}; retain(&x); observe(x.value,a)`, true},
		{"asyncReadCall", `x:=box{value:a}; async(&x); observe(x.value,a)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "storageprobe", `package storageprobe
type box struct { value *int; count int }
func observe(a,b *int) {}
func opaque(any)
var saved *box
func read(p *box) int { return p.count }
func forward(p *box) int { return read(p) }
func mutate(p *box,b *int) { p.value=b }
func retain(p *box) { saved=p }
func async(p *box) { go read(p) }
func probe(a,b *int, pick bool, idx int) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			proof := NewStorage(NewSearchBudget(1000)).Same(args[0], args[1])
			if proof.Proven() != test.want {
				t.Fatalf("Same() = %+v, want proven %t", proof, test.want)
			}
			if proof := NewStorage(NewSearchBudget(0)).Resolve(args[0]); proof.Proven() || proof.Reason != EvidenceBudgetExhausted {
				t.Fatalf("exhaustion = %+v", proof)
			}
		})
	}
}

func TestStorageDeferredObservation(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"initial", `x:=a; inspect(&x)`, true},
		{"replacedBefore", `x:=a; x=b; inspect(&x)`, true},
		{"replacedAfter", `x:=a; inspect(&x); x=b`, false},
		{"opaqueAfter", `x:=a; inspect(&x); opaque(&x)`, false},
		{"mutableCapture", `x:=a; inspect(&x); f:=func(){ x=b }; f()`, false},
		{"reusedInLoop", `var x *int; for pick { x=a; inspect(&x) }`, false},
		{"stableInsideLoop", `x:=a; for pick { inspect(&x) }`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "storageprobe", `package storageprobe
func inspect(**int) {}
func opaque(any)
func probe(a,b *int, pick bool) { `+test.body+` }
`)
			for _, call := range InstructionsOf[*ssa.Call](pkg.Func("probe")) {
				if CallName(call.Common()) != "inspect" {
					continue
				}
				proof := NewStorage(NewSearchBudget(1000)).StableContent(call.Common().Args[0], call)
				if proof.Proven() != test.want {
					t.Fatalf("StableContent() = %+v, want proven %t", proof, test.want)
				}
				return
			}
			t.Fatal("missing observation")
		})
	}
}

func TestStorageAggregateSnapshots(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"unchanged", `x:=a; observe(x,a)`, true},
		{"fieldChanged", `x:=a; x.value=b; observe(x,a)`, false},
		{"snapshotBeforeChange", `x:=a; old:=x; x.value=b; observe(old,a)`, true},
		{"wholeOverwrite", `x:=a; x.value=b; x=a; observe(x,a)`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "storageprobe", `package storageprobe
type box struct { value *int }
func observe(a,b box) {}
func probe(a box,b *int) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			proof := NewStorage(NewSearchBudget(1000)).Same(args[0], args[1])
			if proof.Proven() != test.want {
				t.Fatalf("Same() = %+v, want proven %t", proof, test.want)
			}
		})
	}
}
