package useafter

import (
	"context"
	"database/sql"
	"os"
	"resourcedep"
)

// A call to a helper proven to release the exact resource on every normal
// return is a release point, like a direct Close: a helper that always
// closes, or one that closes behind a flag the call fixes to the closing
// constant, locally or through its imported summary. A helper that may
// leave the file open, that closes something else, or that is deferred does
// not release it before the use.

func closeFile(file *os.File) { _ = file.Close() }

func finishFile(file *os.File, keep bool) {
	if !keep {
		_ = file.Close()
	}
}

func readAfterHelperClose(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	closeFile(file)
	_, _ = file.Read(make([]byte, 1)) // want "resource from os.Open is used after Close"
}

func readAfterFlagHelperClose(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	finishFile(file, false)
	_, _ = file.Read(make([]byte, 1)) // want "resource from os.Open is used after Close"
}

func readAfterImportedHelperClose(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	_ = resourcedep.Close(file)
	_, _ = file.Read(make([]byte, 1)) // want "resource from os.Open is used after Close"
}

func readAfterImportedFlagHelperClose(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	_ = resourcedep.MaybeClose(file, true)
	_, _ = file.Read(make([]byte, 1)) // want "resource from os.Open is used after Close"
}

func readAfterVariableFlagHelper(path string, keep bool) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	finishFile(file, keep)
	_, _ = file.Read(make([]byte, 1))
}

func readAfterKeepingHelper(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	finishFile(file, true)
	_, _ = file.Read(make([]byte, 1))
	_ = file.Close()
}

func closeOwnFile(path string) {
	other, err := os.Open(path)
	if err != nil {
		return
	}
	_ = other.Close()
}

func readAfterHelperClosesAnotherFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	closeOwnFile(path)
	_, _ = file.Read(make([]byte, 1))
	_ = file.Close()
}

func readBeforeDeferredHelperClose(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer closeFile(file)
	_, _ = file.Read(make([]byte, 1))
}

func readAfterBranchHelperClose(path string, early bool) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if early {
		closeFile(file)
	}
	_, _ = file.Read(make([]byte, 1))
	if !early {
		_ = file.Close()
	}
}

type fileHolder struct{ file *os.File }

func closeHeld(holder *fileHolder) { _ = holder.file.Close() }

// The helper is handed a holder, not the file; which file the holder holds
// when the helper runs is the holder's business.
func readAfterHolderClosed(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	holder := &fileHolder{file: file}
	closeHeld(holder)
	_, _ = file.Read(make([]byte, 1))
}

// A helper named for closing that does not close releases nothing.
func closeLater(file *os.File) { _ = file.Name() }

func readAfterMisnamedHelper(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	closeLater(file)
	_, _ = file.Read(make([]byte, 1))
	_ = file.Close()
}

func commitTransaction(transaction *sql.Tx) error { return transaction.Commit() }

// Commit can fail before it finishes the transaction, so a helper commit is
// not a release point.
func execAfterHelperCommit(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := commitTransaction(transaction); err != nil {
		_ = transaction.Rollback()
		return err
	}
	_, err = transaction.ExecContext(ctx, "INSERT")
	return err
}
