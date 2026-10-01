package resourcelifetime

import (
	"context"
	"database/sql"
	"time"
)

func transactionDeferredTimeout(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := db.BeginTx(ctx, nil)
	return err
}

func timedContextCanceledBeforeQuery(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cancel()
	_, err := db.QueryContext(ctx, "SELECT 1")
	return err
}

func transactionDeferredDeadline(db *sql.DB) error {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	_, err := db.BeginTx(ctx, nil)
	defer cancel()
	return err
}

func transactionDirectCancel(db *sql.DB) error {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := db.BeginTx(ctx, nil)
	cancel()
	return err
}

func transactionCancelCause(conn *sql.Conn) error {
	ctx, cancel := context.WithCancelCause(context.Background())
	_, err := conn.BeginTx(ctx, nil)
	cancel(nil)
	return err
}

func transactionTimeoutCause(conn *sql.Conn) error {
	ctx, cancel := context.WithTimeoutCause(context.Background(), time.Minute, context.Canceled)
	defer cancel()
	_, err := conn.BeginTx(ctx, nil)
	return err
}

func transactionDeadlineCause(db *sql.DB) error {
	ctx, cancel := context.WithDeadlineCause(context.Background(), time.Now().Add(time.Minute), context.Canceled)
	defer cancel()
	_, err := db.BeginTx(ctx, nil)
	return err
}

func transactionWrongContext(db *sql.DB, other context.Context) error {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := db.BeginTx(other, nil) // want "owned resource from sql.BeginTx is not released on every return path"
	return err
}

func transactionConditionalCancel(db *sql.DB, cleanup bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := db.BeginTx(ctx, nil) // want "owned resource from sql.BeginTx is not released on every return path"
	if cleanup {
		cancel()
	}
	return err
}

func transactionPriorConditionalCancel(db *sql.DB, cleanup bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	if cleanup {
		defer cancel()
	}
	_, err := db.BeginTx(ctx, nil) // want "owned resource from sql.BeginTx is not released on every return path"
	return err
}

func transactionBeginIgnoresContext(db *sql.DB) error {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := db.Begin() // want "owned resource from sql.Begin is not released on every return path"
	return err
}

func transactionReplacedContext(db *sql.DB) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.Background()
	_, err := db.BeginTx(ctx, nil) // want "owned resource from sql.BeginTx is not released on every return path"
	return err
}
