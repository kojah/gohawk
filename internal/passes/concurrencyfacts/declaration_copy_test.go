package concurrencyfacts

import (
	"go/types"
	"reflect"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func importedDeclarationFunction(t *testing.T) *ssa.Function {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "declarationcopy", `package declarationcopy
 import "sync"
 func root(mu *sync.Mutex){mu.Lock()}`)
	calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("root"))
	if len(calls) != 1 {
		t.Fatalf("calls=%d", len(calls))
	}
	fn := calls[0].Common().StaticCallee()
	if fn == nil || len(fn.Blocks) != 0 || fn.Object() == nil {
		t.Fatalf("not a bodyless imported declaration: %v", fn)
	}
	return fn
}

func declarationCopyFixture(alternatives bool) Fact {
	body := Fact{
		Version:            factVersion,
		Effects:            []Effect{{Kind: ReadLock, Parameter: 0, Fields: []int{0, 1}}, {Kind: Invoke, Parameter: 1, Method: "Complete"}},
		Workers:            []WorkerEffect{{Prefix: 1, Effects: []Effect{{Kind: ReadUnlock, Parameter: 0, Fields: []int{0, 1}}}}},
		CancellationInputs: []int{2},
	}
	if !alternatives {
		return body
	}
	return Fact{Version: factVersion, Alternatives: []FactAlternative{{
		Effects: body.Effects, Workers: body.Workers, CancellationInputs: body.CancellationInputs,
		Conditions: []FactCondition{{Parameter: 3, Constant: FactConstant{Kind: FactConstantBool, Exact: "true"}, Holds: true}},
		Returned:   []FactReturned{{Index: 0, Constant: FactConstant{Kind: FactConstantString, Exact: `"settled"`}}},
	}}}
}

func TestImportedDeclarationMutationCannotChangeCache(t *testing.T) {
	fn := importedDeclarationFunction(t)
	for _, alternatives := range []bool{false, true} {
		name := "linear"
		if alternatives {
			name = "alternatives"
		}
		t.Run(name, func(t *testing.T) {
			engine := NewEngine()
			engine.facts = map[*types.Func]Fact{fn.Object().(*types.Func): declarationCopyFixture(alternatives)}
			got, ok := engine.Declaration(fn, nil)
			if !ok || !reflect.DeepEqual(got, declarationCopyFixture(alternatives)) {
				t.Fatalf("declaration=%+v/%v", got, ok)
			}
			mutateDeclarationCopy(got)
			fresh, ok := engine.Declaration(fn, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
			if !ok || !reflect.DeepEqual(fresh, declarationCopyFixture(alternatives)) {
				t.Fatalf("mutation changed cache: %+v/%v", fresh, ok)
			}
		})
	}
}

func mutateDeclarationCopy(fact Fact) {
	if len(fact.Alternatives) != 0 {
		alternative := &fact.Alternatives[0]
		alternative.Conditions[0].Parameter = -1
		alternative.Conditions[0].Constant.Exact = "false"
		alternative.Returned[0].Index = 1
		alternative.Returned[0].Constant.Exact = `"changed"`
		fact.Effects, fact.Workers, fact.CancellationInputs = alternative.Effects, alternative.Workers, alternative.CancellationInputs
		alternative.Effects = nil
	}
	fact.Effects[0].Fields[0] = 5
	fact.Effects[0].Kind = Unlock
	fact.Effects[1].Method = "Changed"
	fact.Workers[0].Prefix = 0
	fact.Workers[0].Effects[0].Fields[0] = 7
	fact.Workers[0].Effects[0].Parameter = 4
	fact.CancellationInputs[0] = 5
}

func TestImportedDeclarationCutoffDiscardsPartialCopies(t *testing.T) {
	fn := importedDeclarationFunction(t)
	for _, alternatives := range []bool{false, true} {
		want := declarationCopyFixture(alternatives)
		engine := NewEngine()
		engine.facts = map[*types.Func]Fact{fn.Object().(*types.Func): want}
		checkImportedDeclarationCutoffs(t, engine, fn, want)
	}
}

func checkImportedDeclarationCutoffs(t *testing.T, engine *Engine, fn *ssa.Function, want Fact) {
	t.Helper()
	budget := ssaflow.NewSearchBudget(1)
	if got, ok := engine.Declaration(fn, budget); ok || !budget.Exhausted() || !reflect.DeepEqual(got, Fact{}) {
		t.Fatalf("lookup-only allowance retained%+v/%v", got, ok)
	}
	for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
		pool := ssaflow.NewSearchBudget(2 * ssaflow.SummaryBudget)
		child := pool.Within(limit)
		got, ok := engine.Declaration(fn, child)
		if child.Exhausted() {
			if ok || !reflect.DeepEqual(got, Fact{}) || pool.Exhausted() {
				t.Fatalf("cut%d retained%+v/%v", limit, got, ok)
			}
			fresh, ok := engine.Declaration(fn, pool.Within(ssaflow.SummaryBudget))
			if !ok || !reflect.DeepEqual(fresh, want) {
				t.Fatalf("fresh after cut%d=%+v/%v", limit, fresh, ok)
			}
			continue
		}
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("complete%d=%+v/%v", limit, got, ok)
		}
		return
	}
	t.Fatal("copy never completed")
}

func TestImportedDeclarationEmptyAndMissingRemainDistinct(t *testing.T) {
	fn := importedDeclarationFunction(t)
	facts := []Fact{
		{Version: factVersion},
		{Version: factVersion, Effects: []Effect{}, Workers: []WorkerEffect{}, CancellationInputs: []int{}, Alternatives: []FactAlternative{}},
	}
	for _, want := range facts {
		engine := NewEngine()
		engine.facts = map[*types.Func]Fact{fn.Object().(*types.Func): want}
		got, ok := engine.Declaration(fn, ssaflow.NewSearchBudget(1))
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("empty declaration=%+v/%v want%+v", got, ok, want)
		}
	}
	engine := NewEngine()
	if got, ok := engine.Declaration(fn, nil); ok || !reflect.DeepEqual(got, Fact{}) {
		t.Fatalf("missing fact=%+v/%v", got, ok)
	}
	engine.facts = map[*types.Func]Fact{fn.Object().(*types.Func): {Version: factVersion - 1}}
	if got, ok := engine.Declaration(fn, nil); ok || !reflect.DeepEqual(got, Fact{}) {
		t.Fatalf("incompatible fact=%+v/%v", got, ok)
	}
}

func TestImportedDeclarationFieldCopySharesAllowance(t *testing.T) {
	fn := importedDeclarationFunction(t)
	// Lookup and the effect itself fit; the nested field still requires a step.
	want := Fact{Version: factVersion, Effects: []Effect{{Kind: Lock, Parameter: 0, Fields: []int{0}}}}
	engine := NewEngine()
	engine.facts = map[*types.Func]Fact{fn.Object().(*types.Func): want}
	budget := ssaflow.NewSearchBudget(2)
	if got, ok := engine.Declaration(fn, budget); ok || !budget.Exhausted() || !reflect.DeepEqual(got, Fact{}) {
		t.Fatalf("nested field retained%+v/%v", got, ok)
	}
	fresh, ok := engine.Declaration(fn, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if !ok || !reflect.DeepEqual(fresh, want) {
		t.Fatalf("fresh field copy=%+v/%v", fresh, ok)
	}
}

// Exercise the same boundary with a real exported/imported path fact before
// its caller summary is cached, so mutation cannot hide behind a warm summary.
func assertImportedAlternativeCopy(t *testing.T, engine *Engine, caller *ssa.Function) {
	t.Helper()
	target := ssaflow.InstructionsOf[*ssa.Call](caller)[0].Common().StaticCallee()
	fact, ok := engine.Declaration(target, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if !ok || len(fact.Alternatives) != 2 || len(fact.Alternatives[0].Effects) != 1 || len(fact.Alternatives[0].Conditions) != 1 {
		t.Fatalf("imported Pick declaration=%+v/%v", fact, ok)
	}
	effect := fact.Alternatives[0].Effects[0]
	condition := fact.Alternatives[0].Conditions[0]
	fact.Alternatives[0].Effects[0].Parameter = -1
	fact.Alternatives[0].Conditions[0].Holds = !condition.Holds
	fresh, ok := engine.Declaration(target, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if !ok || fresh.Alternatives[0].Effects[0].Parameter != effect.Parameter || fresh.Alternatives[0].Conditions[0] != condition {
		t.Fatalf("real imported mutation changed cache: %+v/%v", fresh, ok)
	}
}
