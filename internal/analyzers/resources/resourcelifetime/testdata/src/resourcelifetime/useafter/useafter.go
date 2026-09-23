package useafter

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"os"
)

// This file covers the use-after-release check: an invalidating
// operation on the exact acquired value that a direct release dominates. The
// leak check is satisfied in every case so only the use-after check reports.

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

// An unsuccessful Commit need not have marked the transaction done yet.
func uncheckedCommit(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_ = transaction.Commit()
	_, err = transaction.ExecContext(ctx, "INSERT")
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

func sameFile(file *os.File) *os.File { return file }

// A release through a helper that returns its argument unchanged is a
// release of the argument, so a later operation on the original is a use
// after release.
func closedThroughPassThroughThenRead(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	_ = sameFile(file).Close()
	_, _ = file.Read(make([]byte, 1)) // want "resource from os.Open is used after Close"
}

func rewindFile(file *os.File) *os.File {
	_, _ = file.Seek(0, 0)
	return file
}

// A helper that returns the file unchanged but also operates on it is not a
// pure pass-through: its effect on the file is opaque to the scan, so the
// later read stays unknown rather than reported.
func closedThroughOperatingHelperThenRead(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	_ = rewindFile(file).Close()
	_, _ = file.Read(make([]byte, 1))
}
