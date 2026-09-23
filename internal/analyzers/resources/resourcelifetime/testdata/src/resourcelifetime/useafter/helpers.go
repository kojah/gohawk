package useafter

import (
	"bufio"
	"context"
	"database/sql"
	"os"
	"resourcedep"
)

// A helper summarized as calling an invalidating operation on the value it
// is handed, on every path, is that operation: passing a closed file to it
// is a use after Close. The helper's summary names the method; whether the
// method invalidates is decided here from the file's own contract.

func readThroughImportedHelper(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	return resourcedep.ReadFirst(file) // want "resource from os.Create is used after Close"
}

// Drain reads from the file on every path.
func Drain(file *os.File) error {
	_, err := file.Read(make([]byte, 1))
	return err
}

func readThroughLocalHelper(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	return Drain(file) // want "resource from os.Create is used after Close"
}

// A helper that reads on some path only requires nothing.
func readSometimesThroughHelper(path string, enabled bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	return resourcedep.ReadSometimes(file, enabled)
}

// Err is allowed after Close, through a helper as directly.
func errThroughHelper(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	_ = rows.Close()
	return resourcedep.LastErr(rows)
}

// A helper that reads a file of its own does not read the one it was
// handed.
func readReplacedThroughHelper(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	return resourcedep.ReadReplaced(file, path)
}

type holder struct{ file *os.File }

// ReadHeld reads the file inside the holder.
func ReadHeld(h holder) error {
	_, err := h.file.Read(make([]byte, 1))
	return err
}

// The helper is handed a wrapper, not the file: the requirement is on a
// path beneath the argument, which this check does not consume.
func readWrappedThroughHelper(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	return ReadHeld(holder{file: file})
}

// ReadBuffered reads through a reader built around the file; the reader
// serves from its buffer when it can, so the file is not read on every
// path.
func ReadBuffered(file *os.File) error {
	reader := bufio.NewReader(file)
	_, err := reader.Read(make([]byte, 1))
	return err
}

func readBufferedThroughHelper(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	return ReadBuffered(file)
}

// A deferred Close runs after the helper, so nothing is used after it.
func deferredCloseThenHelper(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return resourcedep.ReadFirst(file)
}

// A Close on one branch does not dominate the helper call.
func branchCloseThenHelper(path string, early bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if early {
		_ = file.Close()
	}
	err = resourcedep.ReadFirst(file)
	if !early {
		_ = file.Close()
	}
	return err
}

// Next after Close returns false, so a helper that iterates the closed rows
// silently sees none: the helper requires the rows unreleased.
func iterateAfterClose(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	_ = rows.Close()
	return resourcedep.ScanAll(rows) // want "resource from sql.QueryContext is used after Close"
}

func nextAfterClose(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	_ = rows.Close()
	for rows.Next() { // want "resource from sql.QueryContext is used after Close"
	}
	return rows.Err()
}
