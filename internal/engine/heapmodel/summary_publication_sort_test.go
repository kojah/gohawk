package heapmodel

import (
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"testing"
)

// Legacy less relations deliberately retain the pre-change publication order,
// including fields omitted from tie breaking. These are compatibility oracles,
// not an alternative production decision path.
func legacySlotLess(left, right HeapSlot) bool {
	if left.Root != right.Root {
		if left.Root.Kind != right.Root.Kind {
			return left.Root.Kind < right.Root.Kind
		}
		if left.Root.Index != right.Root.Index {
			return left.Root.Index < right.Root.Index
		}
		if left.Root.Package != right.Root.Package {
			return left.Root.Package < right.Root.Package
		}
		return left.Root.Name < right.Root.Name
	}
	return left.Path < right.Path
}

func legacyEdgeLess(left, right HeapEdge) bool {
	if left.From != right.From {
		return legacySlotLess(left.From, right.From)
	}
	if left.To.Kind != right.To.Kind {
		return left.To.Kind < right.To.Kind
	}
	if left.To.Slot != right.To.Slot {
		return legacySlotLess(left.To.Slot, right.To.Slot)
	}
	return left.To.Origin < right.To.Origin
}

func legacyEffectLess(left, right HeapEffect) bool {
	if left.Slot != right.Slot {
		return legacySlotLess(left.Slot, right.Slot)
	}
	if left.Escape != right.Escape {
		return left.Escape < right.Escape
	}
	return left.Release < right.Release
}

func legacyRequirementLess(left, right requirementCandidate) bool {
	if left.requirement.Slot != right.requirement.Slot {
		return legacySlotLess(left.requirement.Slot, right.requirement.Slot)
	}
	if left.requirement.Kind != right.requirement.Kind {
		return left.requirement.Kind < right.requirement.Kind
	}
	if left.requirement.Method != right.requirement.Method {
		return left.requirement.Method < right.requirement.Method
	}
	if left.key.slot.region.serial != right.key.slot.region.serial {
		return left.key.slot.region.serial < right.key.slot.region.serial
	}
	return left.key.slot.path < right.key.slot.path
}

func legacyHoldLess(left, right HeapHold) bool {
	if left.Result != right.Result {
		return left.Result < right.Result
	}
	return left.Parameter < right.Parameter
}

func sortCompatible[T comparable](t *testing.T, values []T, less func(T, T) bool, compare func(T, T) int) {
	t.Helper()
	random := rand.New(rand.NewPCG(17, 29))
	for iteration := range 100 {
		input := slices.Clone(values)
		random.Shuffle(len(input), func(i, j int) { input[i], input[j] = input[j], input[i] })
		want, got := slices.Clone(input), slices.Clone(input)
		sort.Slice(want, func(i, j int) bool { return less(want[i], want[j]) })
		slices.SortFunc(got, compare)
		if !slices.Equal(got, want) {
			t.Fatalf("publication order changed on shuffle %d", iteration)
		}
	}
}

func summarySortInputs(size int) ([]HeapHold, []HeapEdge, []HeapEffect, []requirementCandidate) {
	holds := make([]HeapHold, size)
	edges := make([]HeapEdge, size)
	effects := make([]HeapEffect, size)
	requirements := make([]requirementCandidate, size)
	for index := range size {
		at := HeapSlot{
			Root: HeapRoot{Kind: HeapRootKind(index % 4), Index: index % 3, Package: strconv.Itoa(index % 2), Name: strconv.Itoa(index % 3)},
			Path: strconv.Itoa(index % 2),
		}
		holds[index] = HeapHold{Result: index % 3, Parameter: index % 2, Must: index/6%2 == 0}
		edges[index] = HeapEdge{
			From: at, To: HeapTarget{Kind: HeapTargetKind(index % 5), Slot: at, Origin: strconv.Itoa(index % 2), Object: index},
			Must: index/6%2 == 0,
		}
		effects[index] = HeapEffect{Slot: at, Escape: HeapEscape(index % 3), Release: strconv.Itoa(index % 2), Every: index/12%2 == 0}
		requirements[index] = requirementCandidate{
			key:         requirementKey{slot: slot{region: &region{serial: index % 3}, path: strconv.Itoa(index % 2)}},
			requirement: HeapRequirement{Slot: at, Kind: HeapRequirementKind(index % 2), Method: strconv.Itoa(index % 3)},
		}
	}
	return holds, edges, effects, requirements
}

func TestSummarySortCompatibility(t *testing.T) {
	for _, size := range []int{0, 1, 8, 32, 128} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			holds, edges, effects, requirements := summarySortInputs(size)
			sortCompatible(t, holds, legacyHoldLess, compareHolds)
			sortCompatible(t, edges, legacyEdgeLess, compareEdges)
			sortCompatible(t, effects, legacyEffectLess, compareEffects)
			sortCompatible(t, requirements, legacyRequirementLess, compareRequirementCandidates)
			want := slices.Clone(requirements)
			sort.Slice(want, func(i, j int) bool { return legacyRequirementLess(want[i], want[j]) })
			var wantCalls, gotCalls []requirementKey
			var wantPublished []HeapRequirement
			for index, candidate := range want {
				if index >= heapRequirementProofLimit || len(wantPublished) == heapRequirementLimit {
					break
				}
				wantCalls = append(wantCalls, candidate.key)
				if candidate.key.slot.region.serial%2 == 0 {
					wantPublished = append(wantPublished, candidate.requirement)
				}
			}
			got := boundedRequirements(slices.Clone(requirements), func(key requirementKey) bool {
				gotCalls = append(gotCalls, key)
				return key.slot.region.serial%2 == 0
			})
			if !slices.Equal(gotCalls, wantCalls) || !slices.Equal(got, wantPublished) {
				t.Fatal("sorting changed bounded proof order or publication")
			}
		})
	}
}

func benchmarkSummarySort[T any](b *testing.B, values []T, less func(T, T) bool, compare func(T, T) int) {
	b.Helper()
	for _, typed := range []bool{false, true} {
		name := "reflective"
		if typed {
			name = "typed"
		}
		b.Run(name, func(b *testing.B) {
			work := make([]T, len(values))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				copy(work, values)
				if typed {
					slices.SortFunc(work, compare)
				} else {
					sort.Slice(work, func(i, j int) bool { return less(work[i], work[j]) })
				}
			}
		})
	}
}

func BenchmarkSummarySorts(b *testing.B) {
	for _, size := range []int{0, 1, 8, 32, 128} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			holds, edges, effects, requirements := summarySortInputs(size)
			b.Run("holds", func(b *testing.B) { benchmarkSummarySort(b, holds, legacyHoldLess, compareHolds) })
			b.Run("edges", func(b *testing.B) { benchmarkSummarySort(b, edges, legacyEdgeLess, compareEdges) })
			b.Run("effects", func(b *testing.B) { benchmarkSummarySort(b, effects, legacyEffectLess, compareEffects) })
			b.Run("requirements", func(b *testing.B) {
				benchmarkSummarySort(b, requirements, legacyRequirementLess, compareRequirementCandidates)
			})
		})
	}
}
