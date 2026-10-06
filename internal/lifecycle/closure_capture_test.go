package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestPossibleClosureCapture(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "choicecapture", `package choicecapture
func register(func())
type callback func()
func mixed(ch, other chan int, flag bool) { var fn func()
if flag { fn = func() { <-ch } } else { fn = func() { <-other } }; register(fn) }
func unrelated(ch, other chan int, flag bool) { var fn func()
if flag { fn = func() { <-other } } else { fn = func() {} }; register(fn) }
func converted(ch, other chan int, flag bool) { var fn func()
if flag { fn = func() { <-ch } } else { fn = func() { <-other } }; go callback(fn)() }
`)
	for _, test := range []struct {
		name   string
		reason proofs.EvidenceReason
	}{
		{"mixed", proofs.EvidenceCapturedByClosure},
		{"unrelated", proofs.EvidenceNotFound},
		{"converted", proofs.EvidenceNotFound},
	} {
		fn := pkg.Func(test.name)
		var value ssa.Value
		if test.name == "converted" {
			value = ssaflow.InstructionsOf[*ssa.Go](fn)[0].Common().Value
		} else {
			value = ssaflow.InstructionsOf[*ssa.Call](fn)[0].Common().Args[0]
		}
		pool := proofs.NewSearchBudget(4 * proofs.QueryBudget)
		cut := ProvePossibleClosureCaptureWithin(value, fn.Params[0], pool.Within(0))
		if cut.State != proofs.EvidenceUnknown || cut.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
			t.Fatalf("%s cutoff: %+v", test.name, cut)
		}
		fresh := ProvePossibleClosureCaptureWithin(value, fn.Params[0], pool.Within(proofs.QueryBudget))
		want := proofs.EvidenceDisproven
		if test.name == "mixed" {
			want = proofs.EvidenceProven
		}
		if fresh.Reason != test.reason || fresh.State != want {
			t.Fatalf("%s fresh: %+v", test.name, fresh)
		}
	}
}
