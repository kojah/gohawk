package lockorder

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"

	"github.com/kojah/gohawk/internal/analyzertest"
	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestReadLockFieldContractsUnknown(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "readlockfields")
}

func TestReadLockFieldEvidenceAvailability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fieldcontracts", `package fieldcontracts
 type state struct{a,b int;peer *state}
 type other struct{a int}
 func opaque(){}
 func(s *state) reset(){s.a=0}
 func(s *state) called(){s.b=0;opaque()}
 func(s *state) loaded(){s.peer.b=0}
 func(o *other) reset(){o.a=0}
 func(s *state) inspect(){s.a++;s.b++}
 `)
	method := func(name, member string) *ssa.Function {
		named := pkg.Pkg.Scope().Lookup(name).Type().(*types.Named)
		for object := range named.Methods() {
			if object.Name() == member {
				return pkg.Prog.FuncValue(object)
			}
		}
		t.Fatalf("missing method %s.%s", name, member)
		return nil
	}
	functions := []*ssa.Function{method("state", "reset"), method("state", "called"), method("state", "loaded"), method("other", "reset")}
	stores := ssaflow.InstructionsOf[*ssa.Store](method("state", "inspect"))
	pool := proofs.NewSearchBudget(lockStateWorkBudget)
	fields := collectReadLockFieldEvidence(functions, pool)
	if fields.unavailable || fields.guard(stores[0]).state != proofs.EvidenceUnknown || (fields.guard(stores[1]).state == proofs.EvidenceUnknown) {
		t.Fatalf("field-specific evidence = %+v", fields)
	}
	otherOnly := collectReadLockFieldEvidence(functions[3:], proofs.NewSearchBudget(lockStateWorkBudget))
	if otherOnly.guard(stores[0]).state == proofs.EvidenceUnknown {
		t.Fatal("same field spelling on another type supplies evidence")
	}
	cut := collectReadLockFieldEvidence(functions, pool.Within(0))
	if !cut.unavailable || cut.guard(stores[0]).state != proofs.EvidenceUnknown || cut.guard(stores[1]).state != proofs.EvidenceUnknown {
		t.Fatal("cutoff supplied negative field evidence")
	}
	fresh := collectReadLockFieldEvidence(functions, pool.Within(lockStateWorkBudget))
	if fresh.unavailable || fresh.guard(stores[0]).state != proofs.EvidenceUnknown || (fresh.guard(stores[1]).state == proofs.EvidenceUnknown) {
		t.Fatal("fresh allowance failed to recover field-specific evidence")
	}
}
