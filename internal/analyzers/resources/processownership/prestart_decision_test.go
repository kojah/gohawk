package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestProcessStartDecision(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "startdecision", `package startdecision
 import "os/exec"
 type holder struct{ cmd *exec.Cmd }
 func stored(owner *holder){cmd:=exec.Command("tool");owner.cmd=cmd;cmd.Start()}
 func helper()*exec.Cmd{return exec.Command("tool")}
 func factory(){cmd:=helper();cmd.Start()}
 func caller(cmd *exec.Cmd){cmd.Start()}
 func aggregate(i int){cmds:=[]*exec.Cmd{exec.Command("tool")};cmds[i].Start()}
 func registered(){cmd:=exec.Command("tool");defer func(){cmd.Wait()}();cmd.Start()}
 func local(){cmd:=exec.Command("tool");if err:=cmd.Start();err!=nil{return};println(cmd)}
 func endless(){cmd:=exec.Command("tool");if err:=cmd.Start();err!=nil{return};for{}}
 `)
	for _, test := range []struct {
		name   string
		state  proofs.EvidenceState
		reason processReason
	}{
		{"factory", proofs.EvidenceUnknown, reasonHelperOwnershipUnknown},
		{"caller", proofs.EvidenceUnknown, reasonCallerCommandOwnershipUnknown},
		{"aggregate", proofs.EvidenceUnknown, reasonAggregateCommandOwnershipUnknown},
		{"registered", proofs.EvidenceUnknown, reasonPreStartOwnershipUnknown},
		{"stored", proofs.EvidenceUnknown, reasonPreStartOwnershipUnknown},
		{"local", proofs.EvidenceProven, reasonLocalWaitObligation},
		{"endless", proofs.EvidenceDisproven, reasonSuccessfulStartCannotReturn},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			start := startupTestCall(t, fn)
			command := ssaflow.CallReceiver(start.Common())
			proof := &commandProof{
				pool:     proofs.NewSearchBudget(processPoolBudget),
				evidence: lifecyclefacts.NewLifecycleEvidence(nil, "test", "process-start"),
			}
			got := proveProcessStart(proof, fn, start, command)
			if got.state != test.state || got.reason != test.reason {
				t.Fatalf("start decision = %+v, want state %v reason %s", got, test.state, test.reason.String())
			}
			proof.pool = proofs.NewSearchBudget(0)
			cut := proveProcessStart(proof, fn, start, command)
			if cut.state != proofs.EvidenceUnknown || cut.reason != reasonPreStartCutoff {
				t.Fatalf("cutoff = %+v", cut)
			}
		})
	}
	proof := &commandProof{pool: proofs.NewSearchBudget(processPoolBudget)}
	got := proveProcessStart(proof, nil, nil, nil)
	if got.state != proofs.EvidenceUnknown || got.reason != reasonPreStartEvidenceUnavailable {
		t.Fatalf("unavailable = %+v", got)
	}
}
