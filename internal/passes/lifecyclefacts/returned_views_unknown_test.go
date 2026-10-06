package lifecyclefacts

import (
	"bytes"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

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
