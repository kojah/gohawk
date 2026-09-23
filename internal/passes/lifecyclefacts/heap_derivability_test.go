package lifecyclefacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
)

// Each mask claim should be a query over the heap projection. This table
// records, function by function, which claims the projection expresses
// today and which it does not yet, so the switch from separate summarizers
// to projection queries can proceed claim by claim with the gaps visible.
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
		"Wrap": {{"returned-owner P0", true}},
		// The mask accepts an alternative return of nil results; the
		// projection has the same facts as may edges, and the query for
		// "owner or nil on every return" is the next claim to express.
		"WrapMaybe":   {{"returned-owner P0", false}},
		"Adopt":       {{"receiver-store P1", true}},
		"Keep":        {{"retained P0", true}, {"stored P0", true}},
		"Publish":     {{"retained P0", true}},
		"KeepFirst":   {{"kept P0/field:0", true}},
		"CloseIt":     {{"released P0 Close", true}},
		"CloseSecond": {{"released P0/field:1 Close", true}},
		"Inspect":     {{"nothing", true}},
	} {
		fact := summarize(pass, newRetentionCache(), pkg.Func(name))
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

func parameterSlot(index int, path string) ssaflow.HeapSlot {
	return ssaflow.HeapSlot{Root: ssaflow.HeapRoot{Kind: ssaflow.HeapParameter, Index: index}, Path: path}
}

func edgeTo(heap *ssaflow.HeapSummary, slot ssaflow.HeapSlot, fromKind ssaflow.HeapRootKind, must bool) bool {
	for _, edge := range heap.Edges {
		if edge.From.Root.Kind == fromKind && edge.To.Kind == ssaflow.HeapTargetSlot && edge.To.Slot == slot && (edge.Must || !must) {
			return true
		}
	}
	return false
}

func escaped(heap *ssaflow.HeapSummary, slot ssaflow.HeapSlot, kinds ssaflow.HeapEscape) bool {
	for _, effect := range heap.Effects {
		if effect.Slot == slot && effect.Escape&kinds != 0 {
			return true
		}
	}
	return false
}

func released(heap *ssaflow.HeapSummary, slot ssaflow.HeapSlot, method string) bool {
	for _, effect := range heap.Effects {
		if effect.Slot == slot && effect.Release == method && effect.Every {
			return true
		}
	}
	return false
}

// heapQueries expresses each mask claim over the projection.
var heapQueries = map[string]func(*ssaflow.HeapSummary) bool{
	"returned-owner P0": func(heap *ssaflow.HeapSummary) bool {
		return edgeTo(heap, parameterSlot(0, ""), ssaflow.HeapResult, true)
	},
	"receiver-store P1": func(heap *ssaflow.HeapSummary) bool {
		return edgeTo(heap, parameterSlot(1, ""), ssaflow.HeapParameter, true)
	},
	"retained P0": func(heap *ssaflow.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, ""), ^ssaflow.HeapEscape(0)) ||
			edgeTo(heap, parameterSlot(0, ""), ssaflow.HeapGlobal, false) ||
			edgeTo(heap, parameterSlot(0, ""), ssaflow.HeapResult, false)
	},
	"stored P0": func(heap *ssaflow.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, ""), ssaflow.HeapEscapedGlobal|ssaflow.HeapEscapedField)
	},
	"kept P0/field:0": func(heap *ssaflow.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, "field:0"), ^ssaflow.HeapEscape(0)) ||
			edgeTo(heap, parameterSlot(0, "field:0"), ssaflow.HeapGlobal, false)
	},
	"released P0 Close":         func(heap *ssaflow.HeapSummary) bool { return released(heap, parameterSlot(0, ""), "Close") },
	"released P0/field:1 Close": func(heap *ssaflow.HeapSummary) bool { return released(heap, parameterSlot(0, "field:1"), "Close") },
	"nothing": func(heap *ssaflow.HeapSummary) bool {
		return len(heap.Edges) == 0 && len(heap.Effects) == 0 && len(heap.Truncated) == 0
	},
}

// factClaims reports whether the existing masks make the claim.
func factClaims(fact Fact, name string) bool {
	switch name {
	case "returned-owner P0":
		return fact.ReturnedOwner.contains(0)
	case "receiver-store P1":
		return fact.ReceiverStore.contains(1)
	case "retained P0":
		return fact.Retained.contains(0)
	case "stored P0":
		return fact.Stored.contains(0)
	case "kept P0/field:0":
		for _, kept := range fact.Kept {
			if kept.Parameter == 0 && kept.Path == "field:0" {
				return true
			}
		}
		return false
	case "released P0 Close":
		return fact.Closed.contains(0)
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
