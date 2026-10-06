package heapmodel

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDeferredCellRelationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "deferredprobe", `package deferredprobe
type resource struct{}
func (*resource) Close() {}
func register(func()) {}
func exact(p, other *resource, pick bool) { held:=p; defer func(){held.Close()}() }
func cleared(p, other *resource, pick bool) { held:=p; defer func(){if held!=nil{held.Close()}}(); if pick{p.Close();held=nil} }
func replaced(p, other *resource, pick bool) { held:=p; defer func(){held.Close()}(); held=other }
func mixed(p, other *resource, pick bool) { held:=p; defer func(){held.Close()}(); if pick{held=other} }
func aggregate(p, other *resource, pick bool) { held:=[]*resource{p}; defer func(){for _,r:=range held{r.Close()}}(); held=append(held,p) }
func registered(p, other *resource, pick bool) { held:=p; register(func(){held.Close()}) }
`)
	for _, test := range []struct {
		name string
		want DeferredCellMatch
	}{
		{"exact", DeferredCellExact},
		{"cleared", DeferredCellExact},
		{"replaced", DeferredCellUnknown},
		{"mixed", DeferredCellUnknown},
		{"aggregate", DeferredCellContains},
		{"registered", DeferredCellExact},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			cell, invocation := deferredCellCase(t, function)
			full := proofs.NewSearchBudget(proofs.QueryBudget)
			got, known := DeferredCellRelationWithin(cell, function.Params[0], invocation, full)
			if !known || got != test.want || full.Exhausted() {
				t.Fatalf("full relation=%v, known=%v, want %v", got, known, test.want)
			}
			assertDeferredRelationCutoffs(t, cell, function.Params[0], invocation, test.want)
			pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
			cut := pool.Within(1)
			if got, known := DeferredCellRelationWithin(cell, function.Params[0], invocation, cut); known || got != DeferredCellUnknown {
				t.Fatalf("cut published relation=%v, known=%v", got, known)
			}
			if !cut.Exhausted() || pool.Exhausted() {
				t.Fatal("independent child cutoff was not preserved")
			}
			fresh := pool.Within(proofs.QueryBudget)
			if got, known := DeferredCellRelationWithin(cell, function.Params[0], invocation, fresh); !known || got != test.want {
				t.Fatalf("fresh relation=%v, known=%v, want %v", got, known, test.want)
			}
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
		})
	}
}

func assertDeferredRelationCutoffs(t *testing.T, cell *ssa.Alloc, target ssa.Value, invocation ssa.Instruction, want DeferredCellMatch) {
	t.Helper()
	for allowance := 2; allowance < proofs.QueryBudget; allowance++ {
		budget := proofs.NewSearchBudget(allowance)
		got, known := DeferredCellRelationWithin(cell, target, invocation, budget)
		if budget.Exhausted() {
			if known || got != DeferredCellUnknown {
				t.Fatalf("allowance %d published interrupted relation=%v, known=%v", allowance, got, known)
			}
			continue
		}
		if !known || got != want {
			t.Fatalf("completed allowance %d relation=%v, known=%v, want %v", allowance, got, known, want)
		}
		return
	}
	t.Fatal("relation did not complete within query allowance")
}

func deferredCellCase(t *testing.T, function *ssa.Function) (*ssa.Alloc, ssa.Instruction) {
	t.Helper()
	for instruction := range ssaflow.InstructionsWithin(function, nil) {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			continue
		}
		_, closure := ssacall.DirectCallee(common)
		if closure == nil && ssaflow.CallName(common) == "register" {
			closure, _ = common.Args[0].(*ssa.MakeClosure)
		}
		if closure != nil && len(closure.Bindings) == 1 {
			cell, ok := closure.Bindings[0].(*ssa.Alloc)
			if !ok {
				t.Fatalf("captured binding is %T, want actual cell", closure.Bindings[0])
			}
			return cell, instruction
		}
	}
	t.Fatal("missing deferred/callback cell")
	return nil, nil
}

func TestDeferredObservationCensusDiscardsPrefix(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observationprobe", `package observationprobe
func probe(pick bool) { defer func(){}(); if pick{return} }
`)
	function := pkg.Func("probe")
	firstRun, instructions := 0, 0
	for instruction := range ssaflow.InstructionsWithin(function, nil) {
		instructions++
		if _, ok := instruction.(*ssa.RunDefers); ok && firstRun == 0 {
			firstRun = instructions
		}
	}
	if firstRun == 0 || firstRun >= instructions {
		t.Fatal("expected a deferred observation before the completed census")
	}
	cut := proofs.NewSearchBudget(firstRun)
	if points, available := deferredObservationPoints(function, cut); available || len(points) != 0 || !cut.Exhausted() {
		t.Fatalf("cut retained observation prefix: %d points, available=%v", len(points), available)
	}
	if points, available := deferredObservationPoints(function, proofs.NewSearchBudget(proofs.QueryBudget)); !available || len(points) < 2 {
		t.Fatalf("fresh census did not recover: %d points, available=%v", len(points), available)
	}
}
