package heapmodel

import (
	"fmt"
	"go/types"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// The graph must answer every storage question the demand-driven model
// answers, with the same must-polarity, before any helper can delegate to
// it. This table is the existing storage table verbatim.
func TestRegionGraphStorageParity(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"literal", `x := box{value:a}; observe(x.value, a)`, true},
		{"copy", `x := box{value:a}; y := x; x.value = b; observe(y.value, a)`, true},
		{"copyNotCurrent", `x := box{value:a}; y := x; x.value = b; observe(y.value, b)`, false},
		{"overwrite", `x := box{value:a}; x = box{value:b}; observe(x.value, b)`, true},
		{"overwriteNotOld", `x := box{value:a}; x = box{value:b}; observe(x.value, a)`, false},
		{"arrayCopy", `x := [2]*int{a,b}; y := x; x[0] = b; observe(y[0], a)`, true},
		{"opaqueSlot", `x := box{value:a}; opaque(&x.value); observe(x.value, a)`, false},
		{"opaqueRoot", `x := box{value:a}; opaque(&x); observe(x.value, a)`, false},
		{"opaqueAfterSnapshot", `x := box{value:a}; old := x.value; opaque(&x); observe(old, a)`, true},
		{"ambiguousWrite", `x := box{value:a}; if pick { x.value = b }; observe(x.value, a)`, false},
		{"dynamicWrite", `x := [2]*int{a,b}; x[idx] = b; observe(x[0], a)`, false},
		{"capturedMutation", `x := box{value:a}; f := func() { x.value = b }; f(); observe(x.value, a)`, false},
		{"cyclicCell", `var x any; x = &x; opaque(x); observe(a,b)`, false},
		{"overwriteAfterBranches", `var x box; if pick { x.value=a } else { x.value=b }; x.value=a; observe(x.value,a)`, true},
		{"agreeingBranches", `var x box; if pick { x.value=a } else { x.value=a }; observe(x.value,a)`, true},
		{"conflictingBranches", `var x box; if pick { x.value=a } else { x.value=b }; observe(x.value,a)`, false},
		{"missingBranchWrite", `var x box; if pick { x.value=a }; observe(x.value,a)`, false},
		{"independentField", `x:=box{value:a}; x.count=1; observe(x.value,a)`, true},
		{"independentBranchField", `x:=box{value:a}; if pick { x.count=1 } else { x.count=2 }; observe(x.value,a)`, true},
		{"unchangedAcrossLoop", `x:=box{value:a}; for pick { opaque(b) }; observe(x.value,a)`, true},
		{"unchangedInsideLoop", `x:=box{value:a}; for pick { observe(x.value,a) }`, true},
		{"changedInsideLoop", `x:=box{value:a}; for pick { x.value=b }; observe(x.value,a)`, false},
		{"fixedOffset", `x:=[3]*int{b,a,b}; s:=x[1:]; observe(s[0],a)`, true},
		{"nestedOffset", `x:=[3]*int{b,b,a}; s:=x[1:][1:]; observe(s[0],a)`, true},
		{"offsetWrongSlot", `x:=[3]*int{a,b,b}; s:=x[1:]; observe(s[0],a)`, false},
		{"offsetWrite", `x:=[3]*int{a,a,a}; s:=x[1:]; s[0]=b; observe(x[1],b)`, true},
		{"offsetSibling", `x:=[3]*int{a,a,a}; s:=x[1:]; s[0]=b; observe(x[0],a)`, true},
		{"offsetEscape", `x:=[3]*int{a,a,a}; s:=x[1:]; opaque(s); observe(x[1],a)`, false},
		{"dynamicOffset", `x:=[3]*int{a,a,a}; s:=x[idx:]; observe(s[0],a)`, false},
		{"fullSliceBounds", `x:=[3]*int{b,a,b}; s:=x[1:2:3]; observe(s[0],a)`, true},
		{"pastSliceLength", `x:=[3]*int{b,a,a}; s:=x[1:2:3]; observe(s[1],a)`, false},
		{"readOnlyCall", `x:=box{value:a}; read(&x); observe(x.value,a)`, true},
		{"readOnlyForwarding", `x:=box{value:a}; forward(&x); observe(x.value,a)`, true},
		{"mutatingCall", `x:=box{value:a}; mutate(&x,b); observe(x.value,a)`, false},
		{"retainingCall", `x:=box{value:a}; retain(&x); observe(x.value,a)`, true},
		{"asyncReadCall", `x:=box{value:a}; async(&x); observe(x.value,a)`, false},
		// Beyond the demand-driven model.
		{"loopInitializer", `var x box; x.value=a; for pick { observe(x.value,a) }`, true},
		{"fieldOfCopiedPointee", `y := *p; observe(y.value, p.value)`, true},
		{"fieldOfCopiedPointeeOther", `y := *p; observe(y.value, p.count)`, false},
		{"twoLoadsUntouched", `observe(p.value, p.value)`, true},
		// A call the graph cannot follow reaches only what it was handed:
		// a parameter never let out reads as the same object on both sides.
		{"twoLoadsAcrossCall", `u := p.value; opaque(nil); observe(u, p.value)`, true},
		{"twoLoadsAcrossCallHanded", `u := p.value; opaque(p); observe(u, p.value)`, false},
		{"twoLoadsAcrossCallEscaped", `retain(p); u := p.value; opaque(nil); observe(u, p.value)`, false},
		{"twoGlobalLoadsAcrossCall", `u := saved.value; opaque(nil); observe(u, saved.value)`, false},
		{"twoLoadsAcrossOtherField", `u := p.value; q.count = 1; observe(u, p.value)`, true},
		{"twoLoadsAcrossSameField", `u := p.value; q.value = b; observe(u, p.value)`, false},
		{"phiOfSame", `y := a; if pick { y = a }; observe(y, a)`, true},
		{"phiOfDifferent", `y := a; if pick { y = b }; observe(y, a)`, false},
		{"cellThroughPointer", `x := box{value:a}; ptr := &x; ptr.value = b; observe(x.value, b)`, true},
		// An unresolved call reaches only what it was handed, what escaped
		// before it, and globals; a parameter this function never let out
		// keeps what the function wrote into it.
		{"parameterWrittenAcrossCall", `p.value = a; opaque(nil); observe(p.value, a)`, true},
		{"parameterHandedToCall", `p.value = a; opaque(p); observe(p.value, a)`, false},
		{"parameterEscapedBeforeCall", `p.value = a; retain(p); opaque(nil); observe(p.value, a)`, false},
		{"parameterContentHandedToCall", `p.value = a; opaque(p.value); observe(p.value, a)`, true},
		{"globalAcrossCall", `saved.value = a; opaque(nil); observe(saved.value, a)`, false},
		{"parameterAcrossOtherParameterStore", `p.value = a; w.value = b; observe(p.value, a)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "regionprobe", `package regionprobe
type box struct { value *int; count int }
func observe(a,b any) {}
func opaque(any)
var saved *box
func read(p *box) int { return p.count }
func forward(p *box) int { return read(p) }
func mutate(p *box,b *int) { p.value=b }
func retain(p *box) { saved=p }
func async(p *box) { go read(p) }
func probe(a,b *int, p, q *box, pick bool, idx int, w *box) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			left, right := unwrapInterface(args[0]), unwrapInterface(args[1])
			graph := regionsOfFunction(call.Parent())
			if !graph.available {
				t.Fatal("graph unavailable")
			}
			if got := graph.mustSame(left, right); got != test.want {
				t.Fatalf("mustSame = %t, want %t (left %v right %v)", got, test.want, graph.values[left], graph.values[right])
			}
			if !graph.aliasProof(left, right).Aliases && test.want {
				t.Fatal("must-same values must may-alias")
			}
		})
	}
}

// aliasReasons names the rule behind selected answers.
var aliasReasons = map[string]proofs.EvidenceReason{
	"fieldsOfOneObject":        proofs.EvidenceDisjointPaths,
	"elementsApart":            proofs.EvidenceDisjointPaths,
	"sameFieldTwoObjects":      proofs.EvidenceDisjointObjects,
	"callResults":              proofs.EvidenceDisjointObjects,
	"localAndParameter":        proofs.EvidenceUnescapedLocal,
	"escapedLocalAndParameter": proofs.EvidenceUnescapedLocal,
	"elementAndStar":           proofs.EvidenceSharedSlot,
	"loadsAcrossCall":          proofs.EvidenceSharedSlot,
}

func unwrapInterface(value ssa.Value) ssa.Value { //nolint:ireturn // Test helper over SSA forms.
	if made, ok := value.(*ssa.MakeInterface); ok {
		return made.X
	}
	return value
}

// May-alias keeps the structural contract: objects are the same only when
// the function's own flow connects them. Two parameters, two call results,
// and two fields of one object are distinct; a cell after an overwrite no
// longer holds what it held; a placeholder still matches what was ever
// stored into its slot, and a dynamic element matches every element.
func TestRegionGraphMayAlias(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"fieldsOfOneObject", `observe(&p.value, &p.other)`, false},
		{"contentsOfTwoFields", `observe(p.value, p.other)`, false},
		{"sameFieldTwoObjects", `observe(p.value, q.value)`, false},
		{"localAndParameter", `x := box{}; observe(&x, p)`, false},
		{"escapedLocalAndParameter", `x := box{}; retain(&x); observe(&x, p)`, false},
		{"localsApart", `x := box{}; y := box{}; observe(&x, &y)`, false},
		{"elementAndStar", `var x [2]*int; observe(&x[0], &x[idx])`, true},
		{"elementsApart", `var x [2]*int; observe(&x[0], &x[1])`, false},
		{"callResults", `observe(acquire(), acquire())`, false},
		{"nilAndPointer", `observe(nil, p)`, false},
		{"storedThenReadAfterCall", `p.value = a; saved = nil; observe(p.value, a)`, true},
		{"storedThenReadAfterCallOther", `p.value = a; saved = nil; observe(p.value, b)`, false},
		{"loadsAcrossCall", `u := p.value; saved = nil; observe(u, p.value)`, true},
		{"overwrittenCell", `x := a; x = b; observe(x, a)`, false},
		{"overwrittenCellCurrent", `x := a; x = b; observe(x, b)`, true},
		{"mergedCell", `x := a; if pick { x = b }; observe(x, a)`, true},
		{"clobberedCellHeld", `x := box{value: a}; escapeBox(&x); observe(x.value, a)`, true},
		{"clobberedCellOther", `x := box{value: a}; escapeBox(&x); observe(x.value, b)`, false},
		{"phiOfParameters", `y := a; if pick { y = b }; observe(y, b)`, true},
		// Selecting through nil faults, so a nil start names no slot and
		// leaves the other base's element distinct from the parameter...
		{"elementOfNilOrAppended", `var s []*int; if pick { s = append(s, a) }; observe(&s[idx], p)`, false},
		// ...while the non-nil base still aliases what it selects from.
		{"fieldOfNilOrParameter", `var x *box; if pick { x = p }; observe(&x.value, &p.value)`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "regionprobe", `package regionprobe
type box struct { value *int; other *int }
func observe(a,b any) {}
func acquire() *box
func escapeBox(*box)
var saved *box
func retain(p *box) { saved=p }
func probe(a, b *int, p, q *box, pick bool, idx int) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			left, right := unwrapInterface(args[0]), unwrapInterface(args[1])
			graph := regionsOfFunction(call.Parent())
			proof := graph.aliasProof(left, right)
			if proof.Aliases != test.want {
				t.Fatalf("mayAlias = %+v, want %t (left %v right %v)", proof, test.want, graph.values[left], graph.values[right])
			}
			if want, ok := aliasReasons[test.name]; ok && proof.Reason != want {
				t.Fatalf("reason = %s, want %s", proof.Reason, want)
			}
			if !proof.Aliases && len(graph.disjoint) == 0 {
				t.Fatal("a disjointness answer must be recorded for the dump")
			}
		})
	}
}

// The rendering names each value's pointees by region kind and origin, so
// a reader can check an alias answer against the graph that gave it.
func TestRenderRegions(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "regionprobe", `package regionprobe
type box struct { value *int }
func probe(a *int) *int { x := box{value: a}; return x.value }
`)
	rendered := RenderRegions(pkg.Func("probe"))
	for _, want := range []string{"// regions:", "a -> param:a", "-> local:t0 field:0", "-> param:a"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendering lacks %q:\n%s", want, rendered)
		}
	}
}

// A result or lifecycle pass can finish a graph while another pass reads the
// same cache entry. Both the lookup and the published pointer must use the
// cache lock; returning a graph does not require holding the lock afterwards.
func TestRegionGraphCacheConcurrentPublication(t *testing.T) {
	function := &ssa.Function{Blocks: []*ssa.BasicBlock{{}}}
	entry := &regionGraphEntry{function: function, graph: &regionGraph{available: true}}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		evictLocked(entry)
		regionGraphs.Unlock()
	})

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for range 10000 {
			if graph := regionsOfFunction(function); graph == nil || !graph.available {
				t.Error("cache lookup returned no published graph")
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for range 10000 {
			cacheRegionGraph(entry, &regionGraph{available: true})
		}
	}()
	close(start)
	workers.Wait()
}

// A lookup that finds another analyzer's build in progress waits for it and
// returns the published graph, never an unavailable placeholder: answering
// from the placeholder made results depend on scheduling.
func TestRegionGraphLookupWaitsForBuild(t *testing.T) {
	function := &ssa.Function{Blocks: []*ssa.BasicBlock{{}}}
	entry := &regionGraphEntry{function: function, done: make(chan struct{})}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		if element, ok := regionGraphs.entries[function]; ok {
			delete(regionGraphs.entries, function)
			regionGraphs.order.Remove(element)
		}
		regionGraphs.Unlock()
	})
	published := &regionGraph{available: true}
	looked := make(chan *regionGraph, 1)
	go func() { looked <- regionsOfFunction(function) }()
	select {
	case graph := <-looked:
		t.Fatalf("lookup returned %+v before the build finished", graph)
	case <-time.After(20 * time.Millisecond):
	}
	cacheRegionGraph(entry, published)
	if graph := <-looked; graph != published {
		t.Fatalf("lookup returned %+v, want the published graph", graph)
	}
}

func TestRegionGraphStaleBuildKeepsReplacement(t *testing.T) {
	function := &ssa.Function{Blocks: []*ssa.BasicBlock{{}}}
	entry := &regionGraphEntry{function: function, done: make(chan struct{})}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	evictLocked(entry)
	regionGraphs.Unlock()
	select {
	case <-entry.done:
		t.Fatal("eviction finished a build that is still running")
	default:
	}
	replacement := &regionGraphEntry{function: function, graph: &regionGraph{available: true}}
	regionGraphs.Lock()
	element := regionGraphs.order.PushFront(replacement)
	regionGraphs.entries[function] = element
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		evictLocked(replacement)
		regionGraphs.Unlock()
	})
	cacheRegionGraph(entry, &regionGraph{available: true})
	select {
	case <-entry.done:
	default:
		t.Fatal("rejected publication did not finish its build")
	}
	regionGraphs.Lock()
	kept := regionGraphs.entries[function] == element && element.Value == replacement && !replacement.stale
	rejected := entry.stale && entry.graph == nil
	regionGraphs.Unlock()
	if !kept || !rejected {
		t.Fatalf("stale publication changed replacement: kept=%v rejected=%v", kept, rejected)
	}
}

func TestRegionGraphRejectsChangedSummary(t *testing.T) {
	function, callee := &ssa.Function{}, &ssa.Function{}
	entry := &regionGraphEntry{function: function, done: make(chan struct{})}
	graph := &regionGraph{available: true, consulted: map[*ssa.Function]int{callee: heapSummaryGeneration(callee)}}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		evictLocked(entry)
		delete(regionGraphs.dependents, callee)
		regionGraphs.Unlock()
		heapSummaries.Lock()
		delete(heapSummaries.entries, callee)
		delete(heapSummaries.generations, callee)
		heapSummaries.Unlock()
	})
	RegisterHeapSummary(callee, HeapSummary{})
	cacheRegionGraph(entry, graph)
	select {
	case <-entry.done:
	default:
		t.Fatal("changed-summary rejection did not finish its build")
	}
	regionGraphs.Lock()
	_, cached := regionGraphs.entries[function]
	indexed := regionGraphs.dependents[callee][entry]
	rejected := entry.stale && entry.graph == nil
	regionGraphs.Unlock()
	if cached || indexed || !rejected {
		t.Fatalf("changed-summary build was published: cached=%v indexed=%v rejected=%v", cached, indexed, rejected)
	}
}

func TestRegionHistoryWidensConservatively(t *testing.T) {
	owner := &region{kind: regionSite}
	unknown := &region{kind: regionUnknown}
	held := slot{region: owner, path: "field:0"}
	graph := &regionGraph{history: map[slot]pointees{}, unkR: unknown}
	objects := make([]slot, pointeeLimit+1)
	for index := range objects {
		objects[index] = slot{region: &region{kind: regionSite, serial: index}}
		graph.remember(held, pointees{objects[index]: false})
		if index == pointeeLimit-1 && len(graph.history[held]) != pointeeLimit {
			t.Fatalf("history below limit has %d objects, want %d", len(graph.history[held]), pointeeLimit)
		}
	}
	if got := graph.history[held]; len(got) != 1 || !got.unknown() {
		t.Fatalf("history above limit = %v, want unknown only", got)
	}
	graph.remember(held, pointees{objects[0]: false})
	if got := graph.history[held]; len(got) != 1 || !got.unknown() {
		t.Fatalf("widened history grew again: %v", got)
	}
	from, target := pointees{{region: owner}: false}, pointees{objects[0]: false}
	if !containsThroughSlots(graph.history, from, target) {
		t.Fatal("widened slot history must admit possible transitive containment")
	}
	if !graph.everContainedUnlocked(slot{region: owner}, pointees{objects[0]: false}, nil) {
		t.Fatal("unknown history must admit possible containment")
	}
}

func TestFreshLoopAllocationDoesNotKeepEarlierContents(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "regionreset", `package regionreset
type box struct{value *int}
func observe(a,b *int){}
func probe(value *int, again bool){
 for again {var local box; observe(local.value,value);local.value=value}
}
`)
	function := pkg.Func("probe")
	var dump strings.Builder
	if _, err := function.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	call := heapObservation(t, function)
	graph := regionsOfFunction(function)
	if !graph.available {
		t.Fatalf("graph unavailable: %s", graph.buildReason)
	}
	arguments := call.Common().Args
	if graph.aliasProof(arguments[0], arguments[1]).Aliases {
		t.Fatal("zero field of a fresh loop allocation retained an earlier iteration's value")
	}
}

func TestRegionContainmentObservation(t *testing.T) {
	for _, test := range []struct {
		name, body             string
		history, before, after bool
	}{
		{"laterStore", `x := node{}; observe(&x,a); x.value=a; observe(&x,a)`, true, false, true},
		{"replacedStore", `x := node{value:a}; observe(&x,a); x.value=b; observe(&x,a)`, true, true, false},
		{"unrelated", `x := node{value:b}; observe(&x,a); x.value=b; observe(&x,a)`, false, false, false},
		{"nested", `x,y := node{},node{}; x.next=&y; observe(&x,a); y.value=a; observe(&x,a)`, true, false, true},
		{"cycleApart", `x := node{}; x.next=&x; observe(&x,a); observe(&x,a)`, false, false, false},
		{"cycleContains", `x := node{value:a}; x.next=&x; observe(&x,a); observe(&x,a)`, true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			checkRegionContainment(t, test.body, test.history, test.before, test.after)
		})
	}
}

func TestRegionContainmentDepth(t *testing.T) {
	for _, links := range []int{aliasDepth - 1, aliasDepth} {
		t.Run(fmt.Sprintf("links%d", links), func(t *testing.T) {
			var body strings.Builder
			for index := range links + 1 {
				fmt.Fprintf(&body, "n%d := node{}; ", index)
			}
			for index := range links {
				fmt.Fprintf(&body, "n%d.next=&n%d; ", index, index+1)
			}
			fmt.Fprintf(&body, "n%d.value=a; observe(&n0,a); observe(&n0,a)", links)
			// Each node link consumes one level; reaching its value consumes another.
			want := links < aliasDepth
			checkRegionContainment(t, body.String(), want, want, want)
		})
	}
}

func checkRegionContainment(t *testing.T, body string, history, before, after bool) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "containmentprobe", `package containmentprobe
type node struct { next *node; value *int }
func observe(a,b any) {}
func probe(a,b *int) { `+body+` }
`)
	var observations []*ssa.Call
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("probe")) {
		if ssaflow.CallName(call.Common()) == "observe" {
			observations = append(observations, call)
		}
	}
	if len(observations) != 2 {
		t.Fatalf("want two observations, got %d", len(observations))
	}
	for index, observation := range observations {
		owner, target := unwrapInterface(observation.Common().Args[0]), unwrapInterface(observation.Common().Args[1])
		if got := Contains(owner, target); got != history {
			t.Errorf("observation %d: history = %t, want %t", index, got, history)
		}
		want := []bool{before, after}[index]
		if got, known := ContainsAt(owner, target, observation); !known || got != want {
			t.Errorf("observation %d: at = %t, known = %t, want %t", index, got, known, want)
		}
		if got, known := ContainsAt(owner, target, nil); known || got {
			t.Errorf("missing observation must remain unknown, got %t, known = %t", got, known)
		}
	}
}

func TestContentIsNilAcrossPointerField(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "nilpathprobe", `package nilpathprobe
type inner struct{ value *int }
type outer struct{ next *inner }
type inline struct{ next inner }
func external() *inner
func observe(any) {}
func opaque() { o := &outer{next: external()}; observe(o) }
func knownNil() { o := &outer{next: &inner{}}; observe(o) }
func nilParent() { o := &outer{}; observe(o) }
func inlineNil() { o := &inline{}; observe(o) }
`)
	for _, test := range []struct {
		name string
		path []string
		want bool
	}{
		{"opaque", []string{"field:0", "field:0"}, false},
		{"knownNil", []string{"field:0", "field:0"}, true},
		{"nilParent", []string{"field:0"}, true},
		{"nilParent", []string{"field:0", "field:0"}, false},
		{"inlineNil", []string{"field:0", "field:0"}, true},
	} {
		t.Run(test.name+"/"+test.path[len(test.path)-1], func(t *testing.T) {
			call := heapObservation(t, pkg.Func(test.name))
			root := unwrapInterface(call.Common().Args[0])
			got := regionsOfFunction(call.Parent()).contentIsNil(root, test.path, call)
			if got != test.want {
				t.Errorf("contentIsNil(%v) = %t, want %t\n%s", test.path, got, test.want, RenderRegions(call.Parent()))
			}
		})
	}
}

// The nil query only supports these tests since its analyzer was removed.
// contentIsNil reports whether the slot at path beneath the root's object
// certainly holds nil when the instruction runs: one non-stale entry, and
// it is nil. An empty path asks about the root itself. Intermediate pointer
// fields must be followed to their pointee; a local outer object's zero
// slots say nothing about the pointed-to object's fields.
func (graph *regionGraph) contentIsNil(root ssa.Value, path []string, at ssa.Instruction) bool {
	defer graph.lock()()
	set, ok := graph.pointsToUnlocked(root)
	if !ok {
		return false
	}
	base, ok := singleSlot(set)
	if !ok {
		return false
	}
	if len(path) == 0 {
		return base.region.kind == regionNil
	}
	state := graph.stateAt(at)
	if state == nil {
		return false
	}
	target := base
	if len(path) > 1 {
		// A flattened path through a pointer field would inspect the local
		// outer object's unwritten slot, not the pointee's field. Follow only
		// exact pointer contents; an opaque pointee leaves the answer unknown.
		// https://github.com/timescale/timescaledb-tune/blob/c7a642bd4e16d48a51c060a43dc6dff864cb42d7/pkg/tstune/config_file_test.go#L25-L26
		currentType := root.Type()
		if pointer, ok := currentType.Underlying().(*types.Pointer); ok {
			currentType = pointer.Elem()
		}
		for _, step := range path[:len(path)-1] {
			target.path = joinSlotPath(target.path, step)
			fieldType, ok := selectedFieldType(currentType, step)
			if !ok {
				return false
			}
			switch typed := fieldType.Underlying().(type) {
			case *types.Pointer:
				held, exact := singleSlot(graph.content(state, target))
				if !exact || held.region.kind == regionNil {
					return false
				}
				target = held
				currentType = typed.Elem()
			case *types.Struct:
				currentType = fieldType
			default:
				return false
			}
		}
		target.path = joinSlotPath(target.path, path[len(path)-1])
	} else {
		target.path = joinSlotPath(target.path, ssaflow.JoinAccessPath(path))
	}
	held, ok := singleSlot(graph.content(state, target))
	return ok && held.region.kind == regionNil
}

func selectedFieldType(parent types.Type, step string) (types.Type, bool) {
	indexText, hasField := strings.CutPrefix(step, "field:")
	if !hasField {
		return nil, false
	}
	structure, ok := parent.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || index >= structure.NumFields() {
		return nil, false
	}
	return structure.Field(index).Type(), true
}
