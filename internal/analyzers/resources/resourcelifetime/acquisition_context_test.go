package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAcquisitionContextCancellationAllowance(t *testing.T) {
	pkg := acquisitionContextFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"exact", true},
		{"cause", true},
		{"begin", true},
		{"query", true},
		{"later", false},
		{"deferred", false},
		{"conditional", false},
		{"sibling", false},
		{"replaced", false},
		{"conn", false},
		{"ordinary", false},
		{"txRows", false},
		{"stmtRows", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := contextAcquisitionCall(t, pkg.Func(test.name))
			if test.name == "conn" || test.name == "ordinary" || test.name == "txRows" || test.name == "stmtRows" || test.name == "opaque" {
				if proof := proveAcquisitionContextCanceledWithin(call, proofs.NewSearchBudget(0)); proof.State != proofs.EvidenceDisproven {
					t.Fatalf("noneligible API queried cancellation: %+v", proof)
				}
				return
			}
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return proveAcquisitionContextCanceledWithin(call, budget)
			}, test.want)
		})
	}
}

func TestAcquisitionContextCancellationChildCutoff(t *testing.T) {
	call := contextAcquisitionCall(t, acquisitionContextFixture(t).Func("exact"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := proveAcquisitionContextCanceledWithin(call, pool.Within(1))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("interrupted cancellation = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	fresh := proveAcquisitionContextCanceledWithin(call, pool.Within(releaseSearchBudget))
	if !fresh.Proven() || fresh.Reason != resourceReasonCanceledAcquisition {
		t.Fatalf("fresh cancellation = %+v", fresh)
	}
}

func TestAcquisitionContextCancellationFlow(t *testing.T) {
	pkg := acquisitionContextFixture(t)
	for _, name := range []string{"exact", "later"} {
		t.Run(name, func(t *testing.T) {
			call := contextAcquisitionCall(t, pkg.Func(name))
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			resource := ssaflow.InstructionsOf[*ssa.Extract](call.Parent())
			var result ssa.Value
			for _, value := range resource {
				if value.Tuple == call && value.Index == 0 {
					result = value
				}
			}
			if result == nil {
				t.Fatal("missing resource result")
			}
			got := evaluateResourceFlow(nil, evidence, call, result, resourceContract{
				family: resourceFamilySQL, packagePath: "database/sql", cleanup: []string{"Close"},
			})
			if name == "exact" {
				if got.state != proofs.EvidenceDisproven || got.reason != resourceReasonCanceledAcquisition || got.leak != nil {
					t.Fatalf("canceled acquisition reported = %+v", got)
				}
			} else if got.state != proofs.EvidenceProven || got.leak == nil {
				t.Fatalf("later cancellation lost independent statement leak = %+v", got)
			}
		})
	}
}

func contextAcquisitionCall(t *testing.T, fn *ssa.Function) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		switch ssaflow.CallName(call.Common()) {
		case "PrepareContext", "Prepare", "BeginTx", "QueryContext":
			return call
		}
	}
	t.Fatal("missing acquisition")
	return nil
}

func acquisitionContextFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "acquisitioncontext", `package acquisitioncontext
import("context";"database/sql")
func opaque(db *sql.DB,p context.Context){_,_=db.PrepareContext(p,"q")}
func exact(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.PrepareContext(ctx,"q")}
func cause(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancelCause(p);cancel(nil);_,_=db.PrepareContext(ctx,"q")}
func begin(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.BeginTx(ctx,nil)}
func query(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.QueryContext(ctx,"q")}
func later(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);_,_=db.PrepareContext(ctx,"q");cancel()}
func deferred(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);defer cancel();_,_=db.PrepareContext(ctx,"q")}
func conditional(db *sql.DB,p context.Context,yes bool){ctx,cancel:=context.WithCancel(p);if yes{cancel()};_,_=db.PrepareContext(ctx,"q")}
func sibling(db *sql.DB,p context.Context){ctx,_:=context.WithCancel(p);_,cancel:=context.WithCancel(p);cancel();_,_=db.PrepareContext(ctx,"q")}
func replaced(db *sql.DB,p context.Context){ctx,cancel:=context.WithCancel(p);other,_:=context.WithCancel(p);cancel();ctx=other;_,_=db.PrepareContext(ctx,"q")}
func conn(db *sql.Conn,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.PrepareContext(ctx,"q")}
func ordinary(db *sql.DB,p context.Context){_,cancel:=context.WithCancel(p);cancel();_,_=db.Prepare("q")}
func txRows(db *sql.Tx,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.QueryContext(ctx,"q")}
func stmtRows(db *sql.Stmt,p context.Context){ctx,cancel:=context.WithCancel(p);cancel();_,_=db.QueryContext(ctx)}
`)
}
