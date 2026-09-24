package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestInterfaceDispatchUsesExactReceiver(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "dispatch", `package dispatch
type actor interface { Run() }
type worker struct{}
func (*worker) Run() {}
func exact(w *worker) { var a actor = w; a.Run() }
func different(w, v *worker, flag bool) { var a actor = w; if flag { a = v }; a.Run() }
func unknown(a actor) { a.Run() }
`)
	for _, name := range []string{"exact", "different", "unknown"} {
		for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name)) {
			proof := ssaflow.ResolveInterfaceDispatch(call.Common(), pkg.Prog, ssaflow.NewSearchBudget(ssaflow.QueryBudget))
			if !call.Common().IsInvoke() || len(call.Common().Args) != 0 {
				t.Error("dispatch resolution mutated the source call")
			}
			if proof.Proven() != (name == "exact") {
				t.Errorf("%s: %+v", name, proof)
			}
			if proof.Proven() && proof.Receiver != pkg.Func(name).Params[0] {
				t.Errorf("%s bound wrong receiver: %+v", name, proof)
			}
		}
	}
}
