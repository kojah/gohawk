package lifecycle

import (
	"reflect"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"golang.org/x/tools/go/ssa"
)

const resultGuardFixture = `
package ssaflowtest

type file struct{}

type failure struct{}

func (*failure) Error() string { return "failed" }

func (*file) Close() {}

func open() (*file, error) { return &file{}, nil }

func closeOnError(fail bool) (err error) {
	f, err := open()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	if fail {
		return &failure{}
	}
	return nil
}

func closeAlways() (err error) {
	f, err := open()
	if err != nil {
		return err
	}
	defer func() { f.Close() }()
	return nil
}
`

func TestResultGuards(t *testing.T) {
	pkg := buildTestSSA(t, resultGuardFixture)
	guarded := pkg.Func("closeOnError")
	target := openedFile(t, guarded)
	request := CompletionRequest{Target: target, Methods: []string{"Close"}}
	guards := ProveResultGuards(guarded, request).Guards
	if len(guards) != 1 || len(guards[0].Cells) != 1 {
		t.Fatalf("ResultGuards(closeOnError) = %+v, want one guard on the err result", guards)
	}
	states := map[string]proofs.EvidenceState{}
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](guarded) {
		if !cfg.InstructionDominates(guards[0].Defer, returned) {
			continue
		}
		value, _ := ssaflow.ValueAtReturnWithin(returned, guards[0].Cells[0], nil)
		states[value.String()] = guards[0].CompletesAtReturn(request, returned, ssacall.ValueOutcome)
	}
	if states["nil:error"] != proofs.EvidenceDisproven {
		t.Errorf("the nil return: %v, want the guarded close skipped (disproven); all: %v", states["nil:error"], states)
	}
	for value, state := range states {
		if value != "nil:error" && state != proofs.EvidenceProven {
			t.Errorf("the %s return: %v, want the guarded close run (proven)", value, state)
		}
	}
	always := pkg.Func("closeAlways")
	if guards := ProveResultGuards(always, CompletionRequest{Target: openedFile(t, always), Methods: []string{"Close"}}).Guards; len(guards) != 0 {
		t.Errorf("ResultGuards(closeAlways) = %+v, want none: its release does not turn on the result", guards)
	}
}

func openedFile(t *testing.T, function *ssa.Function) ssa.Value {
	t.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if extract, ok := instruction.(*ssa.Extract); ok && extract.Index == 0 {
				return extract
			}
		}
	}
	t.Fatalf("%s: no opened file", function.Name())
	return nil
}

const resultGuardBudgetFixture = resultGuardFixture + `
func closeOnSuccess(fail bool)(err error){
 f,err:=open();if err!=nil{return err}
 defer func(){if err==nil{f.Close()}}()
 if fail{return &failure{}};return nil
}
func closeOnBool()(ok bool){
 f,_:=open();defer func(){if ok{f.Close()}}();return true
}
func unrelated(fail bool)(err error){
 f,err:=open();if err!=nil{return err}
 defer func(){if fail{f.Close()}}();return nil
}
func multiple()(err error){
 f,err:=open();if err!=nil{return err}
 defer func(){if err!=nil{f.Close()}}()
 defer func(){if err==nil{f.Close()}}();return nil
}
func wrongCell()(err error){
 f,err:=open();if err!=nil{return err};other:=err
 defer func(){if other!=nil{f.Close()}}();return nil
}
func opaque(cleanup func())(err error){
 f,err:=open();if err!=nil{return err};_=f
 defer func(){if err!=nil{cleanup()}}();return nil
}
`

func TestResultGuardDiscoveryAllowance(t *testing.T) {
	pkg := buildTestSSA(t, resultGuardBudgetFixture)
	for _, test := range []struct {
		name  string
		count int
	}{
		{"closeOnError", 1},
		{"closeOnSuccess", 1},
		{"closeOnBool", 1},
		{"closeAlways", 0},
		{"unrelated", 0},
		{"multiple", 2},
		{"wrongCell", 0},
		{"opaque", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}
			baseline := ProveResultGuards(fn, request)
			if !baseline.Proven() || len(baseline.Guards) != test.count {
				t.Fatalf("default discovery = %+v", baseline)
			}
			for limit := range proofs.SummaryBudget {
				budget := proofs.NewSearchBudget(limit)
				request.Budget = budget
				got := ProveResultGuards(fn, request)
				if budget.Exhausted() || budget.PoolExhausted() || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || got.Guards != nil {
						t.Fatalf("cut %d published guard census: %+v", limit, got)
					}
					continue
				}
				if !reflect.DeepEqual(got, baseline) {
					t.Fatalf("complete discovery = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("discovery never completed")
		})
	}
}

func TestResultGuardDiscoveryChildAndFresh(t *testing.T) {
	fn := buildTestSSA(t, resultGuardBudgetFixture).Func("multiple")
	pool := proofs.NewSearchBudget(10000)
	request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}, Budget: pool.Within(5)}
	if got := ProveResultGuards(fn, request); got.State != proofs.EvidenceUnknown || got.Guards != nil || pool.Exhausted() {
		t.Fatalf("child discovery = %+v, parent exhausted %v", got, pool.Exhausted())
	}
	request.Budget = pool.Within(proofs.SummaryBudget)
	if got := ProveResultGuards(fn, request); !got.Proven() || len(got.Guards) != 2 {
		t.Fatalf("fresh discovery = %+v", got)
	}
}

func TestResultGuardDiscoveryPartialList(t *testing.T) {
	fn := buildTestSSA(t, resultGuardBudgetFixture).Func("multiple")
	for limit := range proofs.SummaryBudget {
		budget := proofs.NewSearchBudget(limit)
		got := ProveResultGuards(fn, CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}, Budget: budget})
		if budget.Exhausted() || budget.PoolExhausted() {
			if len(got.Guards) > 0 {
				t.Fatalf("cut %d published %d partial guards", limit, len(got.Guards))
			}
			continue
		}
		if !got.Proven() || len(got.Guards) != 2 {
			t.Fatalf("complete multi-guard census = %+v", got)
		}
		return
	}
	t.Fatal("multi-guard discovery never completed")
}

const resultReturnBudgetFixture = resultGuardFixture + `
func twoResults(fail bool)(err error,ok bool){
 f,_:=open()
 defer func(){if err!=nil && ok{f.Close()}}()
 if fail{return &failure{},true};return nil,false
}
func earlierResult(fail bool)(err error){
 f,_:=open();defer func(){if err!=nil{f.Close()}}()
 if fail{err=&failure{}};return
}
func overwrittenResult()(err error){
 f,_:=open();defer func(){if err!=nil{f.Close()}}()
 err=&failure{};return nil
}
func conditionalRegistration(yes bool)(err error){
 f,_:=open();if yes{defer func(){if err!=nil{f.Close()}}()};return nil
}
`

func TestResultReturnBindingAllowance(t *testing.T) {
	pkg := buildTestSSA(t, resultReturnBudgetFixture)
	for _, name := range []string{"closeOnError", "twoResults", "earlierResult", "overwrittenResult"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}
			guards := returnBindingGuards(fn, request)
			if len(guards) != 1 {
				t.Fatalf("guards=%+v", guards)
			}
			guard := guards[0]
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			checked := 0
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
				if !cfg.InstructionDominates(guard.Defer, returned) {
					continue
				}
				checked++
				baseline := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome)
				if name == "earlierResult" && baseline != proofs.EvidenceUnknown {
					t.Fatalf("earlier result=%v", baseline)
				}
				if name == "overwrittenResult" && baseline != proofs.EvidenceDisproven {
					t.Fatalf("overwritten result=%v", baseline)
				}
				checkReturnStoresWithin(t, returned, guard.Cells)
				checkReturnCompletionWithin(t, guard, request, returned, baseline)
				request.Budget = nil
			}
			if checked == 0 {
				t.Fatal("no guarded return")
			}
		})
	}
}

func TestResultReturnCallbackChildAndFresh(t *testing.T) {
	fn := buildTestSSA(t, resultReturnBudgetFixture).Func("twoResults")
	request := CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}
	guards := returnBindingGuards(fn, request)
	if len(guards) != 1 {
		t.Fatalf("guards=%+v", guards)
	}
	guard := guards[0]
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
		if !cfg.InstructionDominates(guard.Defer, returned) {
			continue
		}
		pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
		request.Budget = pool.Within(1)
		if got := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome); got != proofs.EvidenceUnknown || pool.Exhausted() {
			t.Fatalf("child=%v pool exhausted=%v", got, pool.Exhausted())
		}
		request.Budget = pool.Within(proofs.SummaryBudget)
		baseline := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome)
		if baseline == proofs.EvidenceUnknown {
			t.Fatal("fresh binding remained unknown")
		}
		request.Budget = pool.Within(proofs.SummaryBudget)
		outcome := func(value ssa.Value) (ssacall.Outcome, bool) {
			for request.Budget.Spend() {
			}
			return ssacall.ValueOutcome(value)
		}
		if got := guard.CompletesAtReturn(request, returned, outcome); got != proofs.EvidenceUnknown || pool.Exhausted() {
			t.Fatalf("callback cut=%v pool exhausted=%v", got, pool.Exhausted())
		}
		request.Budget = pool.Within(proofs.SummaryBudget)
		if got := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome); got != baseline {
			t.Fatalf("fresh=%v want%v", got, baseline)
		}
	}
}

func TestResultGuardReturnReachabilityAllowance(t *testing.T) {
	pkg := buildTestSSA(t, resultReturnBudgetFixture)
	for _, name := range []string{"closeOnError", "conditionalRegistration"} {
		fn := pkg.Func(name)
		guards := ProveResultGuards(fn, CompletionRequest{Target: openedFile(t, fn), Methods: []string{"Close"}}).Guards
		if len(guards) != 1 {
			t.Fatalf("%s guards=%+v", name, guards)
		}
		guard := guards[0]
		for _, returned := range ssaflow.InstructionsOf[*ssa.Return](fn) {
			want := proofs.EvidenceDisproven
			if cfg.InstructionDominates(guard.Defer, returned) {
				want = proofs.EvidenceProven
			} else if cfg.InstructionMayFollow(guard.Defer, returned) {
				want = proofs.EvidenceUnknown
			}
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := guard.ProveReachesReturn(returned, budget)
				if budget.Exhausted() || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				if got.State != want {
					t.Fatalf("reach=%+v want%v", got, want)
				}
				break
			}
		}
	}
}

// Discovery intentionally does not infer a conjunction by varying only one
// result. Construct the two-cell binding question directly, without expanding
// the discovery contract under test elsewhere.
func returnBindingGuards(fn *ssa.Function, request CompletionRequest) []ResultGuard {
	if fn.Name() != "twoResults" {
		return ProveResultGuards(fn, request).Guards
	}
	for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](fn) {
		closure, ok := deferred.Call.Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		guard := ResultGuard{Defer: deferred}
		named := ssaflow.ProveNamedResultCellsWithin(fn, nil)
		for _, binding := range closure.Bindings {
			if cell, ok := binding.(*ssa.Alloc); ok {
				if _, found := named.Cells[cell]; found {
					guard.Cells = append(guard.Cells, cell)
				}
			}
		}
		return []ResultGuard{guard}
	}
	return nil
}

func checkReturnStoresWithin(t *testing.T, returned *ssa.Return, cells []*ssa.Alloc) {
	t.Helper()
	for _, cell := range cells {
		want, found := ssaflow.ValueAtReturnWithin(returned, cell, nil)
		completed := false
		for limit := 0; limit <= proofs.QueryBudget; limit++ {
			budget := proofs.NewSearchBudget(limit)
			got, ok := ssaflow.ValueAtReturnWithin(returned, cell, budget)
			if budget.Exhausted() || limit == 0 {
				if got != nil || ok {
					t.Fatalf("cut%d retained store %v", limit, got)
				}
				continue
			}
			if got != want || ok != found {
				t.Fatalf("store changed: %v/%v want %v/%v", got, ok, want, found)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("store lookup never completed")
		}
	}
}

func checkReturnCompletionWithin(t *testing.T, guard ResultGuard, request CompletionRequest, returned *ssa.Return, want proofs.EvidenceState) {
	t.Helper()
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		request.Budget = proofs.NewSearchBudget(limit)
		got := guard.CompletesAtReturn(request, returned, ssacall.ValueOutcome)
		if request.Budget.Exhausted() || limit == 0 {
			if got != proofs.EvidenceUnknown {
				t.Fatalf("cut%d completion=%v", limit, got)
			}
			continue
		}
		if got != want {
			t.Fatalf("complete=%v want%v", got, want)
		}
		return
	}
	t.Fatal("binding never completed")
}

const returnedCleanupFixture = `package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func (*resource) CloseErr() error { return nil }
func cleanup(r *resource) func() { return func() { r.Close() } }
func forward(r *resource) func() { return cleanup(r) }
func bound(r *resource) func() { return r.Close }
func pair() (*resource, func()) { r := new(resource); return r, func() { r.Close() } }
func pairForward() (*resource, func()) { return pair() }
func wrongPair() (*resource, func()) { r, other := new(resource), new(resource); return r, func() { other.Close() } }
func partial(r *resource, yes bool) func() { if yes { return cleanup(r) }; return func() {} }
func changed(r *resource) func() { f := func() { r.Close() }; r = new(resource); return f }
func async(r *resource) func() { return func() { go r.Close() } }
func recursive(r *resource) func() { return recursive(r) }
func wrap(fn func()) func() { return func() { fn() } }
func wrapErr(fn func() error) func() { return func() { _ = fn() } }
func ignoreErr(_ func() error) func() { return func() {} }
func direct(r *resource) { defer cleanup(r)() }
func forwarded(r *resource) { defer forward(r)() }
func method(r *resource) { defer bound(r)() }
func siblings() { r, fn := pair(); fn(); _ = r }
func siblingsForward() { r, fn := pairForward(); fn(); _ = r }
func wrongSibling() { r, fn := wrongPair(); fn(); _ = r }
func distinct() { r, _ := pair(); _, fn := pair(); fn(); _ = r }
func conditional(r *resource, yes bool) { defer partial(r, yes)() }
func reassigned(r *resource) { defer changed(r)() }
func launched(r *resource) { defer async(r)() }
func recursing(r *resource) { defer recursive(r)() }
func cancellation(fn func()) { defer wrap(fn)() }
func boundError(r *resource) { defer wrapErr(r.CloseErr)() }
func ignoredBoundError(r *resource) { defer ignoreErr(r.CloseErr)() }
`

func TestReturnedCleanupCompletion(t *testing.T) {
	pkg := buildTestSSA(t, returnedCleanupFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"direct", true},
		{"forwarded", true},
		{"method", true},
		{"siblings", true},
		{"siblingsForward", true},
		{"wrongSibling", false},
		{"distinct", false},
		{"conditional", false},
		{"reassigned", false},
		{"launched", false},
		{"recursing", false},
		{"cancellation", true},
		{"boundError", true},
		{"ignoredBoundError", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var target ssa.Value
			if len(function.Params) != 0 {
				target = function.Params[0]
			} else {
				target = ssaflow.InstructionsOf[*ssa.Extract](function)[0]
			}
			var invocation ssa.Instruction
			if deferred := ssaflow.InstructionsOf[*ssa.Defer](function); len(deferred) != 0 {
				invocation = deferred[0]
			} else {
				calls := ssaflow.InstructionsOf[*ssa.Call](function)
				invocation = calls[len(calls)-1]
			}
			request := CompletionRequest{Instruction: invocation, Target: target, Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(2000)}
			switch test.name {
			case "cancellation":
				request.Methods, request.InvokeTarget = nil, true
			case "boundError", "ignoredBoundError":
				request.Methods = []string{"CloseErr"}
			}
			if proof := ProveCompletion(request); proof.Proven() != test.want {
				t.Fatalf("proof = %+v, want proven %v", proof, test.want)
			}
		})
	}
}

func TestReturnedCleanupCacheKeepsFixedPolicy(t *testing.T) {
	pkg := buildTestSSA(t, returnedCleanupFixture)
	factory, caller := pkg.Func("cleanup"), pkg.Func("direct")
	factory.Blocks = nil
	lookups := 0
	lookup := func(function *ssa.Function, method string, invoke bool) []ReturnedCleanupRelation {
		lookups++
		if function == factory && method == "Close" && !invoke {
			return []ReturnedCleanupRelation{{CallbackResult: 0, Target: 0}}
		}
		return nil
	}
	evidence := NewLocalEvidenceWithReturnedCleanup(lookup)
	request := CompletionRequest{
		Instruction: ssaflow.InstructionsOf[*ssa.Defer](caller)[0], Target: caller.Params[0], Methods: []string{"Close"},
		Budget: proofs.NewSearchBudget(1000),
	}
	if proof := evidence.Completion(request); !proof.Proven() {
		t.Fatalf("fixed imported relation unavailable: %+v", proof)
	}
	previous := lookups
	request.Budget = proofs.NewSearchBudget(0)
	if proof := evidence.Completion(request); !proof.Proven() || lookups != previous {
		t.Fatalf("completed proof not cached: %+v, lookups %d -> %d", proof, previous, lookups)
	}
	request.Budget = proofs.NewSearchBudget(1000)
	request.ReturnedSummaries = func(*ssa.Function, string, bool) []ReturnedCleanupRelation { return nil }
	if proof := evidence.Completion(request); proof.Proven() {
		t.Fatalf("request override reused fixed-policy proof: %+v", proof)
	}
	var localOnly LocalEvidence
	request.ReturnedSummaries = nil
	if proof := localOnly.Completion(request); proof.Proven() {
		t.Fatalf("local-only scope reused imported proof: %+v", proof)
	}
}

func TestReturnedResultStorageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
var external int
func cleanup(){}
func deferred()(value int){defer cleanup();return 3}
func opaque() int {return external}
func direct() int {return 4}
`)
	for _, name := range []string{"deferred", "opaque", "direct"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			for _, returned := range ssaflow.InstructionsOf[*ssa.Return](function) {
				if returned.Block().Comment == "recover" {
					continue
				}
				baseline := ReturnedResultWithin(returned, 0, nil)
				if baseline == nil {
					t.Fatal("default result unavailable")
				}
				if name == "opaque" && baseline != returned.Results[0] {
					t.Fatal("opaque storage did not retain original load")
				}
				checkReturnedResultAllowances(t, returned, baseline)
			}
		})
	}
}

func checkReturnedResultAllowances(t *testing.T, returned *ssa.Return, baseline ssa.Value) {
	t.Helper()
	complete := false
	for limit := range proofs.SummaryBudget {
		pool := proofs.NewSearchBudget(proofs.SummaryBudget)
		budget := pool.Within(limit)
		result := ReturnedResultWithin(returned, 0, budget)
		if budget.Exhausted() {
			if result != nil || pool.Exhausted() {
				t.Fatal("local cutoff published result or exhausted pool")
			}
			if fresh := ReturnedResultWithin(returned, 0, pool.Within(proofs.SummaryBudget)); fresh != baseline {
				t.Fatal("fresh query did not recover default result")
			}
			continue
		}
		if result != baseline {
			t.Fatalf("result=%v; want %v", result, baseline)
		}
		complete = true
		break
	}
	if !complete {
		t.Fatal("result query never completed")
	}
}
