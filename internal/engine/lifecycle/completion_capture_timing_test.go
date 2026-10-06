package lifecycle

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
)

const completionCaptureTimingFixture = `
 func beforeCapture(p *resource){done:=true;defer func(){if done{p.Close()}}()}
 func earlyCapture(p *resource,early bool){var done bool;defer func(){if done{p.Close()}}();if early{return};done=true}
 func preStoreCapture(p *resource){var done bool;func(){if done{p.Close()}}();done=true}
 func lateCapture(p *resource){var done bool;f:=func(){if done{p.Close()}};done=true;f()}
 func namedCapture(p *resource)(done bool){defer func(){if done{p.Close()}}();return true}
`

func TestCompletionCapturedOutcomeTiming(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+completionCaptureTimingFixture)
	for _, test := range []struct {
		name  string
		known ssacall.Outcome
		want  bool
	}{
		{"beforeCapture", ssacall.OutcomeAny, true},
		{"earlyCapture", ssacall.OutcomeAny, false},
		{"preStoreCapture", ssacall.OutcomeAny, false},
		{"lateCapture", ssacall.OutcomeAny, false},
		{"lateCapture", ssacall.OutcomeTrue, true},
		{"namedCapture", ssacall.OutcomeAny, false},
		{"namedCapture", ssacall.OutcomeTrue, true},
		{"namedCapture", ssacall.OutcomeFalse, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := findLaunch(t, fn)
			body, closure := ssacall.DirectCallee(ssaflow.InstructionCall(call))
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			if _, err := body.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			request := CompletionRequest{Instruction: call, Target: fn.Params[0], Methods: []string{"Close"}}
			if test.known != ssacall.OutcomeAny {
				request.Constants = ssacall.FixedValues{}
				for pair := range ssaflow.ClosureBindingPairsWithin(body, closure, nil) {
					if pair.Free.Name() == "done" {
						request.Constants[pair.Binding] = test.known
					}
				}
				if len(request.Constants) != 1 {
					t.Fatal("missing exact done capture")
				}
			}
			proof := ProveCompletion(request)
			if proof.Proven() != test.want {
				t.Fatalf("completion=%+v want proven=%v", proof, test.want)
			}
		})
	}
}
