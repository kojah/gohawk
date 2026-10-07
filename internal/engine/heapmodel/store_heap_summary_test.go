package heapmodel

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// The projection names only what a caller can name: a returned fresh object
// with the parameter's field stored in it, a parameter stored into a global,
// a field the function read, and nothing about the locals in between.
func TestProjectHeap(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heapprobe", `package heapprobe
import "io"
type File struct{ fd int }
func (f *File) Close() error { return nil }
type Pair struct{ First, Second *File }
type Journal struct{ file *File; log io.Writer }
var registry []*Pair
var keep *File
func adopt(p *Pair, w io.Writer) (*Journal, error) {
	j := &Journal{file: p.First, log: w}
	registry = append(registry, p)
	_ = p.Second.Close()
	return j, nil
}
func keepFirst(p *Pair) { keep = p.First }
func swap(p *Pair, other *File, pick bool) { if pick { p.First = other } }
func opaque(p *Pair, f func(*Pair)) { f(p) }
func readOnly(p *Pair) bool { return p.First != nil }
`)
	for name, want := range map[string][]string{
		"adopt": {
			"edge G:heapprobe.registry -> fresh(call#",
			"edge G:heapprobe.registry/index:* -> P0 may",
			"edge R0 -> fresh(new#",
			"edge R0/field:0 -> P0/field:0 must",
			"edge R0/field:1 -> P1 must",
			"effect P0 escaped field every",
			"read P0/field:0",
			"read P0/field:1",
		},
		"keepFirst": {"edge G:heapprobe.keep -> P0/field:0 must", "effect P0/field:0 escaped global every"},
		"swap":      {"edge P0/field:0 -> P1 may"},
		"opaque":    {"effect P0 escaped call every", "truncated P0"},
		"readOnly":  {"read P0/field:0"},
	} {
		t.Run(name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(name))
			if !ok {
				t.Fatal("projection unavailable")
			}
			rendered := summary.String()
			for _, line := range want {
				if !strings.Contains(rendered, line) {
					t.Errorf("projection lacks %q:\n%s", line, rendered)
				}
			}
			if name == "readOnly" && (len(summary.Edges) != 0 || len(summary.Effects) != 0 || len(summary.Truncated) != 0) {
				t.Errorf("read-only function should project nothing but reads:\n%s", rendered)
			}
		})
	}
}

// An instantiation wrapper calls its generic origin; that call is not
// recursion, so the wrapper returns what the origin returns.
func TestProjectHeapInstantiationWrapper(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "genericprobe", `package genericprobe
type File struct{ fd int }
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
func use(f *File) *File { return must(f, nil) }
`)
	var wrapper *ssa.Function
	for _, block := range pkg.Func("use").Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok {
				if callee := call.Call.StaticCallee(); callee != nil && callee.Origin() != nil {
					wrapper = callee
				}
			}
		}
	}
	if wrapper == nil {
		t.Fatal("no instantiation of must")
	}
	summary, ok := ProjectHeap(wrapper)
	if !ok || !strings.Contains(summary.String(), "edge R0 -> P0 must") {
		t.Fatalf("wrapper summary = %v (ok %t), want the result to be its first parameter", summary.String(), ok)
	}
}

// A callee another analyzer is projecting still yields its summary: the
// caller projects it privately rather than treating the contention as a call
// cycle and getting none.
func TestOnDemandProjectionDuringAnotherProjection(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "contended", `package contended
type File struct{ fd int }
func keep(f *File) *File { return f }
`)
	function := pkg.Func("keep")
	heapSummaries.Lock()
	heapSummaries.entries[function] = heapEntry{state: heapEntryProjecting}
	heapSummaries.Unlock()
	t.Cleanup(func() {
		heapSummaries.Lock()
		delete(heapSummaries.entries, function)
		heapSummaries.Unlock()
	})
	summary, ok := projectHeapOnDemand(function)
	if !ok || !strings.Contains(summary.String(), "edge R0 -> P0 must") {
		t.Fatalf("contended projection = %v (ok %t), want the callee's own summary", summary.String(), ok)
	}
}

func TestProjectHeapNamedSlotDepthBoundary(t *testing.T) {
	atLimit := strings.Repeat("Next.", SummaryPaths-1) + "Value"
	beyond := strings.Repeat("Next.", SummaryPaths) + "Value"
	pkg := ssaflowtest.BuildPackage(t, "slotdepth", `package slotdepth
 type File struct{fd int}
 type Node struct{Next *Node;Value *File}
 var keep *File
 func limit(p *Node,v *File){keep=p.`+atLimit+`;p.`+atLimit+`=v}
 func beyond(p *Node,v *File){keep=p.`+beyond+`;p.`+beyond+`=v}
 func history(p *Node,v *File){p.`+atLimit+`=v;p.`+atLimit+`=nil}
 `)

	for _, test := range []struct {
		name   string
		depth  int
		edge   string
		escape bool
	}{
		{"limit", SummaryPaths, "must", true}, {"beyond", SummaryPaths + 1, "", false}, {"history", SummaryPaths, "may", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(test.name))
			if !ok {
				t.Fatal("projection unavailable")
			}
			rendered := summary.String()
			path := strings.Repeat("field:0/", test.depth-1) + "field:1"
			prefix := "edge P0/" + path + " -> P1 "
			if test.edge == "" {
				if strings.Contains(rendered, prefix) {
					t.Fatalf("over-depth source slot published:\n%s", rendered)
				}
			} else if !strings.Contains(rendered, prefix+test.edge) {
				t.Fatalf("missing state/history edge:\n%s", rendered)
			}
			effect := "effect P0/" + path + " escaped global every"
			if strings.Contains(rendered, effect) != test.escape {
				t.Fatalf("escape depth boundary changed:\n%s", rendered)
			}
			// Forwarded value targets have their own existing naming policy. The
			// shared source/escape slot bound must not silently truncate those targets.
			if test.name != "history" && !strings.Contains(rendered, "edge G:slotdepth.keep -> P0/"+path+" must") {
				t.Fatalf("forwarded target changed:\n%s", rendered)
			}
		})
	}
}

// A summarized callee is applied by substitution: a local the summary does
// not mention keeps its content across the call, an edge from a parameter
// field to another parameter is seen in the caller, and a truncated
// parameter is forgotten beneath. An unsummarized callee still forgets
// everything foreign.
func TestApplyHeapSummary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "applyprobe", `package applyprobe
type box struct{ value *int; other *int }
func observe(a, b any) {}
func summarized(p *box, q *int)
func truncating(p *box)
func unknownCallee(p *box)
func keepsLocal(a *int, p *box) { x := box{value: a}; summarized(p, a); observe(x.value, a) }
func seesEdge(a *int, p *box) { summarized(p, a); observe(p.value, a) }
func seesEdgeOther(a, b *int, p *box) { summarized(p, a); observe(p.value, b) }
func forgetsTruncated(a *int, p *box) { p.value = a; truncating(p); observe(p.value, a) }
func keepsUntouched(a *int, p *box, q *box) { q.value = a; summarized(p, a); observe(q.value, a) }
func keepsUnhanded(a *int, p *box, q *box) { q.value = a; unknownCallee(p); observe(q.value, a) }
func forgetsHanded(a *int, p *box, q *box) { q.value = a; unknownCallee(q); observe(q.value, a) }
`)
	RegisterHeapSummary(pkg.Func("summarized"), HeapSummary{
		Edges: []HeapEdge{{
			From: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 0}, Path: "field:0"},
			To:   HeapTarget{Kind: HeapTargetSlot, Slot: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: 1}}},
			Must: true,
		}},
	})
	RegisterHeapSummary(pkg.Func("truncating"), HeapSummary{
		Truncated: []HeapSlot{{Root: HeapRoot{Kind: HeapParameter, Index: 0}}},
	})
	for name, want := range map[string]bool{
		"keepsLocal": true, "seesEdge": true, "seesEdgeOther": false, "forgetsTruncated": false, "keepsUntouched": true,
		// An unresolved call reaches what it was handed and nothing this
		// function never let out: under the structural contract two
		// parameters are distinct objects.
		"keepsUnhanded": true, "forgetsHanded": false,
	} {
		t.Run(name, func(t *testing.T) {
			call := heapObservation(t, pkg.Func(name))
			left, right := unwrapInterface(call.Common().Args[0]), unwrapInterface(call.Common().Args[1])
			graph := regionsOfFunction(call.Parent())
			if got := graph.mustSame(left, right); got != want {
				t.Fatalf("mustSame = %t, want %t\n%s", got, want, RenderRegions(call.Parent()))
			}
			if name != "keepsUnhanded" && name != "forgetsHanded" && !strings.Contains(RenderRegions(call.Parent()), "//   applied ") {
				t.Fatalf("the dump must record the applied summary:\n%s", RenderRegions(call.Parent()))
			}
		})
	}
}

// A returned field address and a returned field value have opposite nilness
// when the field is unwritten. The summary must keep that distinction when
// it substitutes the receiver's field into the caller.
func TestReturnedFieldAddress(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "addressprobe", `package addressprobe
type box struct{ embedded int; pointer *int }
func address(b *box) *int { return &b.embedded }
func content(b *box) *int { return b.pointer }
func observe(a, b *int) {}
func caller() { var b box; observe(address(&b), content(&b)) }
`)
	address, ok := ProjectHeap(pkg.Func("address"))
	if !ok || !strings.Contains(address.String(), "edge R0 -> &P0/field:0 must") {
		t.Fatalf("field address summary = %s, want a must address edge", address.String())
	}
	call := heapObservation(t, pkg.Func("caller"))
	graph := regionsOfFunction(call.Parent())
	if graph.contentIsNil(call.Common().Args[0], nil, call) {
		t.Fatal("returned address was mistaken for nil field content")
	}
	if !graph.contentIsNil(call.Common().Args[1], nil, call) {
		t.Fatal("returned nil field content was not proven nil")
	}
}

// Dispatch identity and capture identity are separate from the callee's
// effects. Unknown alternatives must not acquire an unconditional write.
func TestHeapCallBindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heapbindings", `package heapbindings
type box struct { value *int }
type setter interface { Set(*int) }
func (b *box) Set(p *int) { b.value = p }
func observe(a,b any) {}
func opaque(*box)
func exactClosure(p *box, q *int) { func() { p.value=q }(); observe(p.value,q) }
func conditionalClosure(p *box, q *int, yes bool) { func() { if yes { p.value=q } }(); observe(p.value,q) }
func opaqueClosure(p *box, q *int) { func() { opaque(p) }(); observe(p.value,q) }
func exactInterface(p *box, q *int) { var s setter=p; s.Set(q); observe(p.value,q) }
func ambiguousInterface(p,r *box, q *int, yes bool) { var s setter=p; if yes { s=r }; s.Set(q); observe(p.value,q) }
func foreignInterface(s setter,p *box,q *int) { s.Set(q); observe(p.value,q) }
func changedCapture(p,r *box,q *int) { f:=func() { p.value=q }; p=r; f(); observe(r.value,q) }
func uncertainCapture(p,r *box,q *int, yes bool) { f:=func() { p.value=q }; if yes { p=r }; f(); observe(p.value,q) }
func writesCapture(p,r *box,q *int) { func() { p=r; p.value=q }(); observe(r.value,q) }
func closureArgument(p *box,q *int) { func(v *int) { p.value=v }(q); observe(p.value,q) }
func closureResult(p *box) { r:=func() *box { return p }(); observe(r,p) }
func recursiveCapture(p *box,q *int) { var f func(); f=func() { f(); p.value=q }; f(); observe(p.value,q) }
func nestedPointer(p *box,q *int) { pp:=&p; func() { (*pp).value=q }(); observe(p.value,q) }
func deferredExact(p *box,q *int) { defer func() { p.value=q }() }
func deferredCapture(p,r *box,q *int) { defer func() { p.value=q }(); p=r }
func deferredConditional(p *box,q *int, yes bool) { if yes { defer func() { p.value=q }() } }
func deferredOrder(p *box,a,b *int) { defer func() { p.value=a }(); defer func() { p.value=b }() }
func deferredLoop(p *box,q *int,n int) { for i:=0;i<n;i++ { defer func() { p.value=q }() } }
func directDeferredConditional(p *box,q *int, yes bool) { if yes { defer p.Set(q) } }
func directDeferredOrder(p *box,a,b *int) { defer p.Set(a); defer p.Set(b) }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"exactClosure", true},
		{"conditionalClosure", false},
		{"opaqueClosure", false},
		{"exactInterface", true},
		{"ambiguousInterface", false},
		{"foreignInterface", false},
		{"changedCapture", true},
		{"uncertainCapture", false},
		{"writesCapture", false},
		{"closureArgument", true},
		{"closureResult", true},
		{"recursiveCapture", false},
		{"nestedPointer", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var dump bytes.Buffer
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			before := map[*ssa.Function]string{}
			for _, child := range function.AnonFuncs {
				summary, _ := ProjectHeap(child)
				before[child] = summary.String()
				t.Log(summary.String())
			}
			call := heapObservation(t, function)
			left, right := unwrapInterface(call.Common().Args[0]), unwrapInterface(call.Common().Args[1])
			if got := regionsOfFunction(function).mustSame(left, right); got != test.want {
				t.Fatalf("mustSame = %t, want %t\n%s", got, test.want, RenderRegions(function))
			}
			for child, previous := range before {
				summary, _ := ProjectHeap(child)
				if summary.String() != previous {
					t.Fatal("call substitution mutated the shared callee summary")
				}
			}
		})
	}
	for _, test := range []struct {
		name   string
		want   string
		absent string
	}{
		{"deferredExact", "edge P0/field:0 -> P1 must", ""},
		{"deferredCapture", "", "edge P1/field:0 -> P2 must"},
		{"deferredConditional", "", "edge P0/field:0 -> P1 must"},
		{"deferredOrder", "edge P0/field:0 -> P1 must", "edge P0/field:0 -> P2 must"},
		{"deferredLoop", "", "edge P0/field:0 -> P1 must"},
		{"directDeferredConditional", "", "edge P0/field:0 -> P1 must"},
		{"directDeferredOrder", "edge P0/field:0 -> P1 must", "edge P0/field:0 -> P2 must"},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(test.name))
			if !ok {
				t.Fatal("summary unavailable")
			}
			rendered := summary.String()
			if test.want != "" && !strings.Contains(rendered, test.want) || test.absent != "" && strings.Contains(rendered, test.absent) {
				t.Fatalf("unexpected deferred summary:\n%s\n%s", rendered, RenderRegions(pkg.Func(test.name)))
			}
		})
	}
}

func TestHeapBindingPreservesDeclarationIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heapdeclarations", `package heapdeclarations
import "errors"
func identity[T any](p *T) *T { return p }
func generic(p *int) *int { return identity(p) }
func imported() error { return errors.New("failure") }
`)
	for _, name := range []string{"generic", "imported"} {
		call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name))[0]
		callee := call.Common().StaticCallee()
		if name == "generic" && callee.Origin() == nil {
			t.Fatal("fixture did not create a generic instantiation")
		}
		if name == "imported" && len(callee.Blocks) != 0 {
			t.Fatal("fixture did not create an imported declaration")
		}
		binding, reason := resolveHeapCall(call.Common(), call)
		if reason != CallSummaryApplied || binding.callee != callee || len(binding.args) != 1 {
			t.Fatalf("%s: lost concrete callee or signature binding: %+v %s", name, binding, reason)
		}
	}
}

func TestProjectHeapAggregateCopies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "copies", `package copies
type File struct{ fd int }
type View struct { Out *File; Level int }
type Context struct { Value View }
func changeLevel(v View) View { v.Level = 1; return v }
func extract(c Context) View { return c.Value }
func replace(v View, f *File) View { v.Out = f; return v }
func snapshot(v View, f *File) View { old := v; v.Out = f; old.Level = 1; return old }
func opaque(v View, mutate func(*View)) View { mutate(&v); return v }
func priorSnapshot(v View, mutate func(*View)) View { old := v; mutate(&v); old.Level = 1; return old }
func zero() View { v := View{}; v.Level = 1; return v }
`)
	for _, test := range []struct {
		name   string
		want   string
		absent string
	}{
		{"changeLevel", "edge R0/field:0 -> P0/field:0 must", ""},
		{"extract", "edge R0/field:0 -> P0/field:0/field:0 must", ""},
		{"replace", "edge R0/field:0 -> P1 must", "edge R0/field:0 -> P0/field:0"},
		{"snapshot", "edge R0/field:0 -> P0/field:0 must", "edge R0/field:0 -> P1"},
		{"opaque", "", "edge R0/field:0 -> P0/field:0 must"},
		// The existing graph conservatively clobbers the shared external
		// origin even for this earlier value copy. Export preserves that gap.
		{"priorSnapshot", "", "edge R0/field:0 -> P0/field:0 must"},
		{"zero", "", "edge R0/field:0 -> P0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(test.name))
			if !ok {
				t.Fatal("projection unavailable")
			}
			rendered := summary.String()
			if test.want != "" && !strings.Contains(rendered, test.want) {
				t.Errorf("projection lacks %q:\n%s", test.want, rendered)
			}
			if test.absent != "" && strings.Contains(rendered, test.absent) {
				t.Errorf("projection incorrectly contains %q:\n%s", test.absent, rendered)
			}
		})
	}
}

func TestProjectHeapAggregateCopyBounds(t *testing.T) {
	var wideFields strings.Builder
	// Give each field a distinct name without making the fixture depend on a
	// manually maintained list that could stop exercising the slot budget.
	for index := range SummarySlots + 1 {
		wideFields.WriteString("F" + strconv.Itoa(index) + " *File;")
	}
	pkg := ssaflowtest.BuildPackage(t, "copybounds", `package copybounds
type File struct{ fd int }
type Wide struct { Level int; `+wideFields.String()+` }
type Deep struct { Nested struct { Nested struct { Nested struct { Out *File } } }; Level int }
type Array struct { Values [2]*File; Level int }
func wide(v Wide) Wide { v.Level = 1; return v }
func deep(v Deep) Deep { v.Level = 1; return v }
func array(v Array) Array { v.Level = 1; return v }
`)
	for _, name := range []string{"wide", "deep", "array"} {
		t.Run(name, func(t *testing.T) {
			summary, ok := ProjectHeap(pkg.Func(name))
			if !ok || len(summary.Truncated) == 0 {
				t.Fatalf("bounded projection must mark omitted content: ok %t\n%s", ok, summary.String())
			}
			if len(summary.Edges) > SummarySlots+1 {
				t.Fatalf("projection exceeds result root plus slot budget:\n%s", summary.String())
			}
		})
	}
}

func TestProjectHeapConditionalAggregateCopies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "conditionalcopies", `package conditionalcopies
type File struct{ fd int }
type View struct { Out *File; Level int }
func conditional(v View, change bool) View { if !change { return v }; v.Level = 1; return v }
func replacement(v View, f *File, change bool) View { if !change { return v }; v.Out = f; return v }
func observe(a, b *File) {}
func preserved(f *File, change bool) { v := conditional(View{Out: f}, change); observe(v.Out, f) }
func replaced(f, other *File, change bool) { v := replacement(View{Out: f}, other, change); observe(v.Out, f) }
`)
	for _, name := range []string{"conditional", "replacement"} {
		summary, ok := ProjectHeap(pkg.Func(name))
		if !ok {
			t.Fatalf("%s projection unavailable", name)
		}
		RegisterHeapSummary(pkg.Func(name), summary)
		preserved := strings.Contains(summary.String(), "edge R0/field:0 -> P0/field:0 must")
		if preserved != (name == "conditional") {
			t.Fatalf("%s preserved writer = %t:\n%s", name, preserved, summary.String())
		}
	}
	for name, want := range map[string]bool{"preserved": true, "replaced": false} {
		call := heapObservation(t, pkg.Func(name))
		graph := regionsOfFunction(call.Parent())
		if got := graph.mustSame(call.Common().Args[0], call.Common().Args[1]); got != want {
			t.Errorf("%s writer identity = %t, want %t:\n%s", name, got, want, RenderRegions(call.Parent()))
		}
	}
}

func TestBoundedRequirements(t *testing.T) {
	makeCandidates := func() []requirementCandidate {
		candidates := make([]requirementCandidate, heapRequirementProofLimit+1)
		for index := range candidates {
			candidates[index] = requirementCandidate{
				key: requirementKey{slot: slot{region: &region{kind: regionExternal, serial: index}}},
				requirement: HeapRequirement{
					Slot: HeapSlot{Root: HeapRoot{Kind: HeapParameter, Index: index}}, Kind: HeapRequiresNonNil,
				},
			}
		}
		return candidates
	}
	t.Run("published limit", func(t *testing.T) {
		calls := 0
		got := boundedRequirements(makeCandidates(), func(requirementKey) bool { calls++; return true })
		if len(got) != heapRequirementLimit || calls != heapRequirementLimit {
			t.Fatalf("got %d requirements after %d proofs, want %d of each", len(got), calls, heapRequirementLimit)
		}
		for index, requirement := range got {
			if requirement.Slot.Root.Index != index {
				t.Fatalf("requirement %d names parameter %d, want %d", index, requirement.Slot.Root.Index, index)
			}
		}
	})
	t.Run("proof-work limit", func(t *testing.T) {
		calls := 0
		got := boundedRequirements(makeCandidates(), func(key requirementKey) bool {
			calls++
			return key.slot.region.serial == heapRequirementProofLimit
		})
		if calls != heapRequirementProofLimit || len(got) != 0 {
			t.Fatalf("got %d requirements after %d proofs, want none after %d", len(got), calls, heapRequirementProofLimit)
		}
	})
}

func TestEveryReturnRequirementDeclinesBudgetCut(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "requirementbudget", `package requirementbudget
func mark() {}
func exactEverywhere(flag bool) { if flag { mark(); return }; mark() }
`)
	function := pkg.Func("exactEverywhere")
	calls := func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		return common != nil && ssaflow.CallName(common) == "mark"
	}
	if onEveryReturn(function, proofs.NewSearchBudget(0), calls) {
		t.Fatal("an exhausted path search established a requirement")
	}
	if !onEveryReturn(function, proofs.NewSearchBudget(100), calls) {
		t.Fatal("a bounded search missed calls on every return")
	}
}
