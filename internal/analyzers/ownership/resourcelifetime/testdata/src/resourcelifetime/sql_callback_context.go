package resourcelifetime

import (
	"database/sql"
	"testing"
)

type callbackDatabase struct { db *sql.DB }

func withDatabase(t *testing.T, callbacks ...func(*callbackDatabase)) {
	for _, callback := range callbacks {
		callback := callback
		t.Run("database", func(t *testing.T) {
			db, err := sql.Open("driver", "")
			if err != nil { t.Fatal(err) }
			t.Cleanup(func() { db.Close() })
			callback(&callbackDatabase{db})
		})
	}
}

func statementWithCallerCleanup(t *testing.T) {
	withDatabase(t, func(owner *callbackDatabase) {
		_, _ = owner.db.Prepare("SELECT 1")
	})
}

func transactionStillNeedsCleanup(t *testing.T) {
	withDatabase(t, func(owner *callbackDatabase) {
		_, _ = owner.db.Begin() // want "owned resource from sql.Begin is not released"
	})
}

func withBorrowedDatabase(db *sql.DB, callback func(*callbackDatabase)) { callback(&callbackDatabase{db}) }

func statementWithoutCallerCleanup(db *sql.DB) {
	withBorrowedDatabase(db, func(owner *callbackDatabase) {
		_, _ = owner.db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
	})
}

func withWrongDatabase(db, other *sql.DB, callback func(*callbackDatabase)) {
	defer other.Close()
	callback(&callbackDatabase{db})
}

func statementWithWrongCallerCleanup(db, other *sql.DB) {
	withWrongDatabase(db, other, func(owner *callbackDatabase) {
		_, _ = owner.db.Prepare("SELECT 1") // want "owned resource from sql.Prepare is not released"
	})
}
