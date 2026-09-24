package resourcelifetime

import (
	"database/sql"
	"resourcedep"
)

// The false result closes Rows inside database/sql. A true result leaves
// another result set available, so that path still needs explicit cleanup.
func rowsNextResultSetClosed(db *sql.DB) {
	rows, err := db.Query("SELECT 1")
	if err != nil {
		return
	}
	if rows.NextResultSet() {
		rows.Close()
	}
}

func rowsNextResultSetStillOpen(db *sql.DB) {
	rows, err := db.Query("SELECT 1") // want "owned resource from sql.Query is not released"
	if err != nil {
		return
	}
	if rows.NextResultSet() {
		return
	}
}

func rowsNextResultSetOtherReceiver(db *sql.DB, other *sql.Rows) {
	rows, err := db.Query("SELECT 1") // want "owned resource from sql.Query is not released"
	if err != nil {
		return
	}
	if other.NextResultSet() {
		rows.Close()
	}
}

func rowsNextResultSetThroughHelper(db *sql.DB) {
	rows, err := db.Query("SELECT 1")
	if err != nil {
		return
	}
	if resourcedep.ForwardAdvanceResultSet(rows) {
		rows.Close()
	}
}

func rowsNextResultSetLoop(db *sql.DB) {
	rows, err := db.Query("SELECT 1")
	if err != nil {
		return
	}
	for resourcedep.ForwardAdvanceResultSet(rows) {
	}
}

type resultSetScanner struct{ rows *sql.Rows }

func (scanner *resultSetScanner) NextResultSet() bool { return scanner.rows.NextResultSet() }
func (scanner *resultSetScanner) Close() error        { return scanner.rows.Close() }

func rowsNextResultSetThroughOwner(db *sql.DB) {
	rows, err := db.Query("SELECT 1")
	if err != nil {
		return
	}
	scanner := &resultSetScanner{rows: rows}
	for scanner.NextResultSet() {
	}
}

type resultSetRows interface{ NextResultSet() bool }
type interfaceResultSetScanner struct{ rows resultSetRows }

func (scanner *interfaceResultSetScanner) NextResultSet() bool { return scanner.rows.NextResultSet() }

func rowsNextResultSetThroughInterfaceOwner(db *sql.DB) {
	rows, err := db.Query("SELECT 1")
	if err != nil {
		return
	}
	scanner := &interfaceResultSetScanner{rows: rows}
	for scanner.NextResultSet() {
	}
}
