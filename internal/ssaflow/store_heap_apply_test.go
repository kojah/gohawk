package ssaflow

import (
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
func forgetsUnknown(a *int, p *box, q *box) { q.value = a; unknownCallee(p); observe(q.value, a) }
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
		"keepsLocal": true, "seesEdge": true, "seesEdgeOther": false, "forgetsTruncated": false, "keepsUntouched": true, "forgetsUnknown": false,
	} {
		t.Run(name, func(t *testing.T) {
			call := heapObservation(t, pkg.Func(name))
			left, right := unwrapInterface(call.Common().Args[0]), unwrapInterface(call.Common().Args[1])
			graph := regionsOfFunction(call.Parent())
			if got := graph.mustSame(left, right); got != want {
				t.Fatalf("mustSame = %t, want %t\n%s", got, want, RenderRegions(call.Parent()))
			}
		})
	}
}
