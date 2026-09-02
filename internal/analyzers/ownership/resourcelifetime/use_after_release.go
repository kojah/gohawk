package resourcelifetime

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The use-after-release audit is the dual of the leak check: after a direct
// release of an acquired resource, an operation the API documents as failing
// on a released value is reported. It is opt-in and deliberately narrow. The
// release must be a plain call on the exact acquired value, not a deferred
// one, and it must dominate the use, so every path to the use has released
// first; a release on one branch and a use after the merge is not claimed.
// Only operations documented to fail on a released value count. Idioms that
// touch a released value harmlessly, such as rows.Err after rows.Close, a
// deferred Close after an explicit one, or Rollback after a failed Commit,
// are not in the table.

type invalidatingOperation struct {
	packagePath string
	name        string
	methods     []string
}

// invalidatingOperations lists, per resource type, the methods whose
// documented behavior on a released value is an error.
func invalidatingOperations() []invalidatingOperation {
	return []invalidatingOperation{
		{"os", "File", []string{
			"Read", "ReadAt", "ReadFrom", "Write", "WriteAt", "WriteString", "Seek", "Sync", "Truncate", "Readdir", "ReadDir", "Readdirnames",
		}},
		{"database/sql", "Rows", []string{"Scan", "Columns", "ColumnTypes"}},
		{"database/sql", "Tx", []string{
			"Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext", "Prepare", "PrepareContext", "Stmt", "StmtContext",
		}},
		{"database/sql", "Stmt", []string{"Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext"}},
		{"net/http", "Response", []string{"Read"}},
		{"compress/gzip", "Reader", []string{"Read"}},
		{"compress/gzip", "Writer", []string{"Write", "Flush"}},
		{"compress/zlib", "Writer", []string{"Write", "Flush"}},
	}
}

func invalidatingMethods(resource ssa.Value) []string {
	for _, entry := range invalidatingOperations() {
		if syntax.NamedType(resource.Type(), entry.packagePath, entry.name) {
			return entry.methods
		}
	}
	return nil
}

func reportUsesAfterRelease(pass *analysis.Pass, function *ssa.Function, acquisition *ssa.Call, resource ssa.Value, contract resourceContract) {
	methods := invalidatingMethods(resource)
	if len(methods) == 0 {
		return
	}
	for _, release := range directReleases(function, resource, contract) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok || call == release || !ssaflow.InstructionDominates(release, call) {
					continue
				}
				if !slices.Contains(methods, ssaflow.CallName(call.Common())) || !operatesOnResource(call.Common(), resource) {
					continue
				}
				emitUseAfterRelease(pass, function, acquisition, release, call)
				check.Reportf(
					pass,
					check.ResourceUseAfterRelease,
					call.Pos(),
					"resource from %s.%s is used after %s",
					syntax.ShortPackageName(contract.packagePath),
					contract.name,
					ssaflow.CallName(release.Common()),
				)
			}
		}
	}
}

// directReleases returns the plain calls of a cleanup method on the exact
// resource. Deferred releases run at return and cannot precede a use.
func directReleases(function *ssa.Function, resource ssa.Value, contract resourceContract) []*ssa.Call {
	var releases []*ssa.Call
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			call, ok := instruction.(*ssa.Call)
			if ok && slices.Contains(contract.cleanup, ssaflow.CallName(call.Common())) && operatesOnResource(call.Common(), resource) {
				releases = append(releases, call)
			}
		}
	}
	return releases
}

// operatesOnResource reports whether the call's receiver is the exact
// resource value, or the body of an exact response. Identity through phis or
// stored locals is deliberately not followed: a variable rebound to a fresh
// acquisition must never match the released one.
func operatesOnResource(common *ssa.CallCommon, resource ssa.Value) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil {
		return false
	}
	if inner, ok := ssaflow.UnwrapTransparentValue(
		receiver,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		receiver = inner
	}
	if receiver == resource {
		return true
	}
	load, ok := receiver.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	return ok && field.X == resource && syntax.NamedType(resource.Type(), "net/http", "Response")
}

func emitUseAfterRelease(pass *analysis.Pass, function *ssa.Function, acquisition *ssa.Call, release, use *ssa.Call) {
	analysisTrace.EmitIfEnabled(pass, analysisTrace.Event{
		Analyzer: "resourcelifetime",
		Check:    string(check.ResourceUseAfterRelease),
		Phase:    "evidence",
		Reason:   "release-dominates-use",
		Outcome:  analysisTrace.OutcomeRejected,
		Pos:      use.Pos(),
		Function: function.String(),
		Details: map[string]string{
			"acquisition": pass.Fset.Position(acquisition.Pos()).String(),
			"release":     release.String(),
			"instruction": use.String(),
		},
	})
}
