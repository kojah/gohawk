package resourcedep

import "database/sql"

func AdvanceResultSet(rows *sql.Rows) bool {
	return rows.NextResultSet()
}

func ForwardAdvanceResultSet(rows *sql.Rows) bool {
	return AdvanceResultSet(rows)
}
