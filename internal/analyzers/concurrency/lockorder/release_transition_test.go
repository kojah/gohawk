package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCompletedUnlockTransition(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "releasetransition", `package releasetransition
 import "sync"
 func yes(mu *sync.Mutex){mu.Unlock()}
 func maybe(mu *sync.Mutex,flag bool){if flag{mu.Unlock()}}
 func root(mu *sync.Mutex,flag bool,opaque func(*sync.Mutex)){
 yes(mu);go yes(mu);maybe(mu,flag);go maybe(mu,flag);opaque(mu);defer yes(mu)
 }
 `)
	fn := pkg.Func("root")
	calls := ssaflow.InstructionsOf[ssa.CallInstruction](fn)
	for _, test := range []struct {
		name               string
		index              int
		released, possible bool
	}{
		{"called", 0, true, false},
		{"spawned", 1, true, false},
		{"conditional call", 2, false, true},
		{"conditional worker", 3, false, false},
		{"opaque", 4, false, false},
		{"deferred", 5, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var evidence lifecycle.LocalEvidence
			flow := lockFlowContext{
				releases:   newLockReleaseQueries(&evidence, proofs.NewSearchBudget(lockStateWorkBudget)),
				lockValues: map[string][]ssa.Value{"target": {fn.Params[0]}},
				released:   map[string]bool{}, unprovenRelease: map[string]bool{},
			}
			state := lockFlowState{held: []string{"target"}, guards: map[string]lockGuard{"target": {condition: "guard", value: true}}}
			state.held = flow.transferCompletedUnlocks(calls[test.index], state)
			_, guarded := state.guards["target"]
			if (len(state.held) == 0) != test.released || flow.released["target"] != test.released || guarded == test.released ||
				flow.unprovenRelease["target"] != test.possible || flow.releases.cutoff.Reason != proofs.EvidenceNone {
				t.Fatalf("held=%v released=%v guard=%v possible=%v cutoff=%+v", state.held, flow.released, guarded, flow.unprovenRelease, flow.releases.cutoff)
			}
		})
	}
}
