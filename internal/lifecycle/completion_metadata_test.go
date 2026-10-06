package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestCallbackEnvironmentMetadata(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
func subject(first, second chan int) { go func(arg chan int) { <-arg; <-first; <-second }(first) }
`)
	spawn := ssaflow.InstructionsOf[*ssa.Go](pkg.Func("subject"))[0]
	function, closure := ssaflow.DirectCallee(spawn.Common())
	outer, captures := &callbackBindings{}, &callbackBindings{}
	callee := completionCallee{
		common: spawn.Common(), function: function, closure: closure,
		environment: captures, invocation: spawn,
	}
	search := newCompletionSearch("", CoverageEveryReturn, proofs.NewSearchBudget(2))
	search.bindings = outer
	key := completionKey{}
	attempts := 0
	query := func() completionAnswer {
		return search.memo.Answer(key, func() completionAnswer {
			attempts++
			return completionAnswer{proven: search.bindCallbackArguments(callee) != nil}
		})
	}
	if answer := query(); answer.proven || !search.budget.Exhausted() || !*search.incomplete {
		t.Fatalf("partial callback environment = %+v, incomplete=%v", answer, *search.incomplete)
	}
	search.budget = proofs.NewSearchBudget(3)
	if answer := query(); !answer.proven || attempts != 2 {
		t.Fatalf("cutoff poisoned memo: answer=%+v attempts=%d", answer, attempts)
	}
	search.budget = proofs.NewSearchBudget(3)
	bindings := search.bindCallbackArguments(callee)
	if bindings == nil || len(bindings.values) != 3 || search.budget.Exhausted() {
		t.Fatal("complete three-binding environment unavailable")
	}
	argument := bindings.values[function.Params[0]]
	if argument.value != spawn.Common().Args[0] || argument.bindings != outer || argument.observation != spawn {
		t.Fatalf("argument environment changed: %+v", argument)
	}
	for index, free := range function.FreeVars {
		binding := bindings.values[free]
		if binding.value != closure.Bindings[index] || binding.bindings != captures || binding.observation != spawn {
			t.Fatalf("capture environment changed: %+v", binding)
		}
	}
}

func TestEnclosingReadOnlyMetadata(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type wrapper struct { value int }
 func helper(first int, w *wrapper, last int) {}
 func subject(w *wrapper) { helper(1,w,2) }
`)
	value := pkg.Func("subject").Params[0]
	pool := proofs.NewSearchBudget(8)
	search := &enclosingSearch{
		request: EnclosingCompletionRequest{Budget: pool.Within(1)},
		memo:    ssaflow.NewCallGraphMemo[*enclosingFrame, bool](),
	}
	if search.readOnly(value) || !search.request.Budget.Exhausted() {
		t.Fatal("partial helper-binding census proved read-only")
	}
	search.request.Budget = pool.Within(4)
	if !search.readOnly(value) || search.request.Budget.Exhausted() {
		t.Fatal("fresh allowance did not recover complete read-only evidence")
	}
	search.request.Budget = proofs.NewSearchBudget(1).Within(4)
	if search.readOnly(value) || !search.request.Budget.PoolExhausted() {
		t.Fatal("shared-pool cutoff proved read-only")
	}
}
