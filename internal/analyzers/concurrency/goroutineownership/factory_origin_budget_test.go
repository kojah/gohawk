package goroutineownership

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestFactoryOriginAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "origins", `package origins
type channel chan int
type group struct{}
type groupPointer *group
type owner struct{ member group }
func opaqueSignal() channel
func opaqueGroup() *group
func opaqueOwner() *owner
func freshGroup() *group { return new(group) }
func signalDirect() chan int { value:=opaqueSignal(); go func(){}(); return value }
func signalWrapped() chan int { value:=chan int(opaqueSignal()); go func(){}(); return value }
func signalPhi(flag bool) chan int { value:=chan int(opaqueSignal()); if flag{value=make(chan int)}; go func(){}(); return value }
func signalLocal() chan int { value:=make(chan int); go func(){}(); return value }
func groupDirect() *group { value:=opaqueGroup(); go func(){}(); return value }
func groupWrapped() groupPointer { value:=groupPointer(opaqueGroup()); go func(){}(); return value }
func groupPhi(flag bool) *group { value:=opaqueGroup(); if flag{value=new(group)}; go func(){}(); return value }
func groupField() *group { value:=&opaqueOwner().member; go func(){}(); return value }
func groupFresh() *group { value:=freshGroup(); go func(){}(); return value }
func groupLocal() *group { value:=new(group); go func(){}(); return value }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"signalDirect", true},
		{"signalWrapped", true},
		{"signalPhi", true},
		{"signalLocal", false},
		{"groupDirect", true},
		{"groupWrapped", true},
		{"groupPhi", true},
		{"groupField", true},
		{"groupFresh", false},
		{"groupLocal", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertFactoryOriginQuery(t, pkg.Func(test.name), strings.HasPrefix(test.name, "signal"), test.want)
		})
	}
}

func assertFactoryOriginQuery(t *testing.T, function *ssa.Function, signal, want bool) {
	t.Helper()
	var dump bytes.Buffer
	if _, err := function.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	value := ssaflow.InstructionsOf[*ssa.Return](function)[0].Results[0]
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	query := func(budget *ssaflow.SearchBudget) ssaflow.Proof {
		if signal {
			return helperSignalOrigin(value, spawn, budget)
		}
		return opaqueGroupOrigin(value, budget)
	}
	fresh := query(ssaflow.NewSearchBudget(ssaflow.QueryBudget))
	if !fresh.Known() || fresh.Proven() != want {
		t.Fatalf("fresh origin = %+v, want proven=%v", fresh, want)
	}
	for _, limit := range []int{0, 1} {
		budget := ssaflow.NewSearchBudget(limit)
		proof := query(budget)
		if proof.Known() || !budget.Exhausted() {
			t.Fatalf("limit %d must stop before completing fold and storage leaf: %+v", limit, proof)
		}
	}
	for limit := range 64 {
		budget := ssaflow.NewSearchBudget(limit)
		proof := query(budget)
		if budget.Exhausted() && (proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted) {
			t.Fatalf("limit %d returned completed proof after cutoff: %+v", limit, proof)
		}
		if proof.Known() && proof.Proven() != want {
			t.Fatalf("limit %d changed origin policy: %+v", limit, proof)
		}
	}
}
