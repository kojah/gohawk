package resourcelifetime

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Context cancellation pairs exact standard constructor results with their
// cancel invocation. Pre-acquisition DB failure and later transaction cleanup
// uncertainty share pairing mechanics but retain different lifecycle meanings.

// These DB methods obtain a connection through DB.conn, which checks ctx.Done
// before giving the driver any work. Conn, Tx, and Stmt methods do not share
// that entry check. Only an ordinary invocation of the exact paired cancel
// dominating acquisition counts; deadline expiry, deferred calls, and sleeps
// do not establish cancellation before acquisition.
// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/driver_test.go#L2794-L2803
func proveAcquisitionContextCanceledWithin(acquisition *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	common := acquisition.Common()
	if !sqlDatabaseCall(common, "PrepareContext", "QueryContext", "BeginTx") || len(common.Args) < 2 {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
	}
	constructor := contextCancelConstructor(common.Args[1])
	if constructor == nil {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
	}
	for instruction := range ssaflow.InstructionsWithin(acquisition.Parent(), budget) {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			continue
		}
		if invokesContextCancel(call.Common(), constructor) && cfg.InstructionDominates(call, acquisition) {
			return carriedValueProof(true, resourceReasonCanceledAcquisition, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// BeginTx binds the transaction lifetime to its context. Cancellation triggers
// database/sql's rollback watcher; it does not prove synchronous rollback or
// successful commit. Only the exact factory's paired cancel is recognized.
// https://pkg.go.dev/database/sql#DB.BeginTx
// https://github.com/OdyseeTeam/odysee-api/blob/6cb1fd36ef7d25a038e3ddf572e3ddb3bbbb3d79/apps/watchman/olapdb/olapdb.go#L112-L139
func cancelsTransactionContext(acquisition *ssa.Call, instruction ssa.Instruction) bool {
	common := acquisition.Common()
	if !sqlReceiverCall(common, "DB", "BeginTx") && !sqlReceiverCall(common, "Conn", "BeginTx") || len(common.Args) < 2 {
		return false
	}
	switch instruction.(type) {
	case *ssa.Call, *ssa.Defer:
		return invokesContextCancel(ssaflow.InstructionCall(instruction), contextCancelConstructor(common.Args[1]))
	}
	return false
}

// Pairing stays structural: context result zero and cancel result one from one
// standard constructor. A deadline alone, a replaced context, or another
// factory's cancel supplies no cancellation witness.
func contextCancelConstructor(value ssa.Value) *ssa.Call {
	ctx, ok := value.(*ssa.Extract)
	if !ok || ctx.Index != 0 {
		return nil
	}
	constructor, ok := ctx.Tuple.(*ssa.Call)
	if !ok || !ssaflow.CallMatchesAnySymbol(constructor.Common(),
		syntax.PackageFunction("context", "WithCancel"),
		syntax.PackageFunction("context", "WithCancelCause"),
		syntax.PackageFunction("context", "WithDeadline"),
		syntax.PackageFunction("context", "WithDeadlineCause"),
		syntax.PackageFunction("context", "WithTimeout"),
		syntax.PackageFunction("context", "WithTimeoutCause")) {
		return nil
	}
	return constructor
}

func invokesContextCancel(common *ssa.CallCommon, constructor *ssa.Call) bool {
	if common == nil || constructor == nil {
		return false
	}
	cancel, ok := common.Value.(*ssa.Extract)
	return ok && cancel.Tuple == constructor && cancel.Index == 1
}
