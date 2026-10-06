package resourcelifetime

import (
	"go/token"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/resultfacts"
	"github.com/kojah/gohawk/internal/analysis/summaries"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestResourceBranchAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "branchbudget", `package branchbudget
import (
 "errors"
 "os"
)
func identity(err error) error { return err }
func direct(err error) { if err != nil { println(err) } }
func reversed(err error) { if nil == err { println(err) } }
func unrelated(err, other error) { if other != nil { println(err) } }
func asserted(err error) { if _, ok := err.(interface{ Timeout() bool }); ok { println(err) } }
func sentinel(err error) { if err == os.ErrNotExist { println(err) } }
func matching(err error) { if errors.Is(err, os.ErrNotExist) { println(err) } }
func wrapped(err error) { if os.IsNotExist(identity(err)) { println(err) } }
func joined(err error) { if errors.Is(errors.Join(err, os.ErrNotExist), os.ErrNotExist) { println(err) } }
func derived(err error) {
`+strings.Repeat("err = identity(err)\n", 64)+`if err != nil { println(err) }
}
func predicate(err error) bool {
`+strings.Repeat("println(err)\n", 32)+`return err != nil
}
func summarized(err error) { if predicate(err) { println(err) } }
func captured(err error) { check := predicate; func() { if check(err) { println(err) } }() }
`)
	for _, test := range []struct {
		name    string
		known   bool
		success bool
	}{
		{"direct", true, false},
		{"reversed", true, true},
		{"unrelated", false, false},
		{"asserted", true, false},
		{"derived", true, false},
		{"summarized", true, false},
		{"captured", true, false},
		{"sentinel", true, false},
		{"matching", true, false},
		{"wrapped", true, false},
		{"joined", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			errorValue := ssa.Value(fn.Params[0])
			if test.name == "captured" {
				fn = fn.AnonFuncs[0]
				errorValue = ssaflow.InstructionsOf[*ssa.Call](fn)[0].Common().Args[0]
			}
			branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
			if test.name == "derived" && len(ssaflow.InstructionsOf[*ssa.Call](fn)) != 65 {
				t.Fatal("fixture must retain 64 provenance calls and println in actual SSA")
			}
			t.Log(branch.String())
			if test.name == "derived" || test.name == "summarized" || test.name == "captured" {
				// These actual bodies require more than sixteen structural visits;
				// standalone derivation or summary budgets would wrongly accept them.
				got := proveResourceSuccessBranch(nil, branchTestKnowledge(), branch.Block(), branch.Block().Succs[0],
					errorValue, token.NoPos, proofs.NewSearchBudget(16))
				if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
					t.Fatalf("nested work bypassed the branch allowance: %+v", got)
				}
			}
			for arm, successor := range branch.Block().Succs {
				wantSuccess := test.success == (arm == 0)
				// Non-nil predicate evidence only decides the true edge.
				wantKnown := test.known && (arm == 0 || test.name == "direct" || test.name == "reversed" || test.name == "derived")
				checkResourceBranchArm(t, branch, successor, errorValue, wantKnown, wantSuccess)
			}
		})
	}
}

func checkResourceBranchArm(t *testing.T, branch *ssa.If, successor *ssa.BasicBlock, errorValue ssa.Value, wantKnown, wantSuccess bool) {
	t.Helper()
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got := proveResourceSuccessBranch(nil, branchTestKnowledge(), branch.Block(), successor, errorValue, token.NoPos, budget)
		if resourceFlowExhausted(budget) {
			if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || got.success {
				t.Fatalf("allowance %d admitted interrupted branch: %+v", limit, got)
			}
			continue
		}
		if got.Proven() != wantKnown || wantKnown && got.success != wantSuccess {
			t.Fatalf("completed branch = %+v", got)
		}
		return
	}
	t.Fatal("fresh branch never completed")
}

func branchTestKnowledge() *summaries.Provider {
	return resourceSummaries.Provider(&analysis.Pass{ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}})
}

func TestResourceBranchChildCutoffDiscardsSuccessors(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "branchcutoff", `package branchcutoff
type resource struct{}
func acquire() *resource { return new(resource) }
func identity(err error) error { return err }
func branch(err error) {
value := acquire()
`+strings.Repeat("err = identity(err)\n", proofs.SummaryBudget)+`if err != nil { println(value) }
}`)
	fn := pkg.Func("branch")
	branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
	acquisition := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	state := resourceFlowState{block: branch.Block()}
	knowledge := branchTestKnowledge()
	evidence, _ := knowledge.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	query := &resourceAnalysis{
		summaries: knowledge, evidence: evidence, function: fn, acquisition: acquisition, resource: acquisition, pool: pool,
		contract: resourceContract{cleanup: []string{"Close"}}, actions: map[ssa.Instruction]resourceAction{},
	}
	got := resourceSuccessorStates(query, state, fn.Params[0], pool)
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || got.states != nil || pool.Exhausted() {
		t.Fatalf("branch child cutoff must discard all successors while outer pool remains live: %+v, outer exhausted %v", got, pool.Exhausted())
	}
	// Isolate the edge proof from instruction classification so this control
	// requires the final flow to retain the child query's availability.
	for instruction := range ssaflow.InstructionsWithin(fn, nil) {
		query.actions[instruction] = actionNone
	}
	query.pool = proofs.NewSearchBudget(resourcePoolBudget)
	flow := query.proveResourceFlow(fn.Params[0])
	if flow.state != proofs.EvidenceUnknown || flow.reason != resourceReasonBudgetExhausted || flow.leak != nil || query.pool.Exhausted() {
		t.Fatalf("final flow admitted a child cutoff: %+v, outer exhausted %v", flow, query.pool.Exhausted())
	}
}

func TestSQLRowsEdgeAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "rowsbudget", `package rowsbudget
import "database/sql"
func next(rows, other *sql.Rows) { if rows.Next() { println(rows) } }
`)
	fn := pkg.Func("next")
	branch := ssaflow.InstructionsOf[*ssa.If](fn)[0]
	for arm, successor := range branch.Block().Succs {
		for index, resource := range fn.Params {
			completed := false
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveSQLRowsExhaustionEdge(branch.Block(), successor, resource, budget)
				if resourceFlowExhausted(budget) {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("Rows cutoff admitted proof: %+v", got)
					}
					continue
				}
				if got.Proven() != (arm == 1 && index == 0) {
					t.Fatalf("Rows arm %d receiver %d = %+v", arm, index, got)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("fresh Rows query never completed")
			}
		}
	}
}
