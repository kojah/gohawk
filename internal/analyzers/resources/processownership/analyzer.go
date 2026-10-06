// Package processownership implements the processownership gohawk analyzer.
package processownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	"github.com/kojah/gohawk/internal/syntax"

	proofs "github.com/kojah/gohawk/internal/proof"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

var summaryKnowledge = summaries.Select(summaries.Requirements{Results: true, Lifecycle: true})

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "processownership",
		Doc:      "checks that started os/exec commands are waited on or transferred to a wait owner",
		Requires: summaryKnowledge.Requires(),
		Run:      runProcessOwnership,
	}
}

func runProcessOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		evidence, _ := summaryKnowledge.Provider(pass).LifecycleEvidence("processownership", string(check.ProcessWait))
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				start, command, ok := startedCommand(instruction)
				if !ok {
					continue
				}
				evidence.ForCandidate(start.Pos())
				probe := analysisTrace.For(pass, "processownership", string(check.ProcessWait), start.Pos())
				proof := &commandProof{evidence: evidence, pool: proofs.NewSearchBudget(processPoolBudget).Observed(probe.Observer())}
				decision := proveProcessStart(proof, function, start, command)
				if decision.state != proofs.EvidenceProven {
					emitProcessDecision(pass, function, start, command, decision)
					continue
				}
				reportStartedCommand(pass, proof, function, start, command)
			}
		}
	}
	return nil, nil
}

// startedCommand returns the Start call and the *exec.Cmd it starts.
func startedCommand(instruction ssa.Instruction) (*ssa.Call, ssa.Value, bool) { //nolint:ireturn // Commands retain their concrete SSA forms.
	start, ok := instruction.(*ssa.Call)
	startCall := syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Start"})
	if !ok || !ssaflow.CallMatchesSymbol(start.Common(), startCall) || !execCommandValue(ssaflow.CallReceiver(start.Common())) {
		return nil, nil, false
	}
	return start, ssaflow.CallReceiver(start.Common()), true
}

// reportStartedCommand presents the authoritative successful-return proof.
func reportStartedCommand(pass *analysis.Pass, proof *commandProof, function *ssa.Function, start *ssa.Call, command ssa.Value) {
	result := proveProcessReturns(pass, proof, function, start, command)
	command = result.command
	emitProcessDecision(pass, function, start, command, result.decision)
	if result.decision.state != proofs.EvidenceProven {
		return
	}
	subject := "the command"
	if name := commandName(pass, command); name != "" {
		subject = "`" + name + "`"
	}
	source := syntax.SourceRange(pass, start.Pos())
	check.Report(pass, check.ProcessWait, analysis.Diagnostic{
		Pos:     source.Pos(),
		End:     source.End(),
		Message: "started command is not waited on every successful return path",
		Related: check.ReturnEvidence(pass, start, result.witness, "waiting for "+subject),
	})
}

// commandName names the command by its variable: the one exec.Command's
// result was assigned to, or the local a load reads it from.
func commandName(pass *analysis.Pass, command ssa.Value) string {
	switch value := command.(type) {
	case *ssa.Call:
		return syntax.AssignedName(pass, value.Pos(), 0)
	case *ssa.UnOp:
		if cell, ok := value.X.(*ssa.Alloc); ok && value.Op == token.MUL {
			return cell.Comment
		}
	case *ssa.Alloc:
		return value.Comment
	}
	return ""
}
