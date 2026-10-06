package heapmodel

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestProjectionCutoffPreservesMutationBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", projectionBoundaryFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"accepted", true},
		{"readOnlyRoot", true},
		{"readOnlySlot", true},
		{"convertedReadOnlyRoot", true},
		{"escapedLater", true},
		{"convertedEscapedRoot", false},
		{"convertedEscapedSlot", false},
		{"reassigned", false},
		{"escapedRoot", false},
		{"escapedAddress", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, observation := projectionQuery(t, pkg.Func(test.name))
			argument := observation.Common().Args[0]
			for allowance := 1; allowance <= proofs.QueryBudget; allowance++ {
				budget := proofs.NewSearchBudget(allowance)
				proof := NewStorage(budget).Projection(argument, root, observation)
				if budget.Exhausted() {
					if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut %d published evidence: %+v", allowance, proof)
					}
					continue
				}
				if proof.Proven() != test.want {
					t.Fatalf("complete %d: %+v, want proven=%v", allowance, proof, test.want)
				}
				return
			}
			t.Fatal("small query never completed")
		})
	}
}

func TestProjectionRetainsIndependentPathCutoff(t *testing.T) {
	source := `package projectionprobe
 type node struct { child *node; body *int }
 func acquire() *node { return nil }
 func cleanup(*int){}
 func deep(){p:=acquire(); cleanup(p.` + strings.Repeat("child.", proofs.QueryBudget+1) + `body)}
 func shallow(){p:=acquire(); cleanup(p.body)}
 `
	pkg := ssaflowtest.BuildPackage(t, "projectionprobe", source)
	budget := proofs.NewSearchBudget(10 * proofs.QueryBudget)
	storage := NewStorage(budget)
	root, observation := projectionQuery(t, pkg.Func("deep"))
	proof := storage.Projection(observation.Common().Args[0], root, observation)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted || budget.Exhausted() {
		t.Fatalf("child cutoff lost: %+v, parent exhausted=%v", proof, budget.Exhausted())
	}
	root, observation = projectionQuery(t, pkg.Func("shallow"))
	if proof := storage.Projection(observation.Common().Args[0], root, observation); !proof.Proven() {
		t.Fatalf("independent cutoff poisoned a fresh query: %+v", proof)
	}
}

func TestProjectionObservationWindowCutoffIsUnavailable(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", projectionBoundaryFixture)
	fn := pkg.Func("reassigned")
	root, observation := projectionQuery(t, fn)
	origin := root.(ssa.Instruction)
	stores := ssaflow.InstructionsOf[*ssa.Store](fn)
	if len(stores) != 1 {
		t.Fatal("fixture lost selected field replacement")
	}
	cut := proofs.NewSearchBudget(1)
	if instructionWithinObservation(stores[0], origin, observation, cut) || !cut.Exhausted() {
		t.Fatal("observation window bypassed caller allowance")
	}
	if !instructionWithinObservation(stores[0], origin, observation, proofs.NewSearchBudget(proofs.QueryBudget)) {
		t.Fatal("fresh query lost mutating use inside observation window")
	}
	cut = proofs.NewSearchBudget(1)
	if NewStorage(cut).addressDoesNotEscapeBetween(stores[0].Addr, origin, observation, map[ssa.Value]bool{}) || !cut.Exhausted() {
		t.Fatal("shortened observation window certified stable storage")
	}
}

func TestEmbeddedFieldSetupSharesStorageAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fieldprobe", `package fieldprobe
 type inner struct { body *int }
 type owner struct { nested inner; sibling int }
 func observe(*owner){}
 func stable(){p:=new(owner); p.nested.body=new(int); p.sibling++; observe(p)}
 func changed(){p:=new(owner); p.nested.body=new(int); observe(p); p.nested.body=nil}
 `)
	for _, name := range []string{"stable", "changed"} {
		fn := pkg.Func(name)
		var address ssa.Value
		for _, field := range ssaflow.InstructionsOf[*ssa.FieldAddr](fn) {
			if _, nested := field.X.(*ssa.FieldAddr); nested {
				address = field
				break
			}
		}
		observation := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
		if address == nil {
			t.Fatal("fixture lost nested field address")
		}
		assertEmbeddedFieldSetupCutoff(t, address)
		completed := false
		for allowance := 1; allowance <= proofs.QueryBudget; allowance++ {
			budget := proofs.NewSearchBudget(allowance)
			proof := NewStorage(budget).StableFieldContent(address, observation)
			if budget.Exhausted() {
				if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
					t.Fatalf("%s cut %d: %+v", name, allowance, proof)
				}
				continue
			}
			if proof.Proven() != (name == "stable") {
				t.Fatalf("%s complete %d: %+v", name, allowance, proof)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("small embedded field query never completed")
		}
	}
}

func projectionQuery(t *testing.T, fn *ssa.Function) (ssa.Value, *ssa.Call) {
	t.Helper()
	var root ssa.Value
	var observation *ssa.Call
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		switch ssaflow.CallName(call.Common()) {
		case "acquire":
			if root == nil {
				root = call
			}
		case "cleanup":
			observation = call
		}
	}
	if root == nil || observation == nil {
		t.Fatal("fixture lost acquisition or observation")
	}
	return root, observation
}

func assertEmbeddedFieldSetupCutoff(t *testing.T, address ssa.Value) {
	t.Helper()
	cut := proofs.NewSearchBudget(1)
	path, known := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(cut), address,
		func(value ssa.Value) bool { _, fresh := value.(*ssa.Alloc); return fresh })
	if known || !cut.Exhausted() || path.Depth != 0 {
		t.Fatalf("fixture did not cut during embedded setup: %+v, known=%v", path, known)
	}
	// Three visits suffice to name this allocation/field/field location.
	// Embedded setup must share them even when no observation is supplied.
	setup := proofs.NewSearchBudget(3)
	if proof := NewStorage(setup).StableFieldContent(address, nil); !setup.Exhausted() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("embedded setup bypassed storage allowance: %+v", proof)
	}
}
