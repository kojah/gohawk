package resourcelifetime

import (
	"context"
	"database/sql"
	"os"
)

//gohawk:example flagged Leaked file
func read(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

//gohawk:example end

//gohawk:example flagged Leaked database rows
func query(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	_ = rows
	return nil
}

//gohawk:example end

//gohawk:example ok
func readSafely(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

//gohawk:example end
