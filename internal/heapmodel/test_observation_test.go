package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func heapObservation(tb testing.TB, function *ssa.Function) *ssa.Call {
	tb.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if ok && ssaflow.CallName(call.Common()) == "observe" {
				return call
			}
		}
	}
	tb.Fatal("no observation")
	return nil
}
