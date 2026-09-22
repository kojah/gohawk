package resourcelifetime

import (
	"context"
	"database/sql"
)

func statementParentDeferred(db *sql.DB) {
	defer db.Close()
	_, _ = db.Prepare("SELECT 1")
}

func statementParentClosedLater(db *sql.DB, ctx context.Context) {
	_, _ = db.PrepareContext(ctx, "SELECT 1")
	db.Close()
}

func statementOtherParent(db, other *sql.DB) {
	defer other.Close()
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
}

func statementConditionalParent(db *sql.DB, closeIt bool) {
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
	if closeIt {
		db.Close()
	}
}

func statementConditionalPriorParent(db *sql.DB, closeIt bool) {
	if closeIt {
		defer db.Close()
	}
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
}

func rowsKeepOwnObligation(db *sql.DB) {
	defer db.Close()
	_, _ = db.Query("SELECT 1") // want "owned resource from sql.Query is not released"
}

func rowsExhausted(db *sql.DB) error {
	rows, err := db.Query("SELECT 1")
	if err != nil {
		return err
	}
	for rows.Next() {
		var value int
		_ = rows.Scan(&value)
	}
	return rows.Err()
}

func rowsEarlyBreak(db *sql.DB) {
	rows, err := db.Query("SELECT 1") // want "owned resource from sql.Query is not released"
	if err != nil {
		return
	}
	for rows.Next() {
		break
	}
}

func rowsScanError(db *sql.DB) error {
	rows, err := db.Query("SELECT 1") // want "owned resource from sql.Query is not released"
	if err != nil {
		return err
	}
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			return err
		}
	}
	return rows.Err()
}

func rowsOtherReceiver(db *sql.DB, other *sql.Rows) {
	_, _ = db.Query("SELECT 1") // want "owned resource from sql.Query is not released"
	for other.Next() {
	}
}

func canceledDBAcquisitions(db *sql.DB) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = db.PrepareContext(ctx, "SELECT 1")
	_, _ = db.QueryContext(ctx, "SELECT 1")
	_, _ = db.BeginTx(ctx, nil)
}

func canceledCauseAcquisition(db *sql.DB) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(nil)
	_, _ = db.PrepareContext(ctx, "SELECT 1")
}

func cancellationTooLate(db *sql.DB) {
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = db.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released"
	cancel()
}

func cancellationDeferred(db *sql.DB) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _ = db.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released"
}

func cancellationConditional(db *sql.DB, cancelIt bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cancelIt {
		cancel()
	}
	_, _ = db.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released"
}

func cancellationOtherContext(db *sql.DB, ctx context.Context) {
	_, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = db.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released"
}

func cancellationDetachedContext(db *sql.DB) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = db.PrepareContext(context.WithoutCancel(ctx), "SELECT 1") // want "owned resource from sql.PrepareContext is not released"
}

func statementReassignedParent(db, other *sql.DB) {
	defer db.Close()
	db = other
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
}

func statementMixedParent(db, other *sql.DB, choose bool) {
	defer db.Close()
	if choose {
		db = other
	}
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
}

func statementParentClosedAsync(db *sql.DB) {
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
	go db.Close()
}

func cancellationAsync(db *sql.DB) {
	ctx, cancel := context.WithCancel(context.Background())
	go cancel()
	_, _ = db.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released"
}

func canceledConnDoesNotUseDBEntry(conn *sql.Conn) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = conn.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released"
}

func statementCapturedParent(db *sql.DB, later func(func())) {
	defer db.Close()
	_, _ = db.Prepare("SELECT 1")
	later(func() { db.Ping() })
}

func statementCapturedParentReassigned(db, other *sql.DB, later func(func())) {
	defer db.Close()
	db = other
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
	later(func() { db.Ping() })
}

func statementCapturedParentMutated(db, other *sql.DB, mutate func(func())) {
	defer db.Close()
	mutate(func() { db = other })
	_, _ = db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
}
