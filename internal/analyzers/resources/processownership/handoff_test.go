package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestProcessClosureChoices(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "processchoices")
}

func TestPossibleCallbackCapture(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "choicecapture", `package choicecapture
 import "os/exec"
 func register(func()error)
 type callback func()error
 func mixed(cmd,other *exec.Cmd,flag bool){var fn func()error
 if flag {fn=func()error{return cmd.Wait()}}else{fn=func()error{return other.Wait()}}
 register(fn)}
 func unrelated(cmd,other *exec.Cmd,flag bool){var fn func()error
 if flag {fn=func()error{return other.Wait()}}else{fn=func()error{return nil}}
 register(fn)}
 func converted(cmd,other *exec.Cmd,flag bool){var fn func()error
 if flag {fn=func()error{return cmd.Wait()}}else{fn=func()error{return other.Wait()}}
 go callback(fn)()}
 `)
	for _, test := range []struct {
		name   string
		reason ssaflow.EvidenceReason
	}{
		{"mixed", ssaflow.EvidenceCapturedByClosure},
		{"unrelated", ssaflow.EvidenceNotFound},
		{"converted", ssaflow.EvidenceNotFound},
	} {
		fn := pkg.Func(test.name)
		var value ssa.Value
		if test.name == "converted" {
			value = ssaflow.InstructionsOf[*ssa.Go](fn)[0].Common().Value
		} else {
			value = ssaflow.InstructionsOf[*ssa.Call](fn)[0].Common().Args[0]
		}
		pool := ssaflow.NewSearchBudget(processPoolBudget)
		cut := provePossibleCallbackCapture(value, fn.Params[0], pool.Within(0))
		if cut.State != ssaflow.EvidenceUnknown || cut.Reason != ssaflow.EvidenceBudgetExhausted || pool.Exhausted() {
			t.Fatalf("%s cutoff: %+v", test.name, cut)
		}
		fresh := provePossibleCallbackCapture(value, fn.Params[0], pool.Within(processQueryBudget))
		want := ssaflow.EvidenceDisproven
		if test.name == "mixed" {
			want = ssaflow.EvidenceUnknown
		}
		if fresh.Reason != test.reason || fresh.State != want {
			t.Fatalf("%s fresh: %+v", test.name, fresh)
		}
	}
}

func TestPossibleNonreturningWaitAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "waitreach", `package waitreach
 import "os/exec"
 func endless(cmd *exec.Cmd){go func(){cmd.Wait();for{}}()}
 func returning(cmd *exec.Cmd){go func(){cmd.Wait()}()}
 func unrelated(cmd *exec.Cmd){go func(){for{}}()}
 `)
	for _, name := range []string{"endless", "returning", "unrelated"} {
		fn := pkg.Func(name)
		instruction := ssaflow.InstructionsOf[*ssa.Go](fn)[0]
		pool := ssaflow.NewSearchBudget(processPoolBudget)
		cut := provePossibleWaitHandoff(instruction, fn.Params[0], pool.Within(0))
		if cut.Reason != ssaflow.EvidenceBudgetExhausted || cut.State != ssaflow.EvidenceUnknown || pool.Exhausted() {
			t.Fatalf("%s cutoff: %+v", name, cut)
		}
		fresh := provePossibleWaitHandoff(instruction, fn.Params[0], pool.Within(processQueryBudget))
		want := ssaflow.EvidenceDisproven
		if name == "endless" {
			want = ssaflow.EvidenceUnknown
		}
		if fresh.State != want || fresh.Reason == ssaflow.EvidenceBudgetExhausted {
			t.Fatalf("%s fresh: %+v", name, fresh)
		}
	}
}
