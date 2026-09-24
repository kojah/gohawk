package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestGraphBuildReasonCodes(t *testing.T) {
	want := map[GraphBuildReason]string{
		GraphBuildUnknown: "graph-build-unknown", GraphBuildComplete: "graph-build-complete",
		GraphBuildNoBody: "graph-build-no-body", GraphBuildBudgetExhausted: "graph-build-budget-exhausted",
		GraphBuildFixpointLimit: "graph-build-fixpoint-limit",
	}
	if len(want) != int(graphBuildReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range graphBuildReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []GraphBuildReason{graphBuildReasonCount, 255} {
		if reason.String() != "invalid-graph-build-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

func TestGraphBuildFailureClassification(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "graphreasons", `package graphreasons
func body() *int { return new(int) }
func opaque()
`)
	graph := buildRegionGraph(pkg.Func("body"))
	if !graph.available || graph.buildReason != GraphBuildComplete {
		t.Fatal("complete graph was not classified")
	}
	graph.budget = ssaflow.NewSearchBudget(1)
	reason, detail := graph.fixpoint()
	if reason != GraphBuildBudgetExhausted || detail != "" {
		t.Fatalf("budget: reason=%s detail=%q", reason, detail)
	}
	graph.buildReason, graph.buildDetail = reason, detail
	if graph.buildFailureText() != "budget exhausted" {
		t.Fatal("budget dump spelling changed")
	}
	opaque := &regionGraph{function: pkg.Func("opaque")}
	if reason, _ := opaque.fixpoint(); reason != GraphBuildNoBody {
		t.Fatal("absent body classified as complete")
	}
	graph.buildReason, graph.buildDetail = GraphBuildFixpointLimit, "in 6 rounds; changes: example"
	if graph.buildFailureText() != "fixpoint did not settle in 6 rounds; changes: example" {
		t.Fatal("fixpoint dump spelling changed")
	}
}
