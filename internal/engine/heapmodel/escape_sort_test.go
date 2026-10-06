package heapmodel

import (
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

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
