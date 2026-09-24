package resourcemodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestForwardedResultSetReleaseUsesExactField(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "example.com/resourcemodeltest", `package resourcemodeltest
import "database/sql"
type scanner struct { rows, other *sql.Rows }
func (s *scanner) NextResultSet() bool { return s.other.NextResultSet() }
func Caller(rows, other *sql.Rows) bool {
	s := &scanner{rows: rows, other: other}
	return s.NextResultSet()
}
`)
	caller := pkg.Func("Caller")
	var call *ssa.Call
	for _, candidate := range ssaflow.InstructionsOf[*ssa.Call](caller) {
		if candidate.Common().StaticCallee() != nil && candidate.Common().StaticCallee().Name() == "NextResultSet" {
			call = candidate
		}
	}
	if call == nil {
		t.Fatal("missing scanner call")
	}
	var owner *ssa.Alloc
	for _, candidate := range ssaflow.InstructionsOf[*ssa.Alloc](caller) {
		if candidate == call.Common().Args[0] {
			owner = candidate
		}
	}
	if owner == nil {
		t.Fatal("missing scanner owner")
	}
	if proof := ProveRelation(owner, caller.Params[0], call, nil); proof.Proven() {
		t.Fatal("relation without a budget was treated as proven")
	}
	predicate := lifecycle.CompletionPredicate{Outcome: lifecycle.CompletionWhenFalse}
	if ConditionalRelease(call, caller.Params[0], "Close", false, predicate, nil) {
		t.Fatal("conditional release without a budget was treated as proven")
	}
	budget := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	for index, path := range []string{"field:0", "field:1"} {
		relation := ProveRelation(owner, caller.Params[index], call, budget)
		if !relation.Proven() || !relation.Relation.At([]string{path}) {
			t.Errorf("parameter %d relation = %+v, want %s", index, relation, path)
		}
	}
	if direct := ProveRelation(caller.Params[0], caller.Params[0], call, budget); !direct.Proven() || !direct.Relation.At(nil) {
		t.Errorf("direct relation = %+v, want empty path", direct)
	}
	if ConditionalRelease(call, caller.Params[0], "Close", false, predicate, budget) {
		t.Fatal("closing the other field was credited to rows")
	}
	if !ConditionalRelease(call, caller.Params[1], "Close", false, predicate, budget) {
		t.Fatal("exact other field was not recognized")
	}
}
