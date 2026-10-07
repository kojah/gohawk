package lifecyclefacts

import (
	"bytes"
	"fmt"
	"go/types"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedCleanupFactsForwardAcrossPackages(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type resource struct{}
func (*resource) Close() {}
func Base(r *resource) func() { return func() { r.Close() } }
func Forward(r *resource) func() { return Base(r) }
func Caller(r *resource) { defer Forward(r)() }
func Wrong(r, other *resource) { defer Forward(other)() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	base := summarize(pass, pkg.Func("Base"))
	if base.ReturnedCleanup == nil || len(base.ReturnedCleanup.Effects) != 1 || base.MethodMask("Close") != 0 {
		t.Fatalf("factory fact: %+v", base)
	}
	baseFunction := pkg.Func("Base")
	baseFunction.Blocks = nil
	pass.ImportObjectFact = func(object types.Object, fact analysis.Fact) bool {
		if object == baseFunction.Object() {
			if target, ok := fact.(*publishedFact); ok {
				*target = *publish(base)
				return true
			}
		}
		return false
	}
	forward := pkg.Func("Forward")
	fact := summarize(pass, forward)
	if fact.ReturnedCleanup == nil || len(fact.ReturnedCleanup.Effects) != 1 || fact.MethodMask("Close") != 0 {
		t.Fatalf("forwarding factory fact: %+v", fact)
	}
	forward.Blocks = nil
	pass.ResultOf = map[*analysis.Analyzer]any{Analyzer: Summaries{forward: fact}}
	for _, name := range []string{"Caller", "Wrong"} {
		function := pkg.Func(name)
		invocation := ssaflow.InstructionsOf[*ssa.Defer](function)[0]
		request := lifecycle.CompletionRequest{
			Instruction: invocation, Target: function.Params[0], Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1000),
		}
		proof := NewLifecycleEvidence(pass, "test", "test").Prove(EvidenceRequest{
			Instruction: invocation, Target: function.Params[0], Completion: &request,
		})
		if proof.Proven() != (name == "Caller") {
			t.Errorf("%s: %+v", name, proof)
		}
	}
}

// An alias changes spelling, not the error contract. Failed construction
// returns may contain errors, while unrelated owner results still prevent
// the body query from promising ownership on every successful return. Tuple
// delegation stays outside this query's result-specific containment proof.
func TestReturnedOwnerErrorAliases(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type resource struct{}
func (*resource) Close() error { return nil }
type owner struct { resource *resource }
type ErrorAlias = error
type ErrorLike interface { Error() string }
func Plain(p *resource, failure error, fail bool) (*owner, error) {
	if fail { return nil, failure }
	return &owner{resource: p}, nil
}
func Alias(p *resource, failure ErrorAlias, fail bool) (*owner, ErrorAlias) {
	if fail { return nil, failure }
	return &owner{resource: p}, nil
}
func DelegatedAlias(p *resource, failure ErrorAlias, fail bool) (*owner, ErrorAlias) {
	return Alias(p, failure, fail)
}
func DelegatedPlain(p *resource, failure error, fail bool) (*owner, error) {
	return Plain(p, failure, fail)
}
func UnrelatedOwner(p *resource, failure ErrorAlias, fail bool) (*owner, ErrorAlias) {
	if fail { return new(owner), failure }
	return &owner{resource: p}, nil
}
func DistinctError(p *resource, failure ErrorLike, fail bool) (*owner, ErrorLike) {
	if fail { return nil, failure }
	return &owner{resource: p}, nil
}
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]bool{
		"Plain":          true,
		"Alias":          true,
		"DelegatedAlias": false,
		"DelegatedPlain": false,
		"UnrelatedOwner": false,
		"DistinctError":  false,
	} {
		function := pkg.Func(name)
		if got := returnedOwnerOnEveryReturn(pass, function, function.Params[0]); got != want {
			t.Errorf("%s: error type %s, returned owner = %t, want %t", name, function.Signature.Results().At(1).Type(), got, want)
		}
	}
}

func TestReturnedViewDeclinesUnavailableReceiverMethod(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type Resource interface { Close() error }
type Owner struct { resource Resource }
func Wrap(resource Resource) *Owner { return &Owner{resource: resource} }
func (*Owner) Close() error
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	function := pkg.Func("Wrap")
	fact := summarize(pass, function)
	if !fact.ReturnedOwner().contains(0) {
		t.Fatal("fixture must prove the parameter is held by the returned owner")
	}
	var dump bytes.Buffer
	if _, err := function.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual SSA:\n%s", dump.String())
	if got := returnedViews(pass, function, fact, Summaries{function: fact}); got != 0 {
		t.Fatalf("unavailable Close summary supplied a positive returned view: %#x", uint64(got))
	}
}

func TestReturnedViewKnownReceiverMethods(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		view bool
	}{
		{"releases", "return owner.resource.Close()", false},
		{"does not release", "return nil", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type Resource interface { Close() error }
type Owner struct { resource Resource }
func Wrap(resource Resource) *Owner { return &Owner{resource: resource} }
func (owner *Owner) Close() error { `+test.body+` }
`)
			pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
			function := pkg.Func("Wrap")
			fact := summarize(pass, function)
			owner := pkg.Type("Owner").Type()
			method := pkg.Prog.LookupMethod(types.NewPointer(owner), pkg.Pkg, "Close")
			summaries := Summaries{function: fact, method: summarize(pass, method)}
			if !fact.ReturnedOwner().contains(0) {
				t.Fatal("fixture must prove returned ownership")
			}
			if got := returnedViews(pass, function, fact, summaries).contains(0); got != test.view {
				t.Fatalf("returned view = %t, want %t", got, test.view)
			}
		})
	}
}

func TestReturnedViewTypeOnlyRuleDoesNotNeedMethodSummary(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type Resource interface { Close() error }
type Reader struct { resource Resource }
func Wrap(resource Resource) *Reader { return &Reader{resource: resource} }
func (*Reader) Read([]byte) (int, error)
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	function := pkg.Func("Wrap")
	fact := summarize(pass, function)
	if !fact.ReturnedOwner().contains(0) {
		t.Fatal("fixture must prove returned ownership")
	}
	if got := returnedViews(pass, function, fact, Summaries{function: fact}); !got.contains(0) {
		t.Fatal("type-only view evidence should not need the unrelated Read summary")
	}
}

func TestReturnedViewVisiblePrivateReceiverMethod(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		view bool
	}{
		{"releases", "return owner.resource.Close()", false},
		{"does not release", "return nil", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type Resource interface { Close() error }
type Owner struct { resource Resource }
func Wrap(resource Resource) *Owner { return &Owner{resource: resource} }
func (*Owner) Close() error { return nil }
func (owner *Owner) helper() error { `+test.body+` }
`)
			pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
			function := pkg.Func("Wrap")
			fact := summarize(pass, function)
			owner := pkg.Type("Owner").Type()
			method := pkg.Prog.LookupMethod(types.NewPointer(owner), pkg.Pkg, "Close")
			summaries := Summaries{function: fact, method: summarize(pass, method)}
			if got := returnedViews(pass, function, fact, summaries).contains(0); got != test.view {
				t.Fatalf("private helper returned view = %t, want %t", got, test.view)
			}
		})
	}
}

func TestPrivateHelperRetentionUsesReturnedWrapperAtEscape(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
import "os"
type holder struct { file *os.File }
var global *holder
var globalHook func()
func wrap(file *os.File) *holder { return &holder{file} }
func install(file *os.File) { global = wrap(file) }
func discard(file *os.File) { _ = wrap(file) }
func returnOnly(file *os.File) *holder { return wrap(file) }
func maybeInstall(file *os.File, fail bool) {
	h := wrap(file)
	if fail { return }
	global = h
}
func unrelated(file *os.File) { global = wrap(nil) }
func recurse(file *os.File) { recurse(file) }
func capture(file *os.File) { globalHook = func() { file.Close() } }
func localMap(file *os.File) { _ = map[int]*os.File{0:file} }
func localSlice(file *os.File) { _ = append([]*os.File{}, file) }
func callerCell(file *os.File, dst **os.File) { *dst = file }
func exercise(file *os.File, fail bool) {
	install(file)
	discard(file)
	_ = returnOnly(file)
	maybeInstall(file, fail)
	unrelated(file)
	recurse(file)
	capture(file)
	localMap(file)
	localSlice(file)
	var dst *os.File
	callerCell(file, &dst)
}
`)
	// wrap stands in for a log.New-style constructor: its result holds the
	// file on every return and it also stores the file into a field.
	parameter := heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter}}
	wrap := Fact{
		Must: MustClaims{ReturnedView: 1},
		Heap: &heapmodel.HeapSummary{
			Version: heapmodel.SummaryVersion,
			Holds:   []heapmodel.HeapHold{{Result: 0, Parameter: 0, Must: true}},
			Effects: []heapmodel.HeapEffect{{Slot: parameter, Escape: heapmodel.HeapEscapedField, Every: true}},
		},
		signature: pkg.Func("wrap").Signature,
	}
	pass := &analysis.Pass{
		ResultOf: map[*analysis.Analyzer]any{Analyzer: Summaries{pkg.Func("wrap"): wrap}},
		ImportObjectFact: func(types.Object, analysis.Fact) bool {
			t.Error("consumer retention must use prerequisite summaries")
			return false
		},
	}
	evidence := NewLifecycleEvidence(pass, "test", "test")
	want := map[string]bool{
		"install": true, "discard": false, "returnOnly": false,
		"maybeInstall": false, "unrelated": false, "recurse": false, "capture": false,
		"localMap": false, "localSlice": false, "callerCell": false,
	}
	function := pkg.Func("exercise")
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		name := call.Common().StaticCallee().Name()
		expected, ok := want[name]
		if !ok {
			continue
		}
		if got := evidence.ArgumentRetainedByCallee(call, function.Params[0]); got != expected {
			t.Errorf("%s retention = %v, want %v", name, got, expected)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing calls: %v", want)
	}
}

func TestReturnedViewBindingAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "viewbinding", `package viewbinding
type holder struct { value *int }
func view(*int) *holder
func aggregate(*holder) *holder
func direct(p, other *int, choose bool) *holder { return view(p) }
func unrelated(p, other *int, choose bool) *holder { return view(other) }
func ambiguous(p, other *int, choose bool) *holder {
 x := p; if choose { x = other }; return view(x)
}
func contained(p, other *int, choose bool) *holder { return aggregate(&holder{p}) }
`)
	fact := Fact{Must: MustClaims{ReturnedView: 1}}
	for _, test := range []struct {
		name string
		want bool
	}{{"direct", true}, {"unrelated", false}, {"ambiguous", false}, {"contained", true}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			if got := fact.ReturnsView(call, fn.Params[0]); got != test.want {
				t.Fatalf("default binding = %v, want %v", got, test.want)
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
				got := fact.ProveReturnsViewWithin(call, fn.Params[0], budget)
				if budget.Exhausted() {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted binding: %+v", limit, got)
					}
					continue
				}
				if got.State == proofs.EvidenceUnknown || got.Proven() != test.want {
					t.Fatalf("complete binding at allowance %d: %+v", limit, got)
				}
				return
			}
			t.Fatal("binding never completed")
		})
	}
}

func TestReturnedViewBindingPreservesStorageCutoff(t *testing.T) {
	var source strings.Builder
	source.WriteString("package viewcap\ntype a *int; type b *int\nfunc view(a) *int\nfunc long(p a) *int {\n")
	previous := "p"
	for index := range proofs.QueryBudget + 10 {
		target := "b"
		if index%2 != 0 {
			target = "a"
		}
		name := fmt.Sprintf("x%d", index)
		fmt.Fprintf(&source, "%s := %s(%s)\n", name, target, previous)
		previous = name
	}
	fmt.Fprintf(&source, "return view(%s)\n}\n", previous)
	pkg := ssaflowtest.BuildPackage(t, "viewcap", source.String())
	fn := pkg.Func("long")
	if count := len(ssaflow.InstructionsOf[*ssa.ChangeType](fn)); count <= proofs.QueryBudget {
		t.Fatalf("fixture has only %d SSA conversions", count)
	}
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	fact := Fact{Must: MustClaims{ReturnedView: 1}}
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	got := fact.ProveReturnsViewWithin(call, fn.Params[0], budget)
	if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || budget.Exhausted() {
		t.Fatalf("local storage cutoff lost availability: %+v, parent exhausted %v", got, budget.Exhausted())
	}
}
