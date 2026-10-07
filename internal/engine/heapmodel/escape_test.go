package heapmodel

import (
	"bytes"
	"slices"
	"sort"
	"strconv"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestEscapeQueryBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "escapequery", `package escapequery
import "unsafe"
type box struct { p *int }
var saved *int
func opaque(*int)
func local() { p := new(int); *p = 1 }
func returned() *int { p := new(int); return p }
func conditional(b bool) *int { p := new(int); if b { return p }; return nil }
func global() { p := new(int); q := p; saved = q }
func field(o *box) { p := new(int); o.p = p }
func sent(c chan *int) { p := new(int); c <- p }
func selected(c chan *int) { p := new(int); select { case c <- p: default: } }
func hidden() { p := new(int); opaque(p) }
func closure() func() int { p := new(int); return func() int { return *p } }
func started() { p := new(int); go func() { *p = 2 }() }
func nested() *box { p := new(int); return &box{p:p} }
func interfaceResult() any { p := new(int); return p }
func unrelated() *int { p := new(int); *p = 1; return new(int) }
func parameter(p *int) { *p = 1 }
func unsafePointerResult() unsafe.Pointer { p:=new(int); return unsafe.Pointer(p) }
func unsafeStringResult() string { p:=new(byte); return unsafe.String(p,1) }
func ordinaryUnsafeString(p *byte) string { return "copy" }
func misleadingName() string { p:=new(byte); return ordinaryUnsafeString(p) }
`)
	for _, test := range []struct {
		name    string
		outcome EscapeOutcome
		kind    EscapeDestination
	}{
		{"local", EscapeLocal, 0},
		{"returned", EscapeObserved, EscapeToResult},
		{"conditional", EscapeObserved, EscapeToResult},
		{"global", EscapeObserved, EscapeToGlobal},
		{"field", EscapeObserved, EscapeToField},
		{"sent", EscapeObserved, EscapeToChannel},
		{"selected", EscapeObserved, EscapeToChannel},
		{"hidden", EscapeUnknown, EscapeToCall},
		{"closure", EscapeObserved, EscapeToResult},
		{"started", EscapeObserved, EscapeToGoroutine},
		{"nested", EscapeObserved, EscapeToResult},
		{"interfaceResult", EscapeObserved, EscapeToResult},
		{"unrelated", EscapeLocal, 0},
		{"parameter", EscapeUnknown, 0},
		{"unsafePointerResult", EscapeUnknown, 0},
		{"unsafeStringResult", EscapeUnknown, 0},
		{"misleadingName", EscapeLocal, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var value ssa.Value
			if test.name == "parameter" {
				value = function.Params[0]
			} else {
				value = ssaflow.InstructionsOf[*ssa.Alloc](function)[0]
			}
			got := QueryEscape(value, EscapeFunction)
			if got.Outcome != test.outcome {
				var dump bytes.Buffer
				if _, err := function.WriteTo(&dump); err != nil {
					t.Fatal(err)
				}
				t.Fatalf("outcome %v, want %v: %+v\n%s\n%s", got.Outcome, test.outcome, got, dump.String(), RenderRegions(function))
			}
			if test.kind == 0 {
				return
			}
			for _, event := range got.Events {
				if event.Destination == test.kind && event.Instruction != nil {
					return
				}
			}
			t.Fatalf("missing destination %v: %+v", test.kind, got)
		})
	}
}

func TestEscapeScopesAndUncertainty(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "escapescopes", `package escapescopes
type box struct { p *int }
func source() (int, error)
func loop(n int) error {
 for i:=0; i<n; i++ { _,err:=source(); if err!=nil { return err }; p:=new(box); p.p=new(int) }
 return nil
}
func choice(b bool) *int { p,q:=new(int),new(int); if b { return p }; return q }
func merged(b bool) *int { p:=new(int); if b { p=new(int) }; return p }
func external(*int)
`)
	function := pkg.Func("loop")
	var dump bytes.Buffer
	if _, err := function.WriteTo(&dump); err != nil {
		t.Fatal(err)
	}
	t.Log(dump.String())
	value := ssaflow.InstructionsOf[*ssa.Alloc](function)[0]
	if got := QueryEscape(value, EscapeFunction); got.Outcome != EscapeUnknown || got.Reason != EscapeReachabilityUnknown {
		t.Fatalf("unrelated opaque result must not prove whole-function confinement: %+v", got)
	}
	if got := QueryEscape(value, EscapeBody); got.Outcome != EscapeLocal || got.Scope != EscapeBody {
		t.Fatalf("body-only scope lost local loop container: %+v", got)
	}
	merged := ssaflow.InstructionsOf[*ssa.Return](pkg.Func("merged"))[0].Results[0]
	if got := QueryEscape(merged, EscapeFunction); got.Outcome != EscapeUnknown || got.Reason != EscapeIdentityUnknown {
		t.Fatalf("merged allocation identities must stay unknown: %+v", got)
	}
	for _, value := range []ssa.Value{nil, pkg.Func("external").Params[0]} {
		if got := QueryEscape(value, EscapeFunction); got.Outcome != EscapeUnknown {
			t.Fatalf("unavailable graph became confinement: %+v", got)
		}
	}
}

func TestEscapeReachabilityCutoffs(t *testing.T) {
	target := slot{region: &region{kind: regionSite}}
	graph := &regionGraph{}
	state := newRegionState()
	if _, complete := graph.escapeReachability(state, pointees{target: false}, target, proofs.NewSearchBudget(0)); complete {
		t.Fatal("exhausted query claimed complete reachability")
	}
	unknown := slot{region: &region{kind: regionUnknown}}
	for range 32 {
		found, complete := graph.escapeReachability(state, pointees{target: false, unknown: false}, target, proofs.NewSearchBudget(proofs.QueryBudget))
		if !found || complete {
			t.Fatal("known witness hid unknown sibling contents")
		}
	}
	if _, complete := graph.escapeReachability(state, pointees{unknown: false}, target, proofs.NewSearchBudget(proofs.QueryBudget)); complete {
		t.Fatal("unknown contents proved non-escape")
	}
	owner := slot{region: &region{kind: regionSite}}
	state.contents[owner] = pointees{owner: false}
	if found, complete := graph.escapeReachability(state, pointees{owner: false}, target, proofs.NewSearchBudget(proofs.QueryBudget)); found || !complete {
		t.Fatal("exact local cycle did not terminate with a negative answer")
	}
	state.backing[owner] = &region{kind: regionSnapshot}
	if _, complete := graph.escapeReachability(state, pointees{owner: false}, target, proofs.NewSearchBudget(proofs.QueryBudget)); complete {
		t.Fatal("unresolved backing copy proved non-escape")
	}
}

func TestEscapeReasonCodes(t *testing.T) {
	seen := map[string]bool{}
	for reason := EscapeUnavailable; reason <= EscapeRepresentationUnknown; reason++ {
		code := reason.String()
		if seen[code] || code == "invalid-escape-reason" {
			t.Fatalf("unhandled or duplicated reason: %d", reason)
		}
		seen[code] = true
	}
}

// Retain the legacy ordering as a compatibility oracle, especially for SSA
// instructions that share a source position and require block-order ties.
func legacyEscapeEventLess(a, b EscapeEvent) bool {
	if a.Instruction.Pos() != b.Instruction.Pos() {
		return a.Instruction.Pos() < b.Instruction.Pos()
	}
	if a.Instruction.Block().Index != b.Instruction.Block().Index {
		return a.Instruction.Block().Index < b.Instruction.Block().Index
	}
	if a.Instruction != b.Instruction {
		for _, instruction := range a.Instruction.Block().Instrs {
			if instruction == a.Instruction || instruction == b.Instruction {
				return instruction == a.Instruction
			}
		}
	}
	return a.Destination < b.Destination
}

func escapeSortEvents(tb testing.TB) []EscapeEvent {
	tb.Helper()
	pkg := ssaflowtest.BuildPackage(tb, "escapeordering", `package escapeordering
func pair() (int, int)
func events(flag bool) int {
 a, b := pair()
 if flag { a++; b++ } else { a--; b-- }
 return a+b
}
`)
	var events []EscapeEvent
	for _, block := range pkg.Func("events").Blocks {
		for _, instruction := range block.Instrs {
			for destination := EscapeToResult; destination <= EscapeToOpaqueRepresentation; destination++ {
				events = append(events, EscapeEvent{Instruction: instruction, Destination: destination})
			}
		}
	}
	return events
}

func TestEscapeEventSortCompatibility(t *testing.T) {
	events := escapeSortEvents(t)
	positionTie := false
	for _, left := range events {
		for _, right := range events {
			if left.Instruction != right.Instruction && left.Instruction.Block() == right.Instruction.Block() &&
				left.Instruction.Pos() == right.Instruction.Pos() {
				positionTie = true
			}
		}
	}
	if !positionTie {
		t.Fatal("fixture must exercise instruction-order ties within one block")
	}
	sortCompatible(t, events, legacyEscapeEventLess, compareEscapeEvents)
	for _, size := range []int{0, 1, 8, 32} {
		sortCompatible(t, events[:size], legacyEscapeEventLess, compareEscapeEvents)
	}
}

func BenchmarkEscapeEventSort(b *testing.B) {
	events := escapeSortEvents(b)
	slices.Reverse(events)
	for _, size := range []int{0, 1, 8, 32} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			for _, typed := range []bool{false, true} {
				name := "reflective"
				if typed {
					name = "typed"
				}
				b.Run(name, func(b *testing.B) {
					work := make([]EscapeEvent, size)
					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						copy(work, events[:size])
						if typed {
							slices.SortFunc(work, compareEscapeEvents)
						} else {
							sort.Slice(work, func(i, j int) bool { return legacyEscapeEventLess(work[i], work[j]) })
						}
					}
				})
			}
		})
	}
}

func BenchmarkEscapeQueryConfined(b *testing.B) {
	pkg := ssaflowtest.BuildPackage(b, "confinedquery", `package confinedquery
func local() { p := new(int); *p = 1 }
`)
	value := ssaflow.InstructionsOf[*ssa.Alloc](pkg.Func("local"))[0]
	proof := QueryEscape(value, EscapeFunction)
	if proof.Outcome != EscapeLocal || proof.Reason != EscapeConfined || proof.Events != nil {
		b.Fatalf("fixture must prove confinement without events: %+v", proof)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		QueryEscape(value, EscapeFunction)
	}
}
