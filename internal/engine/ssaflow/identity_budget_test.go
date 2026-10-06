package ssaflow

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStructuralIdentitySharedBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "identity", `package identity
func merged(flag bool, x int) int64 { var v int64; if flag { v=int64(x) } else { v=int64(x) }; return v }
func loads(p *int) (int,int) { a:=*p; b:=*p; return a,b }
`)
	function := pkg.Func("merged")
	value := InstructionsOf[*ssa.Return](function)[0].Results[0]
	if _, ok := value.(*ssa.Phi); !ok {
		t.Fatal("expected an actual phi with converted alternatives")
	}
	cutoff := proofs.NewSearchBudget(2)
	if StructurallyIdenticalWithin(value, function.Params[1], cutoff) || !cutoff.Exhausted() {
		t.Fatal("a partial phi/wrapper comparison cannot prove identity")
	}
	pool := proofs.NewSearchBudget(2)
	shared := pool.Within(proofs.QueryBudget)
	if StructurallyIdenticalWithin(value, function.Params[1], shared) || !shared.PoolExhausted() {
		t.Fatal("nested structural folds must share their candidate pool")
	}
	fresh := proofs.NewSearchBudget(proofs.QueryBudget)
	if !StructurallyIdenticalWithin(value, function.Params[1], fresh) || fresh.Exhausted() || !StructurallyIdentical(value, function.Params[1]) {
		t.Fatal("fresh phi/wrapper identity must preserve default policy")
	}
	returned := InstructionsOf[*ssa.Return](pkg.Func("loads"))[0]
	if returned.Results[0] == returned.Results[1] {
		t.Fatal("expected distinct actual SSA loads")
	}
	if StructurallyIdenticalWithin(returned.Results[0], returned.Results[1], proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("structural identity must keep distinct loads unproved")
	}
	zero := proofs.NewSearchBudget(0)
	if StructurallyIdenticalWithin(value, value, zero) || !zero.Exhausted() {
		t.Fatal("even same-value comparison must charge its allowance")
	}
}

func identityProjectionFixture(t *testing.T) (*ssa.Function, []ssa.Value) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "paths", `package paths
 type owner struct { values [2]int }
 func projections(a,b *owner, i int) (*int,*int,*int,*int) { return &a.values[1],&b.values[1],&b.values[0],&a.values[i] }
`)
	function := pkg.Func("projections")
	return function, InstructionsOf[*ssa.Return](function)[0].Results
}

func TestAccessPathBudgetAndPolicy(t *testing.T) {
	function, values := identityProjectionFixture(t)
	left := AccessPath{Value: values[0], Root: function.Params[0]}
	want := []string{"field:0", "index:1"}
	fresh := proofs.NewSearchBudget(proofs.QueryBudget)
	steps, ok := AccessPathStepsWithin(left.Value, left.Root, fresh)
	if !ok || !slices.Equal(steps, want) || fresh.Exhausted() {
		t.Fatal("fresh projection must preserve exact static path")
	}
	steps, ok = AccessPathSteps(left.Value, left.Root)
	if !ok || !slices.Equal(steps, want) || !ValueIsAccessPathFromWithin(left.Value, left.Root, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("default and bounded projection policies differ")
	}
	cutoff := proofs.NewSearchBudget(1)
	if steps, ok := AccessPathStepsWithin(left.Value, left.Root, cutoff); ok || steps != nil || !cutoff.Exhausted() {
		t.Fatal("projection cutoff must not publish a partial path")
	}
	dynamic := proofs.NewSearchBudget(proofs.QueryBudget)
	if ValueIsAccessPathFromWithin(values[3], function.Params[0], dynamic) || dynamic.Exhausted() {
		t.Fatal("dynamic index must remain opaque even with available allowance")
	}
}

func TestCorrespondingPathIdentityBudget(t *testing.T) {
	function, values := identityProjectionFixture(t)
	left := AccessPath{Value: values[0], Root: function.Params[0]}
	right := AccessPath{Value: values[1], Root: function.Params[1]}
	if proof := ProveIdentityWithin(left, right, proofs.NewSearchBudget(2)); proof.State != proofs.EvidenceUnknown ||
		proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatal("unfinished identity must be unknown with budget reason")
	}
	full := proofs.NewSearchBudget(proofs.QueryBudget)
	proof := ProveIdentityWithin(left, right, full)
	defaultProof := ProveIdentityWithin(left, right, nil)
	if !proof.Proven() || proof.Reason != proofs.EvidenceSameAccessPath || full.Exhausted() ||
		proof != defaultProof || !SameAccessPathWithin(left, right, nil) {
		t.Fatal("corresponding paths must retain the exact default proof")
	}
	// Leave one fewer step than the completed query needs: path discovery
	// succeeds, but the final path comparison still cannot publish a proof.
	lastStep := proofs.NewSearchBudget(proofs.QueryBudget - full.Remaining() - 1)
	if proof := ProveIdentityWithin(left, right, lastStep); proof.State != proofs.EvidenceUnknown ||
		proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatal("final path comparison must spend the same allowance")
	}
	different := AccessPath{Value: values[2], Root: function.Params[1]}
	proof = ProveIdentityWithin(left, different, proofs.NewSearchBudget(proofs.QueryBudget))
	if proof.State != proofs.EvidenceDisproven || proof.Reason != proofs.EvidenceNotFound || SameAccessPathWithin(left, different, nil) {
		t.Fatal("completed differing constant-index paths must remain distinct")
	}
}

func TestSameAccessPathWithinKeepsPathOnlyPolicy(t *testing.T) {
	function, values := identityProjectionFixture(t)
	left := AccessPath{Value: values[0], Root: function.Params[0]}
	right := AccessPath{Value: values[1], Root: function.Params[1]}
	cut := proofs.NewSearchBudget(1)
	if SameAccessPathWithin(left, right, cut) || !cut.Exhausted() {
		t.Fatal("corresponding paths bypassed caller allowance")
	}
	if !SameAccessPathWithin(left, right, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("fresh corresponding paths failed")
	}
	dynamic := AccessPath{Value: values[3], Root: function.Params[0]}
	if !ProveIdentityWithin(dynamic, dynamic, nil).Proven() {
		t.Fatal("fixture lost direct identity")
	}
	if SameAccessPathWithin(dynamic, dynamic, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("direct identity bypassed dynamic-index path policy")
	}
}
