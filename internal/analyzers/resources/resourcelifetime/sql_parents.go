package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// SQL parent cleanup supplies uncertainty for statements and rows whose exact
// parent is closed or finished. Storage queries keep their smaller child caps
// and share the caller allowance; neither contract proves synchronous release.

// DB.Close retires the connections that own DB-prepared driver statements.
// This is not a claim that the Stmt becomes unusable immediately: outstanding
// connection users can delay release. Rows, transactions, and Conn-prepared
// statements keep their own obligations. Require the captured receiver itself,
// not an existential alias through a mutable cell or a mixed-parent phi.
// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/benchmark_test.go#L377-L407
// Finishing a transaction cancels the transaction context watched by active
// Rows, including rows from statements prepared on that exact transaction.
// The cancellation may close rows asynchronously, so this establishes an
// opaque parent-owned lifetime, not a synchronous Rows.Close guarantee.
// DB.Close and Stmt.Close do not have this contract for their active rows.
// https://github.com/bluesky-social/indigo/blob/41278964ec8e3253e70d4e919dfb8e34211c543d/carstore/sqlite_store.go#L171-L190
func proveSQLParentCleanupWithin(acquisition *ssa.Call, instruction ssa.Instruction, budget *proofs.SearchBudget) resourceProof {
	// Symbol applicability precedes any query. An unrelated helper must reach
	// its own completion proof and exhaustion event rather than a SQL cutoff.
	unmatched := resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
	if acquisition == nil {
		return unmatched
	}
	switch instruction.(type) {
	case *ssa.Call, *ssa.Defer:
	default:
		return unmatched
	}
	common := ssaflow.InstructionCall(instruction)
	databaseParent := sqlDatabaseCall(acquisition.Common(), "Prepare", "PrepareContext") && sqlDatabaseCall(common, "Close")
	transactionParent := sqlReceiverCall(common, "Tx", "Commit", "Rollback") &&
		(sqlReceiverCall(acquisition.Common(), "Tx", "Query", "QueryContext") ||
			sqlReceiverCall(acquisition.Common(), "Stmt", "Query", "QueryContext"))
	if !databaseParent && !transactionParent {
		return unmatched
	}
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if databaseParent {
		return proveSQLParentIdentityWithin(ssaflow.CallReceiver(common), ssaflow.CallReceiver(acquisition.Common()),
			resourceReasonStatementParentClosed, budget)
	}
	parent := proveRowsTransactionWithin(acquisition, budget)
	if parent.State != proofs.EvidenceProven {
		return parent.resourceProof
	}
	return proveSQLParentIdentityWithin(ssaflow.CallReceiver(common), parent.Parent, resourceReasonRowsTransactionFinished, budget)
}

type rowsTransactionProof struct {
	resourceProof
	Parent ssa.Value
}

func proveRowsTransactionWithin(acquisition *ssa.Call, budget *proofs.SearchBudget) rowsTransactionProof {
	if !budget.Spend() {
		return rowsTransactionProof{resourceProof: carriedValueProof(false, resourceReasonUntouched, budget)}
	}
	common := acquisition.Common()
	if sqlReceiverCall(common, "Tx", "Query", "QueryContext") {
		return rowsTransactionProof{
			resourceProof: carriedValueProof(true, resourceReasonRowsTransactionFinished, budget),
			Parent:        ssaflow.CallReceiver(common),
		}
	}
	if !sqlReceiverCall(common, "Stmt", "Query", "QueryContext") {
		return rowsTransactionProof{resourceProof: carriedValueProof(false, resourceReasonUntouched, budget)}
	}
	// Require the exact constructor result. A replaced statement, mixed phi or
	// constructor on another parent must retain its independent obligation.
	child := budget.Within(proofs.QueryBudget)
	statement := heapmodel.NewStorage(child).Resolve(ssaflow.CallReceiver(common))
	if resourceFlowExhausted(child) {
		return rowsTransactionProof{resourceProof: carriedValueProof(false, resourceReasonUntouched, child)}
	}
	extract, ok := statement.Value.(*ssa.Extract)
	if !statement.Proven() || !ok || extract.Index != 0 {
		return rowsTransactionProof{resourceProof: carriedValueProof(false, resourceReasonUntouched, budget)}
	}
	prepare, ok := extract.Tuple.(*ssa.Call)
	if !ok || !sqlReceiverCall(prepare.Common(), "Tx", "Prepare", "PrepareContext") {
		return rowsTransactionProof{resourceProof: carriedValueProof(false, resourceReasonUntouched, budget)}
	}
	return rowsTransactionProof{
		resourceProof: carriedValueProof(true, resourceReasonRowsTransactionFinished, budget),
		Parent:        ssaflow.CallReceiver(prepare.Common()),
	}
}

// Later closure captures can spill a local into a cell. Two loads agree only
// when their point-in-time stored values agree; a shared address alone must
// never accept a reassigned DB. Graph and type internals retain separate costs.
func proveSQLParentIdentityWithin(left, right ssa.Value, reason resourceLifetimeReason, budget *proofs.SearchBudget) resourceProof {
	child := budget.Within(proofs.QueryBudget)
	proof := heapmodel.NewStorage(child).Same(left, right)
	return carriedValueProof(proof.Proven(), reason, child)
}
