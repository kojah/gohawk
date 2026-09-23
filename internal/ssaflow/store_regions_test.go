package ssaflow

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"

	"golang.org/x/tools/go/ssa"
)

// The graph must answer every storage question the demand-driven model
// answers, with the same must-polarity, before any helper can delegate to
// it. This table is the existing storage table verbatim.
func TestRegionGraphStorageParity(t *testing.T) {
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
		{"retainingCall", `x:=box{value:a}; retain(&x); observe(x.value,a)`, true},
		{"asyncReadCall", `x:=box{value:a}; async(&x); observe(x.value,a)`, false},
		// Beyond the demand-driven model.
		{"loopInitializer", `var x box; x.value=a; for pick { observe(x.value,a) }`, true},
		{"fieldOfCopiedPointee", `y := *p; observe(y.value, p.value)`, true},
		{"fieldOfCopiedPointeeOther", `y := *p; observe(y.value, p.count)`, false},
		{"twoLoadsUntouched", `observe(p.value, p.value)`, true},
		// A call the graph cannot follow reaches only what it was handed:
		// a parameter never let out reads as the same object on both sides.
		{"twoLoadsAcrossCall", `u := p.value; opaque(nil); observe(u, p.value)`, true},
		{"twoLoadsAcrossCallHanded", `u := p.value; opaque(p); observe(u, p.value)`, false},
		{"twoLoadsAcrossCallEscaped", `retain(p); u := p.value; opaque(nil); observe(u, p.value)`, false},
		{"twoGlobalLoadsAcrossCall", `u := saved.value; opaque(nil); observe(u, saved.value)`, false},
		{"twoLoadsAcrossOtherField", `u := p.value; q.count = 1; observe(u, p.value)`, true},
		{"twoLoadsAcrossSameField", `u := p.value; q.value = b; observe(u, p.value)`, false},
		{"phiOfSame", `y := a; if pick { y = a }; observe(y, a)`, true},
		{"phiOfDifferent", `y := a; if pick { y = b }; observe(y, a)`, false},
		{"cellThroughPointer", `x := box{value:a}; ptr := &x; ptr.value = b; observe(x.value, b)`, true},
		// An unresolved call reaches only what it was handed, what escaped
		// before it, and globals; a parameter this function never let out
		// keeps what the function wrote into it.
		{"parameterWrittenAcrossCall", `p.value = a; opaque(nil); observe(p.value, a)`, true},
		{"parameterHandedToCall", `p.value = a; opaque(p); observe(p.value, a)`, false},
		{"parameterEscapedBeforeCall", `p.value = a; retain(p); opaque(nil); observe(p.value, a)`, false},
		{"parameterContentHandedToCall", `p.value = a; opaque(p.value); observe(p.value, a)`, true},
		{"globalAcrossCall", `saved.value = a; opaque(nil); observe(saved.value, a)`, false},
		{"parameterAcrossOtherParameterStore", `p.value = a; w.value = b; observe(p.value, a)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "regionprobe", `package regionprobe
type box struct { value *int; count int }
func observe(a,b any) {}
func opaque(any)
var saved *box
func read(p *box) int { return p.count }
func forward(p *box) int { return read(p) }
func mutate(p *box,b *int) { p.value=b }
func retain(p *box) { saved=p }
func async(p *box) { go read(p) }
func probe(a,b *int, p, q *box, pick bool, idx int, w *box) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			left, right := unwrapInterface(args[0]), unwrapInterface(args[1])
			graph := regionsOfFunction(call.Parent())
			if !graph.available {
				t.Fatal("graph unavailable")
			}
			if got := graph.mustSame(left, right); got != test.want {
				t.Fatalf("mustSame = %t, want %t (left %v right %v)", got, test.want, graph.values[left], graph.values[right])
			}
			if !graph.aliasProof(left, right).Aliases && test.want {
				t.Fatal("must-same values must may-alias")
			}
		})
	}
}

// aliasReasons names the rule behind selected answers.
var aliasReasons = map[string]EvidenceReason{
	"fieldsOfOneObject":        EvidenceDisjointPaths,
	"elementsApart":            EvidenceDisjointPaths,
	"sameFieldTwoObjects":      EvidenceDisjointObjects,
	"callResults":              EvidenceDisjointObjects,
	"localAndParameter":        EvidenceUnescapedLocal,
	"escapedLocalAndParameter": EvidenceUnescapedLocal,
	"elementAndStar":           EvidenceSharedSlot,
	"loadsAcrossCall":          EvidenceSharedSlot,
}

func unwrapInterface(value ssa.Value) ssa.Value { //nolint:ireturn // Test helper over SSA forms.
	if made, ok := value.(*ssa.MakeInterface); ok {
		return made.X
	}
	return value
}

// May-alias keeps the structural contract: objects are the same only when
// the function's own flow connects them. Two parameters, two call results,
// and two fields of one object are distinct; a cell after an overwrite no
// longer holds what it held; a placeholder still matches what was ever
// stored into its slot, and a dynamic element matches every element.
func TestRegionGraphMayAlias(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"fieldsOfOneObject", `observe(&p.value, &p.other)`, false},
		{"contentsOfTwoFields", `observe(p.value, p.other)`, false},
		{"sameFieldTwoObjects", `observe(p.value, q.value)`, false},
		{"localAndParameter", `x := box{}; observe(&x, p)`, false},
		{"escapedLocalAndParameter", `x := box{}; retain(&x); observe(&x, p)`, false},
		{"localsApart", `x := box{}; y := box{}; observe(&x, &y)`, false},
		{"elementAndStar", `var x [2]*int; observe(&x[0], &x[idx])`, true},
		{"elementsApart", `var x [2]*int; observe(&x[0], &x[1])`, false},
		{"callResults", `observe(acquire(), acquire())`, false},
		{"nilAndPointer", `observe(nil, p)`, false},
		{"storedThenReadAfterCall", `p.value = a; saved = nil; observe(p.value, a)`, true},
		{"storedThenReadAfterCallOther", `p.value = a; saved = nil; observe(p.value, b)`, false},
		{"loadsAcrossCall", `u := p.value; saved = nil; observe(u, p.value)`, true},
		{"overwrittenCell", `x := a; x = b; observe(x, a)`, false},
		{"overwrittenCellCurrent", `x := a; x = b; observe(x, b)`, true},
		{"mergedCell", `x := a; if pick { x = b }; observe(x, a)`, true},
		{"clobberedCellHeld", `x := box{value: a}; escapeBox(&x); observe(x.value, a)`, true},
		{"clobberedCellOther", `x := box{value: a}; escapeBox(&x); observe(x.value, b)`, false},
		{"phiOfParameters", `y := a; if pick { y = b }; observe(y, b)`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "regionprobe", `package regionprobe
type box struct { value *int; other *int }
func observe(a,b any) {}
func acquire() *box
func escapeBox(*box)
var saved *box
func retain(p *box) { saved=p }
func probe(a, b *int, p, q *box, pick bool, idx int) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			left, right := unwrapInterface(args[0]), unwrapInterface(args[1])
			graph := regionsOfFunction(call.Parent())
			proof := graph.aliasProof(left, right)
			if proof.Aliases != test.want {
				t.Fatalf("mayAlias = %+v, want %t (left %v right %v)", proof, test.want, graph.values[left], graph.values[right])
			}
			if want, ok := aliasReasons[test.name]; ok && proof.Reason != want {
				t.Fatalf("reason = %s, want %s", proof.Reason, want)
			}
			if !proof.Aliases && len(graph.disjoint) == 0 {
				t.Fatal("a disjointness answer must be recorded for the dump")
			}
		})
	}
}

// The rendering names each value's pointees by region kind and origin, so
// a reader can check an alias answer against the graph that gave it.
func TestRenderRegions(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "regionprobe", `package regionprobe
type box struct { value *int }
func probe(a *int) *int { x := box{value: a}; return x.value }
`)
	rendered := RenderRegions(pkg.Func("probe"))
	for _, want := range []string{"// regions:", "a -> param:a", "-> local:t0 field:0", "-> param:a"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendering lacks %q:\n%s", want, rendered)
		}
	}
}
