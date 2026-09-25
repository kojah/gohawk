package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// missingReleaseEvidence cites the return the flow walk reached with the
// resource still owed, naming the variable the acquisition assigned. The
// witness is the proof's own, so the report and its evidence cannot disagree.
func missingReleaseEvidence(pass *analysis.Pass, call *ssa.Call, result int, leak *ssa.Return) []analysis.RelatedInformation {
	subject := "the resource"
	if name := syntax.AssignedName(pass, call.Pos(), result); name != "" {
		subject = "`" + name + "`"
	}
	return check.ReturnEvidence(pass, leak, "releasing "+subject)
}
