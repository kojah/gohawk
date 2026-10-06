package ssaflow

import (
	"slices"
	"strconv"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func BenchmarkStateWorklist(b *testing.B) {
	for _, shape := range []string{"chain", "tree"} {
		b.Run(shape, func(b *testing.B) {
			const count = 128
			next := make([][]int, count)
			for index := range count {
				if shape == "chain" && index+1 < count {
					next[index] = []int{index + 1}
				}
				if shape == "tree" {
					for _, child := range []int{2*index + 1, 2*index + 2} {
						if child < count {
							next[index] = append(next[index], child)
						}
					}
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				visited := 0
				WalkStates([]int{0}, func(state int) int { return state }, func(state int) ([]int, bool) {
					visited++
					return next[state], true
				})
				if visited != count {
					b.Fatalf("visited %d states, want %d", visited, count)
				}
			}
		})
	}
}

func TestStateWorklistPreservesOrderBudgetAndInputs(t *testing.T) {
	next := [][]int{{2, 3, 4}, {4, 5}, {6}, nil, {0}, nil, nil}
	for _, allowance := range []int{8, 9} {
		storage := []int{0, 1, -1, -1, -1, -1}
		var keyed, expanded []int
		budget := NewSearchBudget(allowance)
		WalkStatesWithin(storage[:2], func(state int) int {
			keyed = append(keyed, state)
			return state
		}, func(state int) ([]int, bool) {
			expanded = append(expanded, state)
			return next[state], true
		}, budget)
		wantKeys := []int{0, 1, 2, 3, 4, 4, 5, 6, 0}
		if !slices.Equal(keyed, wantKeys[:allowance]) || !slices.Equal(expanded, []int{0, 1, 2, 3, 4, 5, 6}) {
			t.Fatalf("allowance %d: keys %v, expansions %v", allowance, keyed, expanded)
		}
		if budget.Exhausted() != (allowance == 8) {
			t.Fatalf("allowance %d: unexpected cutoff", allowance)
		}
		if !slices.Equal(storage, []int{0, 1, -1, -1, -1, -1}) || !slices.Equal(next[0], []int{2, 3, 4}) {
			t.Fatal("worklist changed caller-owned initial or successor storage")
		}
	}
}

func BenchmarkReachabilityQueue(b *testing.B) {
	for _, count := range []int{16, 128} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			blocks := make([]*ssa.BasicBlock, count)
			for index := range blocks {
				blocks[index] = &ssa.BasicBlock{Index: index}
			}
			for index := range count - 1 {
				blocks[index].Succs = []*ssa.BasicBlock{blocks[index+1]}
			}
			b.ReportAllocs()
			for b.Loop() {
				if !blockReachableFromWithin(blocks[:1], blocks[count-1], nil) {
					b.Fatal("lost reachable target")
				}
			}
		})
	}
}

func BenchmarkPathGuardKey(b *testing.B) {
	for _, count := range []int{0, 1, 8, 32} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			guards := make(PathGuards, count)
			for index := range guards {
				guards[index] = PathGuard{Identity: "value:" + strconv.Itoa(index), Value: index%2 == 0}
			}
			b.ReportAllocs()
			for b.Loop() {
				_ = guards.KeyWithin(nil)
			}
		})
	}
}

func TestPathGuardKeyBytesAndBudget(t *testing.T) {
	zero := NewSearchBudget(0)
	if got := PathGuards(nil).KeyWithin(zero); got != "" || zero.Exhausted() {
		t.Fatal("an empty key must not spend the visit allowance")
	}
	guards := PathGuards{{Identity: "value:%;=", Value: true}, {Identity: "", Value: false}}
	if got := guards.KeyWithin(nil); got != "value:%;==true;=false" {
		t.Fatalf("unexpected key %q", got)
	}
	if got := guards.KeyWithin(NewSearchBudget(1)); got != "" {
		t.Fatalf("partial key escaped cutoff: %q", got)
	}
	budget := NewSearchBudget(2)
	if got := guards.KeyWithin(budget); got != "value:%;==true;=false" || budget.Exhausted() {
		t.Fatalf("exact-budget key %q unavailable", got)
	}
	pool := NewSearchBudget(1)
	child := pool.Within(2)
	if got := guards.KeyWithin(child); got != "" || !child.PoolExhausted() {
		t.Fatalf("partial pool key %q escaped cutoff", got)
	}
	pool = NewSearchBudget(2)
	child = pool.Within(2)
	if got := guards.KeyWithin(child); got != "value:%;==true;=false" || child.Exhausted() || pool.Exhausted() {
		t.Fatalf("exact pool key %q unavailable", got)
	}
}

func TestPathGuardCapacityOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if got := pathGuardKeyCapacity(0, maxInt-5, true, false); got != maxInt {
		t.Fatalf("largest fitting key capacity %d", got)
	}
	for _, size := range []int{0, 1, maxInt - 4, maxInt} {
		if got := pathGuardKeyCapacity(size, maxInt-4, true, false); got != -1 {
			t.Fatalf("overflow from size %d returned %d", size, got)
		}
	}
	if got := pathGuardKeyCapacity(maxInt-5, 0, true, true); got != -1 {
		t.Fatalf("separator overflow returned %d", got)
	}
	if got := pathGuardKeyCapacity(-1, 0, false, false); got != -1 {
		t.Fatalf("overflow hint became available: %d", got)
	}
}

func TestReachabilityQueuePreservesRevisitChargesAndSeeds(t *testing.T) {
	left, right, target := &ssa.BasicBlock{}, &ssa.BasicBlock{}, &ssa.BasicBlock{}
	left.Succs = []*ssa.BasicBlock{right}
	right.Succs = []*ssa.BasicBlock{left, target}
	sentinel := &ssa.BasicBlock{}
	storage := []*ssa.BasicBlock{left, right, sentinel, sentinel}
	seeds := storage[:2]
	// Three edges and five queued visits, including two revisits, cost eight.
	cut := NewSearchBudget(7)
	if blockReachableFromWithin(seeds, target, cut) || !cut.Exhausted() {
		t.Fatal("a partial visit allowance established reachability")
	}
	exact := NewSearchBudget(8)
	if !blockReachableFromWithin(seeds, target, exact) || exact.Exhausted() {
		t.Fatal("the exact visit allowance lost reachability")
	}
	if storage[0] != left || storage[1] != right || storage[2] != sentinel || storage[3] != sentinel {
		t.Fatal("queue growth changed the source successor backing array")
	}
}
