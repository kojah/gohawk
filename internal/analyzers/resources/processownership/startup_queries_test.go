package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestSuccessfulStartReturnAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "startreturn", `package startreturn
 import "os/exec"
 func returning(cmd *exec.Cmd)error{if err:=cmd.Start();err!=nil{return err};return nil}
 func looping(cmd *exec.Cmd)error{if err:=cmd.Start();err!=nil{return err};for{}}
 func panicking(cmd *exec.Cmd)error{if err:=cmd.Start();err!=nil{return err};panic("done")}
 `)
	for _, test := range []struct {
		name string
		want ssaflow.EvidenceState
	}{
		{"returning", ssaflow.EvidenceDisproven}, {"looping", ssaflow.EvidenceProven}, {"panicking", ssaflow.EvidenceProven},
	} {
		start := startupTestCall(t, pkg.Func(test.name))
		if test.name == "looping" {
			pool := ssaflow.NewSearchBudget(processPoolBudget)
			child := pool.Within(8)
			got := successfulStartCannotReturn(start, child)
			if got.State != ssaflow.EvidenceUnknown || got.Reason != ssaflow.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
				t.Fatalf("successful branch bypassed its allowance: %+v", got)
			}
		}
		checkProcessQuery(t, test.name, test.want, func(budget *ssaflow.SearchBudget) ssaflow.Proof { return successfulStartCannotReturn(start, budget) })
	}
}

func TestLaterWatcherAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "watchercut", `package watchercut
 import "os/exec"
 type holder struct{cmd *exec.Cmd}
 func watched(cmd *exec.Cmd,owner *holder){cmd.Start();go func(){println(owner)}()}
 func before(cmd *exec.Cmd,owner *holder){go func(){println(owner)}();cmd.Start()}
 func unrelated(cmd *exec.Cmd,owner *holder){cmd.Start();go func(){println(1)}()}
 `)
	for _, test := range []struct {
		name string
		want ssaflow.EvidenceState
	}{
		{"watched", ssaflow.EvidenceProven}, {"before", ssaflow.EvidenceDisproven}, {"unrelated", ssaflow.EvidenceDisproven},
	} {
		fn := pkg.Func(test.name)
		start := startupTestCall(t, fn)
		if test.name == "watched" {
			count := 0
			for range ssaflow.InstructionsWithin(fn, nil) {
				count++
			}
			child := ssaflow.NewSearchBudget(processQueryBudget).Within(count)
			result := laterProcessOwnerWatcher(fn, start, []ssa.Value{fn.Params[1]}, child)
			if result.State != ssaflow.EvidenceUnknown || result.Reason != ssaflow.EvidenceBudgetExhausted || !child.Exhausted() {
				t.Fatalf("containment bypassed body-only allowance: %+v", result)
			}
		}
		checkProcessQuery(t, test.name, test.want, func(budget *ssaflow.SearchBudget) ssaflow.Proof {
			return laterProcessOwnerWatcher(fn, start, []ssa.Value{fn.Params[1]}, budget)
		})
	}
}

func checkProcessQuery(t *testing.T, name string, want ssaflow.EvidenceState, query func(*ssaflow.SearchBudget) ssaflow.Proof) {
	t.Helper()
	pool := ssaflow.NewSearchBudget(processPoolBudget)
	for limit := range 1000 {
		child := pool.Within(limit)
		result := query(child)
		if limit == 0 && !child.Exhausted() {
			t.Fatalf("%s bypassed zero allowance: %+v", name, result)
		}
		if !child.Exhausted() {
			if result.State != want {
				t.Fatalf("%s complete: %+v want %v", name, result, want)
			}
			return
		}
		if result.State != ssaflow.EvidenceUnknown || result.Reason != ssaflow.EvidenceBudgetExhausted || pool.Exhausted() {
			t.Fatalf("%s cutoff: %+v", name, result)
		}
		fresh := query(pool.Within(processQueryBudget))
		if fresh.State != want || fresh.Reason == ssaflow.EvidenceBudgetExhausted {
			t.Fatalf("%s fresh: %+v", name, fresh)
		}
	}
	t.Fatalf("%s query never completed", name)
}

func startupTestCall(t *testing.T, fn *ssa.Function) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if start, _, ok := startedCommand(call); ok {
			return start
		}
	}
	t.Fatal("compiled SSA has no Start")
	return nil
}

func TestStartupWrapperDeferredLaunches(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "startupdeferred", `package startupdeferred
 import("os/exec";"testing")
 type holder struct{cmd *exec.Cmd}
 func(owner *holder)close(){}
 func(owner *holder)Close(){owner.close()}
 func nested(owner *holder){defer owner.Close()}
 func called(cmd *exec.Cmd,owner *holder){owner.Close();cmd.Start();go func(){println(owner)}()}
 func nestedCall(cmd *exec.Cmd,owner *holder){nested(owner);cmd.Start();go func(){println(owner)}()}
 func spawned(cmd *exec.Cmd,owner *holder){go owner.Close();cmd.Start();go func(){println(owner)}()}
 func deferred(cmd *exec.Cmd,owner *holder){defer owner.Close();cmd.Start();go func(){println(owner)}()}
 func unwatched(cmd *exec.Cmd,owner *holder){defer owner.Close();cmd.Start()}
 func registered(cmd *exec.Cmd,owner *holder,t *testing.T){t.Cleanup(func(){owner.Close()});cmd.Start();go func(){println(owner)}()}
 `)
	for _, test := range []struct {
		name     string
		want     bool
		deferred bool
	}{
		{"called", false, false},
		{"nestedCall", false, false},
		{"spawned", false, false},
		{"deferred", true, true},
		{"unwatched", false, true},
		{"registered", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			start := startupTestCall(t, fn)
			var before []ssa.Instruction
			for instruction := range ssaflow.InstructionsStrictlyDominatingWithin(start, nil) {
				before = append(before, instruction)
			}
			for _, limit := range []int{0, processPoolBudget} {
				proof := &commandProof{pool: ssaflow.NewSearchBudget(limit), evidence: lifecyclefacts.NewLifecycleEvidence(nil, "test", "startup-wrapper")}
				got := processOwnerDominatesStart(proof, fn, start, []ssa.Value{fn.Params[1]}, before)
				want := test.want
				if limit == 0 {
					want = test.deferred
				}
				if got != want {
					t.Fatalf("allowance %d: accepted=%v, want %v", limit, got, want)
				}
				if !test.deferred && proof.pool.Exhausted() {
					t.Fatal("non-deferred launch spent completion allowance")
				}
			}
		})
	}
}
