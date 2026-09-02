package resourcelifetime

import (
	"context"
	"database/sql"
	"resourcedep"
	"testing"

	"github.com/stretchr/testify/require"
)

func leakedRows(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	_ = rows
	return nil
}

func closedRows(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	defer rows.Close()
	return nil
}

func optionallyQueriedRows(ctx context.Context, database *sql.DB, query string) error {
	var rows *sql.Rows
	var err error
	if query != "" {
		rows, err = database.QueryContext(ctx, query)
	}
	switch {
	case query == "":
		return nil
	case err != nil:
		return err
	default:
		defer rows.Close()
		return nil
	}
}

func optionallyQueriedRowsWithoutCleanup(ctx context.Context, database *sql.DB, query string) error {
	var rows *sql.Rows
	var err error
	if query != "" {
		rows, err = database.QueryContext(ctx, query) // want "owned resource from sql.QueryContext is not released on every return path"
	}
	switch {
	case query == "":
		return nil
	case err != nil:
		return err
	default:
		_ = rows
		return nil
	}
}

func optionalRowsClosedThroughAmbiguousPhi(ctx context.Context, database *sql.DB, other *sql.Rows, query string, choose bool) error {
	var rows *sql.Rows
	var err error
	if query != "" {
		rows, err = database.QueryContext(ctx, query) // want "owned resource from sql.QueryContext is not released on every return path"
	}
	if query == "" {
		return nil
	}
	if err != nil {
		return err
	}
	selected := other
	if choose {
		selected = rows
	}
	defer selected.Close()
	return nil
}

func scanAndCloseRows(rows *sql.Rows) error {
	defer func() { _ = rows.Close() }()
	for rows.Next() {
	}
	return rows.Err()
}

func rowsClosedByStaticHelper(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	return scanAndCloseRows(rows)
}

func conditionallyCloseRows(rows *sql.Rows, closeRows bool) {
	if closeRows {
		defer func() { _ = rows.Close() }()
	}
}

func rowsConditionallyClosedByHelper(ctx context.Context, database *sql.DB, closeRows bool) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	conditionallyCloseRows(rows, closeRows)
	return nil
}

func closeDifferentRows(rows, other *sql.Rows) {
	defer func() { _ = other.Close() }()
	_ = rows
}

func rowsPassedToDifferentHelperTarget(ctx context.Context, database *sql.DB, other *sql.Rows) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	closeDifferentRows(rows, other)
	return nil
}

func reassignRowsBeforeDefer(rows, other *sql.Rows) {
	rows = other
	defer func() { _ = rows.Close() }()
}

func rowsReassignedBeforeHelperDefer(ctx context.Context, database *sql.DB, other *sql.Rows) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	reassignRowsBeforeDefer(rows, other)
	return nil
}

func reassignRowsAfterDefer(rows, other *sql.Rows) {
	defer func() { _ = rows.Close() }()
	rows = other
}

func rowsReassignedAfterHelperDefer(ctx context.Context, database *sql.DB, other *sql.Rows) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	reassignRowsAfterDefer(rows, other)
	return nil
}

type rowsConsumer interface {
	Consume(*sql.Rows)
}

func passRowsToOpaqueConsumer(rows *sql.Rows, consumer rowsConsumer) {
	consumer.Consume(rows)
}

func rowsPassedToOpaqueConsumer(ctx context.Context, database *sql.DB, consumer rowsConsumer) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	passRowsToOpaqueConsumer(rows, consumer)
	return nil
}

func leakedStatement(ctx context.Context, database *sql.DB) error {
	statement, err := database.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released on every return path"
	if err != nil {
		return err
	}
	_ = statement
	return nil
}

func transactionOwnsStatement(ctx context.Context, transaction *sql.Tx) error {
	_, err := transaction.PrepareContext(ctx, "SELECT 1")
	return err
}

// A fatal Error assertion on the acquisition error stops the test unless the
// transaction failed to begin, so the discarded result never owns anything.
func transactionRequiredToFail(t *testing.T, ctx context.Context, database *sql.DB) {
	_, err := database.BeginTx(ctx, nil)
	require.Error(t, err)
}

func transactionDiscardedWithoutAssertion(ctx context.Context, database *sql.DB) error {
	_, err := database.BeginTx(ctx, nil) // want "owned resource from sql.BeginTx is not released on every return path"
	return err
}

// A deferred imported helper summarized as invoking its callback parameter on
// every return runs the bound Close, so the rows are released.
func rowsClosedByImportedInvoker(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	defer resourcedep.IgnoreErrorFunc(rows.Close)
	for rows.Next() {
	}
	return rows.Err()
}

func rowsMaybeClosedByImportedInvoker(ctx context.Context, database *sql.DB, enabled bool) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	defer resourcedep.MaybeIgnoreErrorFunc(rows.Close, enabled)
	return rows.Err()
}
