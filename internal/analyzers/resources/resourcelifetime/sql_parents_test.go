package resourcelifetime

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestSQLParentCleanupAllowance(t *testing.T) {
	pkg := sqlParentFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"dbCell", true},
		{"dbCellReplaced", false},
		{"dbMixed", false},
		{"dbSame", true},
		{"dbSibling", false},
		{"dbReplaced", false},
		{"dbSaved", true},
		{"txSame", true},
		{"txSibling", false},
		{"stmtSame", true},
		{"stmtSibling", false},
		{"stmtReplaced", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			acquisition, cleanup := sqlParentInputs(t, pkg.Func(test.name))
			checkResourceProofAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
				return proveSQLParentCleanupWithin(acquisition, cleanup, budget)
			}, test.want)
		})
	}
}

func TestSQLParentConsumersCutoff(t *testing.T) {
	pkg := sqlParentFixture(t)
	for _, name := range []string{"dbSame", "stmtSame"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			acquisition, cleanup := sqlParentInputs(t, fn)
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			query := &resourceAnalysis{function: fn, acquisition: acquisition, resource: acquisition, summaries: provider, evidence: evidence}
			query.pool = ssaflow.NewSearchBudget(0)
			if action, reason := query.classify(cleanup); action != actionUnknown || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted classifier = %v/%v", action, reason)
			}
			query.pool = ssaflow.NewSearchBudget(resourcePoolBudget)
			want := proveSQLParentCleanupWithin(acquisition, cleanup, nil).Reason
			if action, reason := query.classify(cleanup); action != actionUnknown || reason != want {
				t.Fatalf("fresh classifier = %v/%v", action, reason)
			}
			pool := ssaflow.NewSearchBudget(resourcePoolBudget)
			got := query.provePriorCleanupWithin(acquisition, pool.Within(2))
			if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
				t.Fatalf("interrupted prior cleanup = %+v", got)
			}
			if fresh := query.provePriorCleanupWithin(acquisition, pool.Within(releaseSearchBudget)); !fresh.Proven() || fresh.Reason != want {
				t.Fatalf("fresh prior cleanup = %+v", fresh)
			}
		})
	}
}

func TestSQLParentStorageChildCap(t *testing.T) {
	source := `package sqlparentcap
import "database/sql"
type dbAlias sql.DB
func long(db *sql.DB) { p := db;
` + strings.Repeat("p = (*sql.DB)((*dbAlias)(p))\n", ssaflow.QueryBudget+20) + `
defer p.Close(); _,_ = db.Prepare("q") }
func short(db *sql.DB) { p := (*sql.DB)((*dbAlias)(db)); defer p.Close(); _,_ = db.Prepare("q") }
`
	pkg := ssaflowtest.BuildPackage(t, "sqlparentcap", source)
	for _, name := range []string{"long", "short"} {
		acquisition, cleanup := sqlParentInputs(t, pkg.Func(name))
		pool := ssaflow.NewSearchBudget(resourcePoolBudget)
		got := proveSQLParentCleanupWithin(acquisition, cleanup, pool.Within(releaseSearchBudget))
		if name == "long" {
			if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
				t.Fatalf("storage child cutoff = %+v; parent exhausted %v", got, pool.Exhausted())
			}
		} else if !got.Proven() {
			t.Fatalf("short identity = %+v", got)
		}
	}
}

func sqlParentInputs(t *testing.T, fn *ssa.Function) (*ssa.Call, ssa.Instruction) {
	t.Helper()
	calls := ssaflow.InstructionsOf[*ssa.Call](fn)
	var acquisition *ssa.Call
	for _, call := range calls {
		name := ssaflow.CallName(call.Common())
		if name == "Prepare" || name == "Query" {
			acquisition = call
		}
	}
	if acquisition == nil {
		t.Fatal("missing acquisition")
	}
	deferred := ssaflow.InstructionsOf[*ssa.Defer](fn)
	if len(deferred) != 1 {
		t.Fatalf("expected one cleanup: %s", carriedSSA(t, fn))
	}
	return acquisition, deferred[0]
}

func sqlParentFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "sqlparent", `package sqlparent
import "database/sql"
func dbCell(db *sql.DB){defer db.Close();f:=func(){println(db)};_,_=db.Prepare("q");f()}
func dbCellReplaced(db,other *sql.DB){defer db.Close();db=other;f:=func(){println(db)};_,_=db.Prepare("q");f()}
func dbMixed(db,other *sql.DB,yes bool){parent:=db;if yes {parent=other};defer parent.Close();_,_=db.Prepare("q")}
func dbSame(db *sql.DB){defer db.Close();_,_=db.Prepare("q")}
func dbSibling(db,other *sql.DB){defer other.Close();_,_=db.Prepare("q")}
func dbReplaced(db,other *sql.DB){defer db.Close();db=other;_,_=db.Prepare("q")}
func dbSaved(db,other *sql.DB){saved:=db;defer saved.Close();db=other;_,_=saved.Prepare("q")}
func txSame(tx *sql.Tx){defer tx.Rollback();_,_=tx.Query("q")}
func txSibling(tx,other *sql.Tx){defer other.Rollback();_,_=tx.Query("q")}
func stmtSame(tx *sql.Tx){defer tx.Rollback();s,_:=tx.Prepare("q");_,_=s.Query()}
func stmtSibling(tx,other *sql.Tx){defer other.Rollback();s,_:=tx.Prepare("q");_,_=s.Query()}
func stmtReplaced(tx *sql.Tx,other *sql.Stmt){defer tx.Rollback();s,_:=tx.Prepare("q");s=other;_,_=s.Query()}
`)
}
