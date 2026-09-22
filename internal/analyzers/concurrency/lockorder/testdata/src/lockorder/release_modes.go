package lockorder

import "sync"

// Release modes: sync.RWMutex keeps separate reader and writer state, so
// releasing with the method that does not match the acquisition is fatal at
// run time rather than merely untidy.
//
// The acquisition has to be visible on the path for the mode to be known, so a
// lock a function did not take is left alone.

type releaseModes struct {
	rw    sync.RWMutex
	plain sync.Mutex
}

func (r *releaseModes) writeReleasedAsRead() {
	r.rw.Lock()
	defer r.rw.RUnlock() // want "lock \\(\\*lockorder.releaseModes\\).writeReleasedAsRead.r.rw is acquired with Lock and released with RUnlock"
}

func (r *releaseModes) readReleasedAsWrite() {
	r.rw.RLock()
	defer r.rw.Unlock() // want "lock \\(\\*lockorder.releaseModes\\).readReleasedAsWrite.r.rw is acquired with RLock and released with Unlock"
}

// Accepted: matching pairs.
func (r *releaseModes) write() {
	r.rw.Lock()
	defer r.rw.Unlock()
}

func (r *releaseModes) read() {
	r.rw.RLock()
	defer r.rw.RUnlock()
}

// Accepted: a plain mutex has only one mode.
func (r *releaseModes) plainWrite() {
	r.plain.Lock()
	defer r.plain.Unlock()
}

// Accepted: both modes used one after another, each paired correctly.
func (r *releaseModes) readThenWrite() {
	r.rw.RLock()
	r.rw.RUnlock()
	r.rw.Lock()
	r.rw.Unlock()
}

// Accepted: the mode differs by branch, and each branch pairs correctly. The
// walk keeps the two paths apart, so neither borrows the other's mode.
func (r *releaseModes) byBranch(writing bool) {
	if writing {
		r.rw.Lock()
		defer r.rw.Unlock()
		return
	}
	r.rw.RLock()
	defer r.rw.RUnlock()
}

// Accepted: a helper releasing a lock its caller took has no acquisition here,
// so the mode is unknown rather than mismatched.
func (r *releaseModes) releaseBorrowed() {
	r.rw.Unlock()
}

func (r *releaseModes) releaseBorrowedRead() {
	r.rw.RUnlock()
}
