package lockorder

import (
	"strconv"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestOrderCycleEvidence(t *testing.T) {
	analyzer := Analyzer()
	run := analyzer.Run
	var diagnostics []analysis.Diagnostic
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			diagnostics = append(diagnostics, diagnostic)
			report(diagnostic)
		}
		defer func() { pass.Report = report }()
		return run(pass)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "ordercycles")
	var cycles, helpers, readers int
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, "c -> a -> b -> c") {
			cycles++
			if len(diagnostic.Related) != 6 {
				t.Errorf("cycle has %d locations, want 6", len(diagnostic.Related))
			}
		}
		for _, related := range diagnostic.Related {
			if !related.Pos.IsValid() {
				t.Error("invalid evidence position")
			}
			if strings.Contains(related.Message, "calls ordercycles.") {
				helpers++
			}
			if strings.Contains(related.Message, "read-locked") {
				readers++
			}
		}
	}
	if cycles != 1 || helpers != 2 || readers < 4 {
		t.Fatalf("cycles=%d helper steps=%d read-mode evidence=%d", cycles, helpers, readers)
	}
}

func TestOrderSearchBounds(t *testing.T) {
	orders := newLockOrders()
	for i := range maxOrderDepth + 2 {
		from, to := strconv.Itoa(i), strconv.Itoa(i+1)
		edge := orderEdge{held: lockAcquisition{class: from}, acquired: lockAcquisition{class: to}}
		orders.edges[lockRelation{from: from, to: to}] = edge
		orders.out[from] = append(orders.out[from], edge)
	}
	if path := orders.path("0", "3"); len(path) != 3 {
		t.Fatalf("short path length=%d", len(path))
	}
	if path := orders.path("0", strconv.Itoa(maxOrderDepth)); len(path) != 0 {
		t.Fatal("depth limit exceeded")
	}
	if path := orders.path("3", "0"); len(path) != 0 {
		t.Fatal("invented reverse path")
	}
}

func TestOrderSearchFanout(t *testing.T) {
	orders := newLockOrders()
	for i := range maxOrderSearch + 1 {
		middle := strconv.Itoa(i)
		orders.out["start"] = append(orders.out["start"], orderEdge{acquired: lockAcquisition{class: middle}})
		orders.out[middle] = []orderEdge{{acquired: lockAcquisition{class: "end"}}}
	}
	if len(orders.path("start", "end")) != 0 {
		t.Fatal("search should decline on edge budget exhaustion")
	}
	// Direct opposite orders remain constant-time even in a busy graph.
	orders.edges[lockRelation{from: "start", to: "end"}] = orderEdge{acquired: lockAcquisition{class: "end"}}
	if len(orders.path("start", "end")) != 1 {
		t.Fatal("direct edge lost behind search budget")
	}
}

func TestOrderIdentityAndDedup(t *testing.T) {
	orders := newLockOrders()
	a, b := lockAcquisition{class: "a"}, lockAcquisition{class: "b"}
	orders.record(nil, a, a)
	orders.record(nil, a, lockAcquisition{})
	if len(orders.edges) != 0 {
		t.Fatal("unknown or same-class edge retained")
	}
	orders.record(nil, a, b)
	orders.record(nil, a, b)
	if len(orders.edges) != 1 || len(orders.out["a"]) != 1 {
		t.Fatal("duplicate edge retained")
	}
	for i := range maxOrderEdges {
		orders.edges[lockRelation{from: strconv.Itoa(i), to: "target"}] = orderEdge{}
	}
	orders.record(nil, b, a)
	if len(orders.edges) != maxOrderEdges+1 {
		t.Fatal("edge cap exceeded")
	}
}
