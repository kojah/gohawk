package ssaflow

import (
	"go/token"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// Each function returns a load whose storage query must give up for one
// specific reason. The reason is what a caller prints, so it must name the
// write, use, or merge that stopped the proof rather than "unavailable".
const storageGiveUpFixture = `package ssaflowtest
type box struct{ value *int; other *int }
func conflicting(flag bool) *int {
	var cell *int
	p := &cell
	if flag { *p = new(int) } else { *p = new(int) }
	return *p
}
func escaped(sink func(**int)) *int {
	var cell *int
	cell = new(int)
	sink(&cell)
	return cell
}
func partial() box {
	var b box
	b.value = new(int)
	return b
}
func param(p *box) *int { return p.value }
func stable(after *int) func() *int {
	var cell *int
	cell = new(int)
	f := func() *int { return cell }
	cell = after
	return f
}
`

func returnedLoad(t *testing.T, function *ssa.Function) *ssa.UnOp {
	t.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || len(returned.Results) != 1 {
				continue
			}
			if load, ok := returned.Results[0].(*ssa.UnOp); ok && load.Op == token.MUL {
				return load
			}
		}
	}
	t.Fatalf("%s returns no load", function.Name())
	return nil
}

func TestStorageGiveUpsNameTheirCause(t *testing.T) {
	pkg := buildTestSSA(t, storageGiveUpFixture)
	want := map[string]EvidenceReason{
		"conflicting": EvidenceStorageConflictingWrites,
		"escaped":     EvidenceStorageAddressEscapes,
		"partial":     EvidenceStoragePartialWrite,
		"param":       EvidenceStorageNotLocal,
	}
	for name, reason := range want {
		t.Run(name, func(t *testing.T) {
			var observed []string
			observer := func(reason string, _ token.Pos, _ map[string]string) { observed = append(observed, reason) }
			load := returnedLoad(t, pkg.Func(name))
			content := NewStorage(NewSearchBudget(1000).Observed(observer)).Content(load.X, load)
			if content.Proven() || content.Reason != reason {
				t.Fatalf("Content = %+v, want reason %s", content.Proof, reason)
			}
			if len(observed) == 0 || observed[len(observed)-1] != string(reason) {
				t.Fatalf("observed %v, want %s last", observed, reason)
			}
		})
	}
	t.Run("stable", func(t *testing.T) {
		function := pkg.Func("stable")
		closure := findMakeClosure(t, function)
		cell := closure.Bindings[0]
		content := NewStorage(NewSearchBudget(1000)).StableContent(cell, closure)
		if content.Proven() || content.Reason != EvidenceStorageWriteAfterObservation {
			t.Fatalf("StableContent = %+v, want write after observation", content.Proof)
		}
	})
	t.Run("budget", func(t *testing.T) {
		load := returnedLoad(t, pkg.Func("conflicting"))
		content := NewStorage(NewSearchBudget(1)).Content(load.X, load)
		if content.Proven() || content.Reason != EvidenceBudgetExhausted {
			t.Fatalf("Content = %+v, want budget exhausted", content.Proof)
		}
	})
}

// A silent budget must cost nothing at a give-up: the details are built only
// when an observer is attached, so tracing that is off allocates nothing.
func TestSilentBudgetGiveUpAllocatesNothing(t *testing.T) {
	pkg := buildTestSSA(t, storageGiveUpFixture)
	load := returnedLoad(t, pkg.Func("param"))
	storage := NewStorage(NewSearchBudget(1000))
	allocations := testing.AllocsPerRun(100, func() {
		storage.unknown(EvidenceStorageNotLocal, load)
	})
	if allocations != 0 {
		t.Fatalf("silent give-up allocated %v times per run", allocations)
	}
}

func TestObservedBudgetReportsPositionAndInstruction(t *testing.T) {
	pkg := buildTestSSA(t, storageGiveUpFixture)
	load := returnedLoad(t, pkg.Func("escaped"))
	var at token.Pos
	var details map[string]string
	observer := func(_ string, pos token.Pos, got map[string]string) { at, details = pos, got }
	NewStorage(NewSearchBudget(1000).Observed(observer)).Content(load.X, load)
	if !at.IsValid() || details["instruction"] == "" {
		t.Fatalf("observer got position %v and details %v; want the blocking call", at, details)
	}
}
