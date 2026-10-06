package ssaflow

import (
	"strconv"
	"testing"

	"golang.org/x/tools/go/ssa"
)

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
