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
