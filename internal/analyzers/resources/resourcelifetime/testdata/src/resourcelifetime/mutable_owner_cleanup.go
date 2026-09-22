package resourcelifetime

import (
	"database/sql"
	"errors"
)

// Calling a closure over a separate aggregate owner is an uncertainty boundary,
// not proof that it closes the resource. A closure that only reads that owner
// may therefore hide a leak; direct resource captures stay within the proof.

type mutableTransactionOwner struct {
	tx   *sql.Tx
	done bool
}

func (owner *mutableTransactionOwner) rollback() error {
	if owner.done || owner.tx == nil {
		return nil
	}
	owner.done = true
	return owner.tx.Rollback()
}

func transactionOwnerCapturedBeforeAcquisition(db *sql.DB, fail bool) (*mutableTransactionOwner, error) {
	owner := &mutableTransactionOwner{}
	abort := func() (*mutableTransactionOwner, error) {
		return nil, owner.rollback()
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	owner.tx = tx
	if fail {
		return abort()
	}
	return owner, nil
}

func transactionOwnerUnrelatedClosure(db *sql.DB, fail bool) (*mutableTransactionOwner, error) {
	other := &mutableTransactionOwner{}
	abort := func() (*mutableTransactionOwner, error) {
		return nil, other.rollback()
	}
	tx, err := db.Begin() // want "owned resource from sql.Begin is not released on every return path"
	if err != nil {
		return nil, err
	}
	owner := &mutableTransactionOwner{tx: tx}
	if fail {
		return abort()
	}
	if owner.done {
		return nil, errors.New("discarded")
	}
	return owner, nil
}
