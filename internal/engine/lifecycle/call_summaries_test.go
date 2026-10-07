package lifecycle_test

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const summaryFixture = `package summaries
func leaf() {}
func left() { leaf() }
func right() { leaf() }
func diamond() { left(); right() }
func recursiveA() { recursiveB() }
func recursiveB() { recursiveA(); leaf() }
func opaque()
func effect(ch chan int) { close(ch) }
func calls(a, b chan int) { effect(a); effect(b) }
func closure(ch chan int) { func() { close(ch) }() }
func dynamic(fn func()) { fn() }
`

// These counts are symbolic, additive summaries. Tests deliberately use a
// different summary shape from the channel protocol's ordered event sequence.
type countedSummary struct {
	count  int
	known  bool
	reason ssacall.SummaryUnavailable
}

func countSummaries(visits map[*ssa.Function]int) *ssacall.FunctionSummaries[countedSummary] {
	var summaries *ssacall.FunctionSummaries[countedSummary]
	summaries = ssacall.NewFunctionSummaries(func(function *ssa.Function, budget *proofs.SearchBudget) countedSummary {
		visits[function]++
		result := countedSummary{known: true, count: 1}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if !budget.Spend() {
					return result // Infrastructure must discard this partial answer.
				}
				if call, ok := instruction.(*ssa.Call); ok {
					nested := summaries.Function(call.Common().StaticCallee(), budget)
					if !nested.known {
						return nested
					}
					result.count += nested.count
				}
			}
		}
		return result
	}, func(reason ssacall.SummaryUnavailable) countedSummary {
		return countedSummary{reason: reason}
	})
	return summaries
}

func TestFunctionSummariesReuseAndIsolation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	visits := map[*ssa.Function]int{}
	summaries := countSummaries(visits)
	root := pkg.Func("diamond")
	if got := summaries.Function(root, proofs.NewSearchBudget(100)); !got.known || got.count != 5 {
		t.Fatalf("diamond summary = %+v, want known count 5", got)
	}
	for _, name := range []string{"diamond", "left", "right", "leaf"} {
		if visits[pkg.Func(name)] != 1 {
			t.Errorf("%s computed %d times, want 1", name, visits[pkg.Func(name)])
		}
	}
	if got := summaries.Function(root, proofs.NewSearchBudget(0)); !got.known || got.count != 5 {
		t.Errorf("cached answer requires no new traversal: %+v", got)
	}
	other := countSummaries(map[*ssa.Function]int{})
	if got := other.Function(root, proofs.NewSearchBudget(0)); got.known || got.reason != ssacall.SummaryBudgetExhausted {
		t.Errorf("separate engine reused another policy's answer: %+v", got)
	}
}

func TestFunctionSummariesUnavailable(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	for _, test := range []struct {
		name   string
		limit  int
		reason ssacall.SummaryUnavailable
	}{
		{"missing", 100, ssacall.SummaryBodyUnavailable},
		{"opaque", 100, ssacall.SummaryBodyUnavailable},
		{"recursiveA", 100, ssacall.SummaryRecursive},
		{"recursiveB", 100, ssacall.SummaryRecursive},
		{"diamond", 2, ssacall.SummaryBudgetExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			visits := map[*ssa.Function]int{}
			summaries := countSummaries(visits)
			function := pkg.Func(test.name)
			for range 2 {
				if got := summaries.Function(function, proofs.NewSearchBudget(test.limit)); got.known || got.reason != test.reason || got.count != 0 {
					t.Errorf("summary = %+v, want unavailable reason %v with no partial count", got, test.reason)
				}
			}
			if function != nil && len(function.Blocks) > 0 && visits[function] != 2 {
				t.Errorf("shortened answer cached: computations = %d, want 2", visits[function])
			}
			if test.name == "diamond" {
				if got := summaries.Function(function, proofs.NewSearchBudget(100)); !got.known || got.count != 5 {
					t.Errorf("fresh budget failed to recover: %+v", got)
				}
			}
		})
	}
}

func TestFunctionSummariesCallBindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	summaries := ssacall.NewFunctionSummaries(func(function *ssa.Function, _ *proofs.SearchBudget) []ssa.Value {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if call, ok := instruction.(*ssa.Call); ok {
					return []ssa.Value{call.Common().Args[0]}
				}
			}
		}
		return nil
	}, func(ssacall.SummaryUnavailable) []ssa.Value { return nil })
	caller := pkg.Func("calls")
	for index, call := range ssaflow.InstructionsOf[*ssa.Call](caller) {
		got := summaries.AtCall(call, proofs.NewSearchBudget(10), func(symbolic []ssa.Value, bindings []ssacall.CallBinding) []ssa.Value {
			if len(symbolic) != 1 || len(bindings) != 1 || symbolic[0] != bindings[0].Local || bindings[0].Captured {
				t.Fatalf("unexpected direct bindings: %+v for %v", bindings, symbolic)
			}
			return []ssa.Value{bindings[0].Supplied}
		})
		if len(got) != 1 || got[0] != caller.Params[index] {
			t.Errorf("call %d binding = %v, want parameter %v", index, got, caller.Params[index])
		}
	}
	for _, name := range []string{"closure", "dynamic"} {
		call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name))[0]
		bound := false
		summaries.AtCall(call, proofs.NewSearchBudget(10), func(_ []ssa.Value, bindings []ssacall.CallBinding) []ssa.Value {
			bound = true
			if len(bindings) != 1 || !bindings[0].Captured {
				t.Errorf("closure bindings = %+v", bindings)
			}
			return nil
		})
		if bound != (name == "closure") {
			t.Errorf("%s binding called = %v", name, bound)
		}
	}
}

func TestFunctionSummariesBindingBudgetDoesNotPoisonCallee(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	summaries := countSummaries(map[*ssa.Function]int{})
	callee := pkg.Func("leaf")
	if got := summaries.Function(callee, proofs.NewSearchBudget(10)); !got.known {
		t.Fatal("failed to warm callee summary")
	}
	call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("left"))[0]
	budget := proofs.NewSearchBudget(0)
	got := summaries.AtCall(call, budget, func(answer countedSummary, _ []ssacall.CallBinding) countedSummary {
		budget.Spend()
		return answer // Even a successful answer cannot escape an exhausted binding.
	})
	if got.known || got.reason != ssacall.SummaryBudgetExhausted || got.count != 0 {
		t.Errorf("partial bound answer escaped: %+v", got)
	}
	if cached := summaries.Function(callee, proofs.NewSearchBudget(0)); !cached.known || cached.count != 1 {
		t.Errorf("call-local binding failure poisoned symbolic callee: %+v", cached)
	}
}

func TestFunctionSummariesAlreadyExhaustedBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	summaries := countSummaries(map[*ssa.Function]int{})
	function := pkg.Func("leaf")
	if got := summaries.Function(function, nil); !got.known {
		t.Fatal("unbounded query failed")
	}
	budget := proofs.NewSearchBudget(0)
	budget.Spend()
	if got := summaries.Function(function, budget); got.known || got.reason != ssacall.SummaryBudgetExhausted {
		t.Errorf("cached evidence escaped an already exhausted query: %+v", got)
	}
	if got := summaries.Function(function, proofs.NewSearchBudget(0)); !got.known {
		t.Errorf("fresh query lost the completed summary: %+v", got)
	}
}

// These contracts cover composition boundaries, not analyzer policy. In
// particular a recursive cut may preserve independent positive witnesses;
// it must not make that path-dependent answer reusable as a complete summary.
func TestSummaryContractNestedCutsInvalidateParents(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", `package contracts
func root() { child() }
func child() { root() }
func sibling() {}
`)
	for _, cut := range []string{"recursion", "budget"} {
		t.Run(cut, func(t *testing.T) {
			memo := ssacall.NewCallGraphMemo[*ssa.Function, int]()
			root, child, sibling := pkg.Func("root"), pkg.Func("child"), pkg.Func("sibling")
			budget := proofs.NewSearchBudget(0)
			if cut == "recursion" {
				budget = proofs.NewSearchBudget(10)
			}
			unavailable := func(ssacall.SummaryUnavailable, int) int { return -1 }
			got := memo.Summarize(root, root, budget, func() int {
				memo.Summarize(sibling, sibling, budget, func() int { return 7 }, unavailable)
				return memo.Summarize(child, child, budget, func() int {
					if cut == "budget" {
						budget.Spend()
						return 1
					}
					memo.Summarize(root, root, budget, func() int {
						t.Error("recursive body executed")
						return 99
					}, unavailable)
					return 1 // An independent witness survives, but is not cacheable.
				}, unavailable)
			}, unavailable)
			if (cut == "budget" && got != -1) || (cut == "recursion" && got != 1) {
				t.Fatalf("cut result = %d", got)
			}
			for _, function := range []*ssa.Function{root, child} {
				got := memo.Summarize(function, function, proofs.NewSearchBudget(10), func() int { return 42 }, unavailable)
				if got != 42 {
					t.Errorf("%s retained shortened answer %d", function.Name(), got)
				}
			}
			got = memo.Summarize(sibling, sibling, proofs.NewSearchBudget(0), func() int {
				t.Error("unaffected sibling was unnecessarily recomputed")
				return 99
			}, unavailable)
			if got != 7 {
				t.Errorf("independent completed sibling = %d, want 7", got)
			}
		})
	}
}

func TestSummaryContractBindingOwnsItsResult(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", `package contracts
func helper(ch chan int) { close(ch) }
func caller(a, b chan int) { helper(a); helper(b) }
`)
	callee, caller := pkg.Func("helper"), pkg.Func("caller")
	computations := 0
	summaries := ssacall.NewFunctionSummaries(func(fn *ssa.Function, budget *proofs.SearchBudget) []ssa.Value {
		computations++
		budget.Spend()
		return []ssa.Value{fn.Params[0]}
	}, func(ssacall.SummaryUnavailable) []ssa.Value { return nil })
	for index, call := range ssaflow.InstructionsOf[*ssa.Call](caller) {
		got := summaries.AtCall(call, proofs.NewSearchBudget(10), func(symbolic []ssa.Value, bindings []ssacall.CallBinding) []ssa.Value {
			// Generic summary values can contain arbitrary maps and pointers.
			// Ownership is the adapter's contract, not an implicit deep copy.
			bound := slices.Clone(symbolic)
			bound[0] = bindings[0].Supplied
			return bound
		})
		if len(got) != 1 || got[0] != caller.Params[index] {
			t.Fatalf("invocation %d bound to %v", index, got)
		}
		got[0] = nil
		if symbolic := summaries.Function(callee, proofs.NewSearchBudget(0)); symbolic[0] != callee.Params[0] {
			t.Fatal("call-site mutation changed cached symbolic evidence")
		}
	}
	if computations != 1 {
		t.Errorf("callee computed %d times, want 1", computations)
	}
}

func TestSummaryContractPolicyIsolation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", "package contracts; func helper() {}")
	for _, policyAnswer := range []int{7, 11} {
		summaries := ssacall.NewFunctionSummaries(func(_ *ssa.Function, budget *proofs.SearchBudget) int {
			budget.Spend()
			return policyAnswer
		}, func(ssacall.SummaryUnavailable) int { return -1 })
		for _, limit := range []int{1, 0} {
			if got := summaries.Function(pkg.Func("helper"), proofs.NewSearchBudget(limit)); got != policyAnswer {
				t.Errorf("policy %d got %d", policyAnswer, got)
			}
		}
	}
}

func TestSummaryContractBindingCutInvalidatesCaller(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contracts", `package contracts
func helper() {}
func caller() { helper() }
`)
	caller, callee := pkg.Func("caller"), pkg.Func("helper")
	call := ssaflow.InstructionsOf[*ssa.Call](caller)[0]
	visits := map[*ssa.Function]int{}
	var summaries *ssacall.FunctionSummaries[int]
	summaries = ssacall.NewFunctionSummaries(func(fn *ssa.Function, budget *proofs.SearchBudget) int {
		visits[fn]++
		budget.Spend()
		if fn == callee {
			return 7
		}
		return summaries.AtCall(call, budget, func(answer int, _ []ssacall.CallBinding) int {
			budget.Spend()
			return answer
		})
	}, func(ssacall.SummaryUnavailable) int { return -1 })
	if got := summaries.Function(caller, proofs.NewSearchBudget(2)); got != -1 {
		t.Fatalf("binding-exhausted caller = %d, want unavailable", got)
	}
	if got := summaries.Function(caller, proofs.NewSearchBudget(10)); got != 7 {
		t.Errorf("fresh caller query = %d, want 7", got)
	}
	if visits[caller] != 2 || visits[callee] != 1 {
		t.Errorf("computations caller=%d callee=%d, want 2 and 1", visits[caller], visits[callee])
	}
}

func TestSummaryContextKeys(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	function := pkg.Func("calls")
	type key struct {
		function *ssa.Function
		target   ssa.Value
		strict   bool
	}
	memo := ssacall.NewCallGraphMemo[key, int]()
	for index, target := range function.Params {
		for _, strict := range []bool{false, true} {
			context := key{function: function, target: target, strict: strict}
			want := index + 1
			if strict {
				want += 10
			}
			for attempt := range 2 {
				got := memo.Summarize(context, function, nil, func() int {
					if attempt != 0 {
						t.Error("completed context was not memoized")
					}
					return want
				}, func(ssacall.SummaryUnavailable, int) int { return -1 })
				if got != want {
					t.Errorf("parameter %d strict=%v: got %d, want %d", index, strict, got, want)
				}
			}
		}
	}
}

func TestSummaryBudgetPreservesOnlyMarkedPartialEvidence(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	function := pkg.Func("leaf")
	type evidence struct {
		witness int
		unknown bool
	}
	memo := ssacall.NewCallGraphMemo[*ssa.Function, evidence]()
	computations := 0
	for _, limit := range []int{0, 1, 0} {
		budget := proofs.NewSearchBudget(limit)
		got := memo.Summarize(function, function, budget, func() evidence {
			computations++
			budget.Spend()
			return evidence{witness: 1}
		}, func(_ ssacall.SummaryUnavailable, partial evidence) evidence {
			partial.unknown = true
			return partial
		})
		wantUnknown := computations == 1
		if got.witness != 1 || got.unknown != wantUnknown {
			t.Errorf("limit %d: got %+v, want witness with unknown=%v", limit, got, wantUnknown)
		}
	}
	if computations != 2 {
		t.Errorf("computations = %d, want 2 (partial discarded, complete retained)", computations)
	}
}

func TestSummaryRecursionGuardSpansContexts(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "summaries", summaryFixture)
	function := pkg.Func("leaf")
	memo := ssacall.NewCallGraphMemo[int, bool]()
	failed := func(reason ssacall.SummaryUnavailable, _ bool) bool {
		if reason != ssacall.SummaryRecursive {
			t.Errorf("unexpected cut reason: %v", reason)
		}
		return false
	}
	got := memo.Summarize(1, function, nil, func() bool {
		return memo.Summarize(2, function, nil, func() bool {
			t.Error("changing context bypassed the function recursion guard")
			return true
		}, failed)
	}, failed)
	if got {
		t.Fatal("recursive query produced a proof")
	}
	for _, context := range []int{1, 2} {
		if !memo.Summarize(context, function, nil, func() bool { return true }, failed) {
			t.Errorf("context %d retained a path-dependent cut", context)
		}
	}
}

const compositionScopeFixture = `package scopes
func first() {}
func second() {}
func opaque()
`

func TestSummaryCompositionVisitsIndependentBodies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "scopes", compositionScopeFixture)
	memo := ssacall.NewCallGraphMemo[string, int]()
	visits := 0
	compute := func() int {
		for _, name := range []string{"first", "second", "first"} {
			if !memo.WithFunction(pkg.Func(name), func() { visits++ }) {
				t.Errorf("sequential body %s rejected; prior scope was not released", name)
			}
		}
		return visits
	}
	for range 2 {
		got := memo.Compose("call-site question", proofs.NewSearchBudget(10), compute,
			func(ssacall.SummaryUnavailable, int) int { return -1 })
		if got != 3 || visits != 3 {
			t.Errorf("answer=%d visits=%d, want 3 and 3", got, visits)
		}
	}
	for _, name := range []string{"missing", "opaque"} {
		if memo.WithFunction(pkg.Func(name), func() { t.Error("opaque body visited") }) {
			t.Errorf("%s reported a visited body", name)
		}
	}
}

func TestSummaryCompositionRecursiveAlternativeInvalidatesQuestion(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "scopes", compositionScopeFixture)
	memo := ssacall.NewCallGraphMemo[string, int]()
	unavailable := func(ssacall.SummaryUnavailable, int) int { return -1 }
	visits := 0
	budget := proofs.NewSearchBudget(10)
	for range 2 {
		got := memo.Compose("multiple callees", budget, func() int {
			memo.WithFunction(pkg.Func("first"), func() {
				visits++
				memo.Compose("nested question", budget, func() int {
					if memo.WithFunction(pkg.Func("first"), func() { t.Error("recursive body visited") }) {
						t.Error("recursive alternative accepted")
					}
					return 5 // Positive witness, not an exhaustive summary.
				}, unavailable)
			})
			return 5
		}, unavailable)
		if got != 5 {
			t.Errorf("independent witness lost: %d", got)
		}
	}
	if visits != 2 {
		t.Errorf("path-dependent answer cached: visits=%d", visits)
	}
	if got := memo.Compose("nested question", budget, func() int { return 9 }, unavailable); got != 9 {
		t.Errorf("nested cut poisoned its cache: %d", got)
	}
}

func TestSummaryCompositionPolicyTruncationInvalidatesParents(t *testing.T) {
	memo := ssacall.NewCallGraphMemo[int, int]()
	unavailable := func(ssacall.SummaryUnavailable, int) int { return -1 }
	budget := proofs.NewSearchBudget(10)
	memo.Compose(1, budget, func() int {
		return memo.Compose(2, budget, func() int {
			memo.Incomplete()
			return 7
		}, unavailable)
	}, unavailable)
	for _, key := range []int{1, 2} {
		if got := memo.Compose(key, budget, func() int { return 9 }, unavailable); got != 9 {
			t.Errorf("question %d retained incomplete answer %d", key, got)
		}
	}
}
