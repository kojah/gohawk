package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"golang.org/x/tools/go/analysis"
)

// The transfer and retention claims are read from the heap projection, and
// the discharges are mirrored into it as release effects. This table pins,
// function by function, that each claim the fact answers is also visible in
// the raw projection a caller's graph applies.
func TestLifecycleSummaryClaimsDerivableFromHeap(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{ id int }

func (c *closer) Close() error { return nil }

type owner struct{ body *closer }
type pair struct{ first, second *closer }
type sink interface{ Add(any) }

var saved *closer
var registry sink

func Wrap(c *closer) *owner                { return &owner{body: c} }
func WrapMaybe(c *closer, ok bool) *owner  { if ok { return &owner{body: c} }; return nil }
func Adopt(o *owner, c *closer)            { o.body = c }
func Keep(c *closer)                       { saved = c }
func Publish(c *closer)                    { registry.Add(c) }
func KeepFirst(p *pair)                    { saved = p.first }
func CloseIt(c *closer) error              { return c.Close() }
func CloseSecond(p *pair) error            { return p.second.Close() }
func Inspect(p *pair) bool                 { return p.first != nil }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	type claim struct {
		name    string
		derived bool
	}
	for name, want := range map[string][]claim{
		"Wrap":        {{"returned-owner P0", true}},
		"WrapMaybe":   {{"returned-owner P0", true}},
		"Adopt":       {{"receiver-store P1", true}},
		"Keep":        {{"retained P0", true}, {"stored P0", true}},
		"Publish":     {{"retained P0", true}},
		"KeepFirst":   {{"kept P0/field:0", true}},
		"CloseIt":     {{"released P0 Close", true}},
		"CloseSecond": {{"released P0/field:1 Close", true}},
		"Inspect":     {{"nothing", true}},
	} {
		fact := summarize(pass, pkg.Func(name))
		if fact.Heap == nil {
			t.Fatalf("%s: no projection", name)
		}
		for _, expected := range want {
			derived := deriveClaim(fact, expected.name)
			claimed := factClaims(fact, expected.name)
			if derived != expected.derived {
				t.Errorf("%s: claim %q derived from projection = %t, want %t\n%s", name, expected.name, derived, expected.derived, fact.Heap.String())
			}
			if expected.name != "nothing" && !claimed {
				t.Errorf("%s: the mask does not make claim %q", name, expected.name)
			}
		}
	}
}

// deriveClaim answers a mask claim as a query over the projection.
func deriveClaim(fact Fact, name string) bool {
	query, ok := heapQueries[name]
	return ok && query(fact.Heap)
}

func parameterSlot(index int, path string) heapmodel.HeapSlot {
	return heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index}, Path: path}
}

func edgeTo(heap *heapmodel.HeapSummary, slot heapmodel.HeapSlot, fromKind heapmodel.HeapRootKind, must bool) bool {
	for _, edge := range heap.Edges {
		if edge.From.Root.Kind == fromKind && edge.To.Kind == heapmodel.HeapTargetSlot && edge.To.Slot == slot && (edge.Must || !must) {
			return true
		}
	}
	return false
}

func escaped(heap *heapmodel.HeapSummary, slot heapmodel.HeapSlot, kinds heapmodel.HeapEscape) bool {
	for _, effect := range heap.Effects {
		if effect.Slot == slot && effect.Escape&kinds != 0 {
			return true
		}
	}
	return false
}

func released(heap *heapmodel.HeapSummary, slot heapmodel.HeapSlot, method string) bool {
	for _, effect := range heap.Effects {
		if effect.Slot == slot && effect.Release == method && effect.Every {
			return true
		}
	}
	return false
}

// heapQueries expresses each mask claim over the projection.
var heapQueries = map[string]func(*heapmodel.HeapSummary) bool{
	"returned-owner P0": func(heap *heapmodel.HeapSummary) bool {
		for _, hold := range heap.Holds {
			if hold.Parameter == 0 && hold.Must {
				return true
			}
		}
		return false
	},
	"receiver-store P1": func(heap *heapmodel.HeapSummary) bool {
		return edgeTo(heap, parameterSlot(1, ""), heapmodel.HeapParameter, true)
	},
	"retained P0": func(heap *heapmodel.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, ""), ^heapmodel.HeapEscape(0)) ||
			edgeTo(heap, parameterSlot(0, ""), heapmodel.HeapGlobal, false) ||
			edgeTo(heap, parameterSlot(0, ""), heapmodel.HeapResult, false)
	},
	"stored P0": func(heap *heapmodel.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, ""), heapmodel.HeapEscapedGlobal|heapmodel.HeapEscapedField)
	},
	"kept P0/field:0": func(heap *heapmodel.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, "field:0"), ^heapmodel.HeapEscape(0)) ||
			edgeTo(heap, parameterSlot(0, "field:0"), heapmodel.HeapGlobal, false)
	},
	"released P0 Close":         func(heap *heapmodel.HeapSummary) bool { return released(heap, parameterSlot(0, ""), "Close") },
	"released P0/field:1 Close": func(heap *heapmodel.HeapSummary) bool { return released(heap, parameterSlot(0, "field:1"), "Close") },
	"nothing": func(heap *heapmodel.HeapSummary) bool {
		return len(heap.Edges) == 0 && len(heap.Effects) == 0 && len(heap.Truncated) == 0
	},
}

// factClaims reports whether the existing masks make the claim.
func factClaims(fact Fact, name string) bool {
	switch name {
	case "returned-owner P0":
		return fact.ReturnedOwner().contains(0)
	case "receiver-store P1":
		return fact.ReceiverStore().contains(1)
	case "retained P0":
		return fact.Retained().contains(0)
	case "stored P0":
		return fact.Stored().contains(0)
	case "kept P0/field:0":
		for _, kept := range fact.Kept() {
			if kept.Parameter == 0 && kept.Path == "field:0" {
				return true
			}
		}
		return false
	case "released P0 Close":
		return fact.MethodMask("Close").contains(0)
	case "released P0/field:1 Close":
		for _, discharge := range fact.Discharges {
			if discharge.Parameter == 0 && discharge.Method == "Close" && discharge.Path == "field:1" {
				return true
			}
		}
		return false
	}
	return false
}
