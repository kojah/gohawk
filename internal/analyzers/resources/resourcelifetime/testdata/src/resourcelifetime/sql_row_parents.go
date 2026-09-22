package resourcelifetime

import (
	"context"
	"database/sql"
)

// Finishing the exact transaction cancels its active rows, including rows
// queried through a statement prepared on that transaction. A DB close, a
// sibling transaction, or a conditional finish cannot establish that boundary.
func rowsTransactionDeferred(tx *sql.Tx) {
	defer tx.Rollback()
	_, _ = tx.Query("SELECT 1")
}

func rowsTransactionFinished(tx *sql.Tx, ctx context.Context) {
	_, _ = tx.QueryContext(ctx, "SELECT 1")
	_ = tx.Commit()
}

func rowsTransactionStatement(tx *sql.Tx, ctx context.Context) {
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, "SELECT 1")
	if err != nil {
		return
	}
	_, _ = stmt.QueryContext(ctx)
}

func rowsTransactionSibling(tx, other *sql.Tx) {
	defer other.Rollback()
	_, _ = tx.Query("SELECT 1") // want "owned resource from sql.Query is not released"
}

func rowsTransactionConditional(tx *sql.Tx, finish bool) {
	_, _ = tx.Query("SELECT 1") // want "owned resource from sql.Query is not released"
	if finish {
		_ = tx.Rollback()
	}
}

func rowsTransactionConditionalDefer(tx *sql.Tx, finish bool) {
	if finish {
		defer tx.Rollback()
	}
	_, _ = tx.Query("SELECT 1") // want "owned resource from sql.Query is not released"
}

func rowsTransactionStatementSibling(tx, other *sql.Tx) {
	defer other.Rollback()
	stmt, err := tx.Prepare("SELECT 1")
	if err != nil {
		return
	}
	_, _ = stmt.Query() // want "owned resource from sql.Query is not released"
}

func rowsTransactionStatementReplaced(tx *sql.Tx, other *sql.Stmt) {
	defer tx.Rollback()
	stmt, err := tx.Prepare("SELECT 1")
	if err != nil {
		return
	}
	stmt = other
	_, _ = stmt.Query() // want "owned resource from sql.Query is not released"
}
