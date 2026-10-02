package processownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestProgramEntryProcessDecision(t *testing.T) {
	const body = `cmd:=exec.Command("child");if err:=cmd.Start();err!=nil{return};_=cmd.Process.Kill()`
	for _, test := range []struct {
		name, path, source string
		want               ssaflow.EvidenceState
	}{
		{"once", "main", `func main(){` + body + `}`, ssaflow.EvidenceUnknown},
		{"conditional", "main", `var launch bool;func main(){if launch{` + body + `}}`, ssaflow.EvidenceUnknown},
		{"loop", "main", `var stop bool;func main(){for{` + body + `;if stop{return}}}`, ssaflow.EvidenceProven},
		{"referenced", "main", `var entry=main;func main(){` + body + `}`, ssaflow.EvidenceProven},
		{"called", "main", `func again(){main()};func main(){` + body + `}`, ssaflow.EvidenceProven},
		{"helper", "main", `func launch(){` + body + `};func main(){launch()}`, ssaflow.EvidenceProven},
		{"closure", "main", `func main(){launch:=func(){` + body + `};launch()}`, ssaflow.EvidenceProven},
		{"method", "main", `type app struct{};func(app)main(){` + body + `};func main(){app{}.main()}`, ssaflow.EvidenceProven},
		{"otherPackage", "worker", `func main(){` + body + `}`, ssaflow.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, test.path, "package "+test.path+"\nimport \"os/exec\"\n"+test.source)
			start, command, witness := programEntryInputs(t, pkg)
			got := decideProcessReturn(start, command, witness, false, nil)
			if got.state != test.want {
				t.Fatalf("decision = %+v, want state %v", got, test.want)
			}
			if test.want == ssaflow.EvidenceUnknown && got.reason != reasonProgramLifetimeOwnershipUnknown {
				t.Fatalf("entry reason = %v", got.reason)
			}
			settled := decideProcessReturn(start, command, nil, false, nil)
			if settled.state != ssaflow.EvidenceDisproven || settled.reason != reasonWaitOwnershipProven {
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
