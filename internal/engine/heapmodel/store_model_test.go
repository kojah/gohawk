package heapmodel

import (
	"go/token"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStorageSnapshots(t *testing.T) {
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
		// The callee's summary says it stored the address in a global and
		// wrote nothing through it, so the field still holds a.
		{"retainingCall", `x:=box{value:a}; retain(&x); observe(x.value,a)`, true},
		{"asyncReadCall", `x:=box{value:a}; async(&x); observe(x.value,a)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "storageprobe", `package storageprobe
type box struct { value *int; count int }
func observe(a,b *int) {}
func opaque(any)
var saved *box
func read(p *box) int { return p.count }
func forward(p *box) int { return read(p) }
func mutate(p *box,b *int) { p.value=b }
func retain(p *box) { saved=p }
func async(p *box) { go read(p) }
func probe(a,b *int, pick bool, idx int) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			proof := NewStorage(proofs.NewSearchBudget(1000)).Same(args[0], args[1])
			if proof.Proven() != test.want {
				t.Fatalf("Same() = %+v, want proven %t", proof, test.want)
			}
			if proof := NewStorage(proofs.NewSearchBudget(0)).Resolve(args[0]); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
				t.Fatalf("exhaustion = %+v", proof)
			}
		})
	}
}

func TestStorageDeferredObservation(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"initial", `x:=a; inspect(&x)`, true},
		{"replacedBefore", `x:=a; x=b; inspect(&x)`, true},
		{"replacedAfter", `x:=a; inspect(&x); x=b`, false},
		{"opaqueAfter", `x:=a; inspect(&x); opaque(&x)`, false},
		{"mutableCapture", `x:=a; inspect(&x); f:=func(){ x=b }; f()`, false},
		{"reusedInLoop", `var x *int; for pick { x=a; inspect(&x) }`, false},
		{"stableInsideLoop", `x:=a; for pick { inspect(&x) }`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "storageprobe", `package storageprobe
func inspect(**int) {}
func opaque(any)
func probe(a,b *int, pick bool) { `+test.body+` }
`)
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("probe")) {
				if ssaflow.CallName(call.Common()) != "inspect" {
					continue
				}
				proof := NewStorage(proofs.NewSearchBudget(1000)).StableContent(call.Common().Args[0], call)
				if proof.Proven() != test.want {
					t.Fatalf("StableContent() = %+v, want proven %t", proof, test.want)
				}
				return
			}
			t.Fatal("missing observation")
		})
	}
}

func TestStorageAggregateSnapshots(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"unchanged", `x:=a; observe(x,a)`, true},
		{"fieldChanged", `x:=a; x.value=b; observe(x,a)`, false},
		{"snapshotBeforeChange", `x:=a; old:=x; x.value=b; observe(old,a)`, true},
		{"wholeOverwrite", `x:=a; x.value=b; x=a; observe(x,a)`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "storageprobe", `package storageprobe
type box struct { value *int }
func observe(a,b box) {}
func probe(a box,b *int) { `+test.body+` }
`)
			call := heapObservation(t, pkg.Func("probe"))
			args := call.Common().Args
			proof := NewStorage(proofs.NewSearchBudget(1000)).Same(args[0], args[1])
			if proof.Proven() != test.want {
				t.Fatalf("Same() = %+v, want proven %t", proof, test.want)
			}
		})
	}
}

func TestGraphBuildReasonCodes(t *testing.T) {
	want := map[GraphBuildReason]string{
		GraphBuildUnknown: "graph-build-unknown", GraphBuildComplete: "graph-build-complete",
		GraphBuildNoBody: "graph-build-no-body", GraphBuildBudgetExhausted: "graph-build-budget-exhausted",
		GraphBuildFixpointLimit: "graph-build-fixpoint-limit",
	}
	if len(want) != int(graphBuildReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range graphBuildReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []GraphBuildReason{graphBuildReasonCount, 255} {
		if reason.String() != "invalid-graph-build-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

func TestGraphBuildFailureClassification(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "graphreasons", `package graphreasons
func body() *int { return new(int) }
func opaque()
`)
	graph := buildRegionGraph(pkg.Func("body"))
	if !graph.available || graph.buildReason != GraphBuildComplete {
		t.Fatal("complete graph was not classified")
	}
	graph.budget = proofs.NewSearchBudget(1)
	reason, detail := graph.fixpoint()
	if reason != GraphBuildBudgetExhausted || detail != "" {
		t.Fatalf("budget: reason=%s detail=%q", reason, detail)
	}
	graph.buildReason, graph.buildDetail = reason, detail
	if graph.buildFailureText() != "budget exhausted" {
		t.Fatal("budget dump spelling changed")
	}
	opaque := &regionGraph{function: pkg.Func("opaque")}
	if reason, _ := opaque.fixpoint(); reason != GraphBuildNoBody {
		t.Fatal("absent body classified as complete")
	}
	graph.buildReason, graph.buildDetail = GraphBuildFixpointLimit, "in 6 rounds; changes: example"
	if graph.buildFailureText() != "fixpoint did not settle in 6 rounds; changes: example" {
		t.Fatal("fixpoint dump spelling changed")
	}
}

func TestCachedGraphEvidenceDoesNotBuild(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observation", `package observation
func untouched() *int { return new(int) }
`)
	function := pkg.Func("untouched")
	if got := CachedGraphEvidence(function); got.Cached || got.BuildReason != GraphBuildUnknown || len(got.Calls) != 0 {
		t.Fatalf("observation built a graph: %+v", got)
	}
	if allocations := testing.AllocsPerRun(100, func() { CachedGraphEvidence(function) }); allocations != 0 {
		t.Fatalf("cache miss allocated: %v", allocations)
	}
	regionGraphs.Lock()
	_, found := regionGraphs.entries[function]
	regionGraphs.Unlock()
	if found {
		t.Fatal("observation published a graph")
	}
}

func TestCachedGraphEvidenceDeduplicatesAndCopies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observation", `package observation
func opaque(*int)
func body() { p := new(int); opaque(p) }
`)
	function := pkg.Func("body")
	graph := regionsOfFunction(function)
	if len(graph.applied) != 1 {
		t.Fatalf("expected opaque call record, got %d", len(graph.applied))
	}
	instruction := graph.applied[0].instruction
	// Two visits of one site are one loss; a different slot is another.
	graph.widened = []widening{
		{target: slot{region: graph.unkR}, at: instruction, size: 33},
		{target: slot{region: graph.unkR}, at: instruction, size: 34},
		{target: slot{region: graph.nilR}, at: instruction, size: 33},
	}
	first := CachedGraphEvidence(function)
	if !first.Cached || first.Building || first.BuildReason != GraphBuildComplete || first.Widenings != 2 || len(first.Calls) != 1 {
		t.Fatalf("unexpected snapshot: %+v", first)
	}
	want := first.Calls[0].Reason
	first.Calls[0].Reason = CallApplicationUnknown
	second := CachedGraphEvidence(function)
	if second.Calls[0].Reason != want || second.Widenings != first.Widenings {
		t.Fatal("snapshot mutation or repeated reads changed authoritative evidence")
	}
}

func TestCachedGraphEvidenceBuilding(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "building", `package building
func body() {}
`)
	function := pkg.Func("body")
	regionGraphs.Lock()
	element := regionGraphs.order.PushFront(&regionGraphEntry{function: function})
	regionGraphs.entries[function] = element
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		delete(regionGraphs.entries, function)
		regionGraphs.order.Remove(element)
		regionGraphs.Unlock()
	})
	if got := CachedGraphEvidence(function); !got.Cached || !got.Building || got.BuildReason != GraphBuildUnknown {
		t.Fatalf("in-progress graph presented as complete: %+v", got)
	}
}

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
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", storageGiveUpFixture)
	want := map[string]proofs.EvidenceReason{
		"conflicting": proofs.EvidenceStorageConflictingWrites,
		"escaped":     proofs.EvidenceStorageAddressEscapes,
		"partial":     proofs.EvidenceStoragePartialWrite,
		"param":       proofs.EvidenceStorageNotLocal,
	}
	for name, reason := range want {
		t.Run(name, func(t *testing.T) {
			var observed []string
			observer := func(reason string, _ token.Pos, _ map[string]string) { observed = append(observed, reason) }
			load := returnedLoad(t, pkg.Func(name))
			content := NewStorage(proofs.NewSearchBudget(1000).Observed(observer)).Content(load.X, load)
			if content.Proven() || content.Reason != reason {
				t.Fatalf("Content = %+v, want reason %s", content.Proof, reason)
			}
			if len(observed) == 0 || observed[len(observed)-1] != reason.String() {
				t.Fatalf("observed %v, want %s last", observed, reason)
			}
		})
	}
	t.Run("stable", func(t *testing.T) {
		function := pkg.Func("stable")
		closures := ssaflow.InstructionsOf[*ssa.MakeClosure](function)
		if len(closures) != 1 {
			t.Fatalf("got %d closures, want one", len(closures))
		}
		closure := closures[0]
		cell := closure.Bindings[0]
		content := NewStorage(proofs.NewSearchBudget(1000)).StableContent(cell, closure)
		if content.Proven() || content.Reason != proofs.EvidenceStorageWriteAfterObservation {
			t.Fatalf("StableContent = %+v, want write after observation", content.Proof)
		}
	})
	t.Run("budget", func(t *testing.T) {
		load := returnedLoad(t, pkg.Func("conflicting"))
		content := NewStorage(proofs.NewSearchBudget(1)).Content(load.X, load)
		if content.Proven() || content.Reason != proofs.EvidenceBudgetExhausted {
			t.Fatalf("Content = %+v, want budget exhausted", content.Proof)
		}
	})
}

// A silent budget must cost nothing at a give-up: the details are built only
// when an observer is attached, so tracing that is off allocates nothing.
func TestSilentBudgetGiveUpAllocatesNothing(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", storageGiveUpFixture)
	load := returnedLoad(t, pkg.Func("param"))
	storage := NewStorage(proofs.NewSearchBudget(1000))
	allocations := testing.AllocsPerRun(100, func() {
		storage.unknown(proofs.EvidenceStorageNotLocal, load)
	})
	if allocations != 0 {
		t.Fatalf("silent give-up allocated %v times per run", allocations)
	}
}

func TestObservedBudgetReportsPositionAndInstruction(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/ssaflowtest", storageGiveUpFixture)
	load := returnedLoad(t, pkg.Func("escaped"))
	var at token.Pos
	var details map[string]string
	observer := func(_ string, pos token.Pos, got map[string]string) { at, details = pos, got }
	NewStorage(proofs.NewSearchBudget(1000).Observed(observer)).Content(load.X, load)
	if !at.IsValid() || details["instruction"] == "" {
		t.Fatalf("observer got position %v and details %v; want the blocking call", at, details)
	}
}
