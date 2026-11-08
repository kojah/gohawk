package lockdemo

import "sync"

var accounts sync.Mutex
var ledger sync.Mutex

func Transfer() {
	accounts.Lock()
	defer accounts.Unlock()
	ledger.Lock()
	defer ledger.Unlock()
	// Update the accounts and ledger.
}

func Snapshot() {
	ledger.Lock()
	defer ledger.Unlock()
	accounts.Lock()
	defer accounts.Unlock()
	// Read the accounts and ledger together.
}
