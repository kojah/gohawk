package databasedemo

import (
	"context"
	"database/sql"
)

// FirstName reads the first name returned by a query.
func FirstName(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM users")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var name string
	if rows.Next() {
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
	}
	return name, rows.Err()
}
