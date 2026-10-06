package deferinloop

import (
	"strconv"
	"testing"

	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"golang.org/x/tools/go/ssa"
)

func TestNonLiveDeferSuffixPreservesState(t *testing.T) {
	block := &ssa.BasicBlock{Instrs: make([]ssa.Instruction, 32)}
	for _, status := range []resourceStatus{resourceSettled, resourceUnknown} {
		for _, index := range []int{0, 1, len(block.Instrs), len(block.Instrs) + 1} {
			initial := deferFlowState{block: block, predecessor: block, index: index, status: status}
			want := initial
			want.index = max(index, len(block.Instrs))
			if got := advanceDeferState(nil, analysisTrace.Probe{}, initial, deferObligation{}); got != want {
				t.Fatalf("non-live suffix changed state: got=%+v want=%+v", got, want)
			}
		}
	}
}

func BenchmarkNonLiveDeferSuffix(b *testing.B) {
	for _, count := range []int{1, 32, 256, 1024} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			state := deferFlowState{block: &ssa.BasicBlock{Instrs: make([]ssa.Instruction, count)}, status: resourceSettled}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = advanceDeferState(nil, analysisTrace.Probe{}, state, deferObligation{})
			}
		})
	}
}
