package resourcelifetime

import (
	"context"
	"database/sql"
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
