package useafter

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"os"
)

// This file covers the opt-in use-after-release audit: an invalidating
// operation on the exact acquired value that a direct release dominates. The
// leak check is satisfied in every case so only the audit reports.

func writeAfterClose(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	_, err = file.WriteString("late") // want "resource from os.Create is used after Close"
	return err
}

// Handing the closed body to a reader is not modeled yet: only method calls
// on the exact value count as operations.
func readBodyAfterCloseThroughHelper(client *http.Client, request *http.Request) ([]byte, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	_ = response.Body.Close()
	return io.ReadAll(response.Body)
}

func bodyReadAfterClose(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	_, err = response.Body.Read(make([]byte, 1)) // want "resource from http.Do is used after Close"
	return err
}

func execAfterCommit(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	_, err = transaction.ExecContext(ctx, "INSERT") // want "resource from sql.BeginTx is used after Commit"
	return err
}

// rows.Err after rows.Close is the documented way to read the final error.
func errAfterClose(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	_ = rows.Close()
	return rows.Err()
}

// Rollback after a failed Commit is a harmless idiom.
func rollbackAfterFailedCommit(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		_ = transaction.Rollback()
		return err
	}
	return nil
}

// A release on one branch does not dominate a use after the merge.
func closeOnOneBranch(path string, early bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if early {
		_ = file.Close()
	}
	_, err = file.WriteString("maybe")
	return err
}

// A variable rebound to a fresh acquisition is a different value.
func reopenAfterClose(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	file, err = os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.WriteString("fresh")
	return err
}

// An explicit Close under a deferred Close is the common success-path idiom.
func explicitCloseUnderDefer(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString("data"); err != nil {
		return err
	}
	return file.Close()
}
