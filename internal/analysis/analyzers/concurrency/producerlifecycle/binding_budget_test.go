package producerlifecycle

import (
	"fmt"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestNonReceivingBindingAllowance(t *testing.T) {
	var params, args []string
	for index := range 64 {
		params = append(params, fmt.Sprintf("unused%d int", index))
		args = append(args, "0")
	}
	pkg := ssaflowtest.BuildPackage(t, "producerbindings", "package producerbindings;func worker(ch chan int,"+
		strings.Join(params, ",")+",opaque func()){opaque();ch<-1};func subject(ch chan int,opaque func()){go worker(ch,"+
		strings.Join(args, ",")+",opaque)}")
	fn := pkg.Func("subject")
	launch := ssaflow.InstructionsOf[*ssa.Go](fn)[0]
	t.Log(launch.String())
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	cut := pool.Within(32)
	if proof := nonReceivingUses(launch, fn.Params[0], cut); proof.State != proofs.EvidenceUnknown || !cut.Exhausted() || pool.Exhausted() {
		t.Fatalf("partial bindings=%+v exhausted=%v/%v", proof, cut.Exhausted(), pool.Exhausted())
	}
	fresh := pool.Within(proofs.SummaryBudget)
	if proof := nonReceivingUses(launch, fn.Params[0], fresh); !proof.Proven() || proof.Reason != reasonWorkerChannelUsesComplete || fresh.Exhausted() {
		t.Fatalf("fresh bindings=%+v exhausted=%v", proof, fresh.Exhausted())
	}
}
