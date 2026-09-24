package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

// A summarized callee is applied by substitution: a local the summary does
// not mention keeps its content across the call, an edge from a parameter
// field to another parameter is seen in the caller, and a truncated
// parameter is forgotten beneath. An unsummarized callee still forgets
// everything foreign.
func TestApplyHeapSummary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "applyprobe", `package applyprobe
type box struct{ value *int; other *int }
func observe(a, b any) {}
func summarized(p *box, q *int)
func truncating(p *box)
func unknownCallee(p *box)
func keepsLocal(a *int, p *box) { x := box{value: a}; summarized(p, a); observe(x.value, a) }
func seesEdge(a *int, p *box) { summarized(p, a); observe(p.value, a) }
func seesEdgeOther(a, b *int, p *box) { summarized(p, a); observe(p.value, b) }
func forgetsTruncated(a *int, p *box) { p.value = a; truncating(p); observe(p.value, a) }
func keepsUntouched(a *int, p *box, q *box) { q.value = a; summarized(p, a); observe(q.value, a) }
func keepsUnhanded(a *int, p *box, q *box) { q.value = a; unknownCallee(p); observe(q.value, a) }
func forgetsHanded(a *int, p *box, q *box) { q.value = a; unknownCallee(q); observe(q.value, a) }
`)
	RegisterHeapSummary(pkg.Func("summarized"), HeapSummary{
		Edges: []HeapEdge{{
			From: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 0}, Path: "field:0"},
			To:   HeapTarget{Kind: HeapTargetSlot, Slot: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 1}}},
			Must: true,
		}},
	})
	RegisterHeapSummary(pkg.Func("truncating"), HeapSummary{
		Truncated: []HeapSlot{{Root: HeapRoot{Kind: HeapParameter, Index: 0}}},
	})
	for name, want := range map[string]bool{
		"keepsLocal": true, "seesEdge": true, "seesEdgeOther": false, "forgetsTruncated": false, "keepsUntouched": true,
		// An unresolved call reaches what it was handed and nothing this
		// function never let out: under the structural contract two
		// parameters are distinct objects.
		"keepsUnhanded": true, "forgetsHanded": false,
	} {
		t.Run(name, func(t *testing.T) {
			call := heapObservation(t, pkg.Func(name))
			left, right := unwrapInterface(call.Common().Args[0]), unwrapInterface(call.Common().Args[1])
			graph := regionsOfFunction(call.Parent())
			if got := graph.mustSame(left, right); got != want {
				t.Fatalf("mustSame = %t, want %t\n%s", got, want, RenderRegions(call.Parent()))
			}
			if name != "keepsUnhanded" && name != "forgetsHanded" && !strings.Contains(RenderRegions(call.Parent()), "//   applied ") {
				t.Fatalf("the dump must record the applied summary:\n%s", RenderRegions(call.Parent()))
			}
		})
	}
}

// A returned field address and a returned field value have opposite nilness
// when the field is unwritten. The summary must keep that distinction when
// it substitutes the receiver's field into the caller.
func TestReturnedFieldAddress(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "addressprobe", `package addressprobe
type box struct{ embedded int; pointer *int }
func address(b *box) *int { return &b.embedded }
func content(b *box) *int { return b.pointer }
func observe(a, b *int) {}
func caller() { var b box; observe(address(&b), content(&b)) }
`)
	address, ok := ProjectHeap(pkg.Func("address"))
	if !ok || !strings.Contains(address.String(), "edge R0 -> &P0/field:0 must") {
		t.Fatalf("field address summary = %s, want a must address edge", address.String())
	}
	call := heapObservation(t, pkg.Func("caller"))
	graph := regionsOfFunction(call.Parent())
	if graph.contentIsNil(call.Common().Args[0], nil, call) {
		t.Fatal("returned address was mistaken for nil field content")
	}
	if !graph.contentIsNil(call.Common().Args[1], nil, call) {
		t.Fatal("returned nil field content was not proven nil")
	}
}
