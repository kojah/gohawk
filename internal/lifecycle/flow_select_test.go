package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestSelectedReceiveChannel(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
func mixed(receive <-chan int, send chan<- int) int {
	select {
	case <-receive: return 1
	case send <- 1: return 2
	default: return 3
	}
}
func ordinary(flag bool, channel <-chan int) int {
	if flag { return 1 }; return 2
}
func stringPair() (string, error) { return "", nil }
func stringComparison() int {
	value, _ := stringPair()
	if value == "x" { return 1 }; return 2
}
func boolPair() (bool, error) { return false, nil }
func boolComparison(flag bool) int {
	value, _ := boolPair()
	if value == flag { return 1 }; return 2
}
`)
	function := pkg.Func("mixed")
	var matched *ssa.BasicBlock
	for _, block := range function.Blocks {
		channel, ok := ssaflow.SelectedReceiveChannel(block)
		if !ok {
			continue
		}
		if matched != nil || channel != function.Params[0] {
			t.Fatalf("unexpected selected receive in block %d: %v", block.Index, channel)
		}
		matched = block
	}
	if matched == nil {
		t.Fatal("missing receive case")
	}
	// A shared successor cannot borrow one predecessor's selected operation.
	matched.Preds = append(matched.Preds, function.Blocks[0])
	if _, ok := ssaflow.SelectedReceiveChannel(matched); ok {
		t.Fatal("shared successor was treated as a selected receive")
	}
	if channel, ok := ssaflow.SelectedReceiveOnEdge(matched.Preds[0], matched); !ok || channel != function.Params[0] {
		t.Fatal("exact selected edge lost its receive at a shared successor")
	}
	for _, name := range []string{"ordinary", "stringComparison", "boolComparison"} {
		for _, block := range pkg.Func(name).Blocks {
			if _, ok := ssaflow.SelectedReceiveChannel(block); ok {
				t.Fatalf("%s branch was treated as a selected receive", name)
			}
		}
	}
	if _, ok := ssaflow.SelectedReceiveChannel(nil); ok {
		t.Fatal("nil block was treated as a selected receive")
	}
}

func TestUnownedReturnWithSelectedEdges(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
func start() {}
func cleanup() {}
func opaque() {}
func shared(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: cleanup() }
}
func conditional(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: if flag { cleanup() } }
}
func unknown(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: opaque() }
}
func abandoned(done, work <-chan int, flag bool) {
	start()
	select { case <-done: case <-work: }
}
func defaultArm(done, work <-chan int, flag bool) {
	start()
	select { case <-done: default: }
}
`)
	for _, test := range []struct {
		name                      string
		unowned, uncertainUnowned bool
	}{
		{"shared", false, false},
		{"conditional", true, true},
		{"unknown", true, false},
		{"abandoned", true, true},
		{"defaultArm", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			start := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				return ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "start"
			})
			exact := func(instruction ssa.Instruction) bool {
				return ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "cleanup"
			}
			uncertain := func(instruction ssa.Instruction) bool {
				return exact(instruction) || ssaflow.CallName(ssaflow.InstructionCall(instruction)) == "opaque"
			}
			edge := func(from, to *ssa.BasicBlock) bool {
				channel, selected := ssaflow.SelectedReceiveOnEdge(from, to)
				return selected && channel == function.Params[0]
			}
			if got := ssaflow.UnownedReturnWithEdges(start, exact, nil, edge); got != test.unowned {
				t.Errorf("after start = %v, want %v", got, test.unowned)
			}
			if got := ssaflow.UnownedReturnFromEntryWithEdges(function, exact, edge); got != test.unowned {
				t.Errorf("from entry = %v, want %v", got, test.unowned)
			}
			if got := ssaflow.UnownedReturnAssumingNonNilWithEdges(start, function.Params[0], uncertain, nil, edge); got != test.uncertainUnowned {
				t.Errorf("uncertain/non-nil = %v, want %v", got, test.uncertainUnowned)
			}
			if !ssaflow.UnownedReturn(start, uncertain, nil) {
				t.Error("ordinary instruction-only query borrowed an edge action")
			}
		})
	}
}
