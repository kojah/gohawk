package producerlifecycle

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestFallbackCalleeBindings(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "fallbackcallee", `package fallbackcallee
func worker(ch chan int, opaque func()) { opaque(); ch <- 1; ch <- 2 }
func genericWorker[T any](ch chan T, value T, opaque func()) { opaque(); ch <- value; ch <- value }
func direct(opaque func()) { ch := make(chan int); go worker(ch, opaque); <-ch }
func captured(opaque func()) { ch := make(chan int); go func(){ opaque(); ch <- 1; ch <- 2 }(); <-ch }
func generic(opaque func()) { ch := make(chan int); go genericWorker(ch, 1, opaque); <-ch }
func balanced(opaque func()) { ch := make(chan int); go genericWorker(ch, 1, opaque); <-ch; <-ch }
func dynamic(worker func(chan int)) { ch := make(chan int); go worker(ch); <-ch }
func selected(yes bool, opaque func()) {
 ch := make(chan int)
 worker := func(){ opaque(); ch <- 1; ch <- 2 }
 if yes { worker = func(){ opaque(); ch <- 3; ch <- 4 } }
 go worker(); <-ch
}
`)
	for _, test := range []struct {
		name  string
		count int
		state proofs.EvidenceState
	}{
		{"direct", 2, proofs.EvidenceProven},
		{"captured", 2, proofs.EvidenceProven},
		{"generic", 2, proofs.EvidenceProven},
		{"balanced", 2, proofs.EvidenceDisproven},
		{"dynamic", 0, proofs.EvidenceUnknown},
		{"selected", 0, proofs.EvidenceUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			if test.name == "generic" {
				logGenericWorker(t, function)
			}
			engine := concurrencyfacts.NewEngine()
			sends := producerSends(function, engine)
			if len(sends) != test.count {
				t.Fatalf("sends=%d; want %d", len(sends), test.count)
			}
			for _, send := range sends {
				if send.summarized {
					t.Fatal("opaque worker unexpectedly has complete summary")
				}
				if send.channel != ssaflow.InstructionsOf[*ssa.MakeChan](function)[0] {
					t.Fatal("callee channel was not mapped to caller")
				}
				if send.instruction.Parent() != ssacall.ResolvedCallee(send.spawn.Common()) {
					t.Fatal("send is outside resolved worker body")
				}
			}
			if len(sends) > 0 {
				proof := abandonedProducerSend(function, sends[1], sends, engine)
				if proof.State != test.state {
					t.Fatalf("proof=%+v; want state %v", proof, test.state)
				}
			}
		})
	}
}

func logGenericWorker(t *testing.T, function *ssa.Function) {
	t.Helper()
	spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
	raw := spawn.Common().StaticCallee()
	resolved, _ := ssacall.DirectCallee(spawn.Common())
	if raw == resolved || raw.Origin() != resolved {
		t.Fatal("test did not build a generic instance")
	}
	for _, body := range []*ssa.Function{raw, resolved} {
		var dump strings.Builder
		if _, err := body.WriteTo(&dump); err != nil {
			t.Fatal(err)
		}
		t.Log(dump.String())
	}
}
