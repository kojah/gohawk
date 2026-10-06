package processownership

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestProgramEntryProcessDecision(t *testing.T) {
	const body = `cmd:=exec.Command("child");if err:=cmd.Start();err!=nil{return};_=cmd.Process.Kill()`
	for _, test := range []struct {
		name, path, source string
		want               proofs.EvidenceState
	}{
		{"once", "main", `func main(){` + body + `}`, proofs.EvidenceUnknown},
		{"conditional", "main", `var launch bool;func main(){if launch{` + body + `}}`, proofs.EvidenceUnknown},
		{"loop", "main", `var stop bool;func main(){for{` + body + `;if stop{return}}}`, proofs.EvidenceProven},
		{"referenced", "main", `var entry=main;func main(){` + body + `}`, proofs.EvidenceProven},
		{"called", "main", `func again(){main()};func main(){` + body + `}`, proofs.EvidenceProven},
		{"helper", "main", `func launch(){` + body + `};func main(){launch()}`, proofs.EvidenceProven},
		{"closure", "main", `func main(){launch:=func(){` + body + `};launch()}`, proofs.EvidenceProven},
		{"method", "main", `type app struct{};func(app)main(){` + body + `};func main(){app{}.main()}`, proofs.EvidenceProven},
		{"otherPackage", "worker", `func main(){` + body + `}`, proofs.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, test.path, "package "+test.path+"\nimport \"os/exec\"\n"+test.source)
			start, command, witness := programEntryInputs(t, pkg)
			got := decideProcessReturn(start, command, witness, false, nil)
			if got.state != test.want {
				t.Fatalf("decision = %+v, want state %v", got, test.want)
			}
			if test.want == proofs.EvidenceUnknown && got.reason != reasonProgramLifetimeOwnershipUnknown {
				t.Fatalf("entry reason = %v", got.reason)
			}
			settled := decideProcessReturn(start, command, nil, false, nil)
			if settled.state != proofs.EvidenceDisproven || settled.reason != reasonWaitOwnershipProven {
				t.Fatalf("exact settlement changed: %+v", settled)
			}
		})
	}
}

func programEntryInputs(t *testing.T, pkg *ssa.Package) (*ssa.Call, ssa.Value, *ssa.Return) {
	t.Helper()
	for _, fn := range ssaflow.DeclaredFunctions(pkg) {
		for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
			if ssaflow.CallName(call.Common()) != "Start" {
				continue
			}
			returns := ssaflow.InstructionsOf[*ssa.Return](fn)
			if len(returns) == 0 {
				t.Fatal("missing return witness")
			}
			return call, ssaflow.CallReceiver(call.Common()), returns[len(returns)-1]
		}
	}
	t.Fatal("missing process Start")
	return nil, nil, nil
}
