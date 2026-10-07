package lifecyclefacts

import (
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The exported heap projection carries the claims the masks make, in one
// encoding: a constructor's result holding its parameter is an edge from
// the result to the parameter, a release on every return is a release
// effect at the parameter's path, and a parameter stored into a global is
// an escape effect.
func TestLifecycleSummaryHeapProjection(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{ id int }

func (c *closer) Close() error { return nil }

type owner struct{ body *closer }
type pair struct{ first, second *closer }

var saved *closer

func Wrap(c *closer) *owner        { return &owner{body: c} }
func CloseSecond(p *pair) error    { return p.second.Close() }
func Keep(c *closer)               { saved = c }
func Inspect(p *pair) bool         { return p.first != nil }

type sink interface{ Log(string) }

func SetThenLog(o *owner, c *closer, s sink) { o.body = c; s.Log("set") }
func UseAfterLog(c *closer, s sink) *closer  { o := &owner{}; SetThenLog(o, c, s); return o.body }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string][]string{
		"Wrap":        {"edge R0 -> fresh(new#", "edge R0/field:0 -> P0 must"},
		"CloseSecond": {"effect P0/field:1 released Close every", "read P0/field:1"},
		"Keep":        {"edge G:example.com/lifecyclefactstest.saved -> P0 must", "effect P0 escaped global every"},
		"Inspect":     {"read P0/field:0"},
		// The interface call can reach only what it was handed: the sink
		// is truncated, the owner and the closer keep their edge.
		"SetThenLog":  {"edge P0/field:0 -> P1 must", "truncated P2"},
		"UseAfterLog": {"holds R0 P0 must"},
	} {
		fact := summarize(pass, pkg.Func(name))
		if fact.Heap == nil {
			t.Fatalf("%s: no heap projection", name)
		}
		rendered := fact.Heap.String()
		for _, line := range want {
			if !strings.Contains(rendered, line) {
				t.Errorf("%s projection lacks %q:\n%s", name, line, rendered)
			}
		}
		if name == "SetThenLog" && (strings.Contains(rendered, "truncated P0") || strings.Contains(rendered, "truncated P1")) {
			t.Errorf("%s truncates a parameter the interface call could not reach:\n%s", name, rendered)
		}
	}
	// Inspect reads and dereferences its parameter and nothing more: the
	// projection carries the read and the non-nil requirement, no edge, no
	// effect, and no cut.
	if fact := summarize(pass, pkg.Func("Inspect")); len(fact.Heap.Edges) != 0 || len(fact.Heap.Effects) != 0 || len(fact.Heap.Truncated) != 0 {
		t.Errorf("Inspect should claim nothing beyond its reads, got heap:\n%s", fact.Heap.String())
	}
}

func TestAsynchronousExposureClaim(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
type writer interface{ Write([]byte) (int, error) }
type holder struct{ destination writer }
func Async(w writer) { go w.Write(nil) }
func Borrow(w writer) { _, _ = w.Write(nil) }
func Other(w, other writer) { _, _ = w.Write(nil); go other.Write(nil) }
func Nested(h *holder) { go h.destination.Write(nil) }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]ParameterMask{
		"Async": parameterMaskFor(0), "Borrow": 0, "Other": parameterMaskFor(1), "Nested": 0,
	} {
		fact := summarize(pass, pkg.Func(name))
		if got := fact.Claim(ClaimAsynchronouslyExposes); got != want {
			t.Errorf("%s asynchronous parameters=%b, want %b; heap:\n%s", name, got, want, fact.Heap.String())
		}
	}
	if mask := (&Fact{}).Claim(ClaimAsynchronouslyExposes); mask != 0 {
		t.Errorf("unavailable heap claims asynchronous exposure: %b", mask)
	}
}

// The transfer and retention claims are read from the heap projection, and
// the discharges are mirrored into it as release effects. This table pins,
// function by function, that each claim the fact answers is also visible in
// the raw projection a caller's graph applies.
func TestLifecycleSummaryClaimsDerivableFromHeap(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{ id int }

func (c *closer) Close() error { return nil }

type owner struct{ body *closer }
type pair struct{ first, second *closer }
type sink interface{ Add(any) }

var saved *closer
var registry sink

func Wrap(c *closer) *owner                { return &owner{body: c} }
func WrapMaybe(c *closer, ok bool) *owner  { if ok { return &owner{body: c} }; return nil }
func Adopt(o *owner, c *closer)            { o.body = c }
func Keep(c *closer)                       { saved = c }
func Publish(c *closer)                    { registry.Add(c) }
func KeepFirst(p *pair)                    { saved = p.first }
func CloseIt(c *closer) error              { return c.Close() }
func CloseSecond(p *pair) error            { return p.second.Close() }
func Inspect(p *pair) bool                 { return p.first != nil }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	type claim struct {
		name    string
		derived bool
	}
	for name, want := range map[string][]claim{
		"Wrap":        {{"returned-owner P0", true}},
		"WrapMaybe":   {{"returned-owner P0", true}},
		"Adopt":       {{"receiver-store P1", true}},
		"Keep":        {{"retained P0", true}, {"stored P0", true}},
		"Publish":     {{"retained P0", true}},
		"KeepFirst":   {{"kept P0/field:0", true}},
		"CloseIt":     {{"released P0 Close", true}},
		"CloseSecond": {{"released P0/field:1 Close", true}},
		"Inspect":     {{"nothing", true}},
	} {
		fact := summarize(pass, pkg.Func(name))
		if fact.Heap == nil {
			t.Fatalf("%s: no projection", name)
		}
		for _, expected := range want {
			derived := deriveClaim(fact, expected.name)
			claimed := factClaims(fact, expected.name)
			if derived != expected.derived {
				t.Errorf("%s: claim %q derived from projection = %t, want %t\n%s", name, expected.name, derived, expected.derived, fact.Heap.String())
			}
			if expected.name != "nothing" && !claimed {
				t.Errorf("%s: the mask does not make claim %q", name, expected.name)
			}
		}
	}
}

// deriveClaim answers a mask claim as a query over the projection.
func deriveClaim(fact Fact, name string) bool {
	query, ok := heapQueries[name]
	return ok && query(fact.Heap)
}

func parameterSlot(index int, path string) heapmodel.HeapSlot {
	return heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index}, Path: path}
}

func edgeTo(heap *heapmodel.HeapSummary, slot heapmodel.HeapSlot, fromKind heapmodel.HeapRootKind, must bool) bool {
	for _, edge := range heap.Edges {
		if edge.From.Root.Kind == fromKind && edge.To.Kind == heapmodel.HeapTargetSlot && edge.To.Slot == slot && (edge.Must || !must) {
			return true
		}
	}
	return false
}

func escaped(heap *heapmodel.HeapSummary, slot heapmodel.HeapSlot, kinds heapmodel.HeapEscape) bool {
	for _, effect := range heap.Effects {
		if effect.Slot == slot && effect.Escape&kinds != 0 {
			return true
		}
	}
	return false
}

func released(heap *heapmodel.HeapSummary, slot heapmodel.HeapSlot, method string) bool {
	for _, effect := range heap.Effects {
		if effect.Slot == slot && effect.Release == method && effect.Every {
			return true
		}
	}
	return false
}

// heapQueries expresses each mask claim over the projection.
var heapQueries = map[string]func(*heapmodel.HeapSummary) bool{
	"returned-owner P0": func(heap *heapmodel.HeapSummary) bool {
		for _, hold := range heap.Holds {
			if hold.Parameter == 0 && hold.Must {
				return true
			}
		}
		return false
	},
	"receiver-store P1": func(heap *heapmodel.HeapSummary) bool {
		return edgeTo(heap, parameterSlot(1, ""), heapmodel.HeapParameter, true)
	},
	"retained P0": func(heap *heapmodel.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, ""), ^heapmodel.HeapEscape(0)) ||
			edgeTo(heap, parameterSlot(0, ""), heapmodel.HeapGlobal, false) ||
			edgeTo(heap, parameterSlot(0, ""), heapmodel.HeapResult, false)
	},
	"stored P0": func(heap *heapmodel.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, ""), heapmodel.HeapEscapedGlobal|heapmodel.HeapEscapedField)
	},
	"kept P0/field:0": func(heap *heapmodel.HeapSummary) bool {
		return escaped(heap, parameterSlot(0, "field:0"), ^heapmodel.HeapEscape(0)) ||
			edgeTo(heap, parameterSlot(0, "field:0"), heapmodel.HeapGlobal, false)
	},
	"released P0 Close":         func(heap *heapmodel.HeapSummary) bool { return released(heap, parameterSlot(0, ""), "Close") },
	"released P0/field:1 Close": func(heap *heapmodel.HeapSummary) bool { return released(heap, parameterSlot(0, "field:1"), "Close") },
	"nothing": func(heap *heapmodel.HeapSummary) bool {
		return len(heap.Edges) == 0 && len(heap.Effects) == 0 && len(heap.Truncated) == 0
	},
}

// factClaims reports whether the existing masks make the claim.
func factClaims(fact Fact, name string) bool {
	switch name {
	case "returned-owner P0":
		return fact.ReturnedOwner().contains(0)
	case "receiver-store P1":
		return fact.ReceiverStore().contains(1)
	case "retained P0":
		return fact.Retained().contains(0)
	case "stored P0":
		return fact.Stored().contains(0)
	case "kept P0/field:0":
		for _, kept := range fact.Kept() {
			if kept.Parameter == 0 && kept.Path == "field:0" {
				return true
			}
		}
		return false
	case "released P0 Close":
		return fact.MethodMask("Close").contains(0)
	case "released P0/field:1 Close":
		for _, discharge := range fact.Discharges {
			if discharge.Parameter == 0 && discharge.Method == "Close" && discharge.Path == "field:1" {
				return true
			}
		}
		return false
	}
	return false
}

// A requirement is a method the function calls on the object at a named
// slot on every normal return, directly or through a summarized callee.
// The accepted forms pin the boundary: a call on some path only, a call on
// an object the function made itself, and a receiver the graph cannot
// resolve to one object require nothing.
func TestLifecycleSummaryRequirements(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type reader struct{ id int }

func (r *reader) Read(p []byte) (int, error) { return 0, nil }
func (r *reader) Err() error                 { return nil }

type holder struct{ r *reader }

func open() *reader { return &reader{} }

func Drain(r *reader) error                   { _, err := r.Read(nil); return err }
func Sometimes(r *reader, ok bool) error      { if ok { _, err := r.Read(nil); return err }; return nil }
func Twice(r *reader) error                   { _, _ = r.Read(nil); return r.Err() }
func ViaHelper(r *reader) error               { return Drain(r) }
func ViaSometimes(r *reader, ok bool) error   { return Sometimes(r, ok) }
func Replaced(r *reader) error                { r = open(); _, err := r.Read(nil); return err }
func Held(h holder) error                     { _, err := h.r.Read(nil); return err }
func Either(a, b *reader, ok bool) error      { r := a; if ok { r = b }; _, err := r.Read(nil); return err }
func Panics(r *reader) error                  { _, _ = r.Read(nil); panic("never returns") }

type payload interface{ Read(p []byte) (int, error) }
type response struct {
	body payload
	next *response
}

func Deref(r *reader) int                     { return r.id }
func Guarded(r *reader) int                   { if r == nil { return 0 }; return r.id }
func Fatal(r *reader) int                     { if r == nil { panic("nil") }; return r.id }
func Body(resp *response) error               { _, err := resp.body.Read(nil); return err }
func Next(resp *response) int                 { return resp.next.id() }
func (r *response) id() int                   { return 0 }
func Fresh(r *reader) int                     { local := &reader{}; return local.id + r.id }
func Assign(r *reader) { r.id = 1 }
func ViaDeref(r *reader) int                  { return Deref(r) }
`)
	assertRequirements(t, pkg, map[string][]string{
		"Drain":        {"P0 method Read"},
		"Sometimes":    nil,
		"Twice":        {"P0 method Err", "P0 method Read"},
		"ViaHelper":    {"P0 method Read"},
		"ViaSometimes": nil,
		"Replaced":     nil,
		"Held":         {"P0/field:0 method Read"},
		"Either":       nil,
		"Panics":       nil,
		"Deref":        {"P0 non-nil"},
		"Guarded":      nil,
		"Fatal":        {"P0 non-nil"},
		"Body":         {"P0 non-nil", "P0/field:0 method Read", "P0/field:0 non-nil"},
		"Next":         {"P0 non-nil", "P0/field:1 method id"},
		"Fresh":        {"P0 non-nil"},
		"Assign":       {"P0 non-nil"},
		"ViaDeref":     {"P0 non-nil"},
	})
}

// assertRequirements checks the rendered requirements each named function
// is summarized with.
func assertRequirements(t *testing.T, pkg *ssa.Package, wants map[string][]string) {
	t.Helper()
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range wants {
		fact := summarize(pass, pkg.Func(name))
		if fact.Heap == nil {
			t.Fatalf("%s: no projection", name)
		}
		var got []string
		for _, requirement := range fact.Heap.Requires {
			got = append(got, requirement.String())
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s requires %q, want %q\n%s", name, got, want, fact.Heap.String())
		}
	}
}

// A callee's requirement on a slot beneath an object the caller built is a
// requirement on whatever that slot certainly holds at the call. It moves
// to a parameter only when the slot holds exactly that parameter's object
// there: not one reassigned away before the call, set on one branch only,
// handed to unknown code first, or a local object nobody named.
func TestLifecycleSummaryRequirementsThroughWrappers(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type reader struct{ id int }

func (r *reader) Read(p []byte) (int, error) { return 0, nil }

type wrap struct{ r *reader }
type outer struct{ w *wrap }

var sink func(*wrap)

func deref(w *wrap) int                       { return w.r.id }
func drain(w *wrap) error                     { _, err := w.r.Read(nil); return err }
func derefOuter(o *outer) int                 { return o.w.r.id }

func Wrapped(r *reader) int                   { w := &wrap{r: r}; return deref(w) }
func WrappedMethod(r *reader) error           { w := &wrap{r: r}; return drain(w) }
func DoubleWrapped(r *reader) int             { return derefOuter(&outer{w: &wrap{r: r}}) }
func Reassigned(r, other *reader) int         { w := &wrap{r: r}; w.r = other; return deref(w) }
func OnBranch(r *reader, ok bool) int         { w := &wrap{}; if ok { w.r = r }; return deref(w) }
func Escaped(r *reader) int                   { w := &wrap{r: r}; sink(w); return deref(w) }
func LocalObject(r *reader) int               { w := &wrap{r: &reader{}}; return deref(w) + len(r.String()) }
func (r *reader) String() string              { return "" }
`)
	assertRequirements(t, pkg, map[string][]string{
		"Wrapped":       {"P0 non-nil"},
		"WrappedMethod": {"P0 method Read"},
		"DoubleWrapped": {"P0 non-nil"},
		"Reassigned":    {"P1 non-nil"},
		"OnBranch":      nil,
		"Escaped":       {"G:example.com/lifecyclefactstest.sink non-nil"},
		"LocalObject":   {"P0 method String"},
	})
}

func TestHeapTraceCountsWithoutWarmingCache(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heaptrace", `package heaptrace
type user interface { Use() }
func calls(x user) {
`+strings.Repeat("x.Use()\n", 10)+"}\n")
	function := pkg.Func("calls")
	before := heapTraceDetails(function, nil)
	if before["heap-cached"] != "false" || heapmodel.CachedGraphEvidence(function).Cached {
		t.Fatal("trace collection built a graph")
	}
	// Explicitly request the graph as an ordinary consumer would, then observe.
	heapmodel.RenderRegions(function)
	after := heapTraceDetails(function, &heapmodel.HeapSummary{})
	for key, want := range map[string]string{
		"heap-cached": "true", "heap-building": "false", "heap-build-reason": "graph-build-complete",
		"heap-truncated-count": "0", "calls-interface-call": "10", "calls-applied": "0", "calls-applied-truncated": "0",
	} {
		if after[key] != want {
			t.Errorf("%s: got %q, want %q", key, after[key], want)
		}
	}
	if count := strings.Count(after["calls-unsummarized"], "[interface-call]"); count != tracedCallLimit {
		t.Fatalf("sample has %d calls, want %d (full count must remain 10)", count, tracedCallLimit)
	}
}
