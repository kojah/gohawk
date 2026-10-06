package lockorder

import "sync"

// Release modes: the lock walk tracks whether each held lock was taken with
// Lock or RLock, which read-lock-write depends on. These accepted cases pin
// that mode tracking and keep missing-release quiet on matched pairs.
//
// Known gap: releasing with the method that does not match the acquisition,
// such as RLock followed by Unlock, is fatal at run time but no longer
// reported. The experimental mismatched-release check was retired: the crash
// happens on the first run of that path, so the bug rarely ships, and it found
// one true positive in about 1,000 audited repositories.

type releaseModes struct {
	rw    sync.RWMutex
	plain sync.Mutex
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
// so it is a borrowed lock, not a missing or mismatched release.
func (r *releaseModes) releaseBorrowed() {
	r.rw.Unlock()
}

func (r *releaseModes) releaseBorrowedRead() {
	r.rw.RUnlock()
}

func (r *releaseModes) deferredReadRestoration() {
	r.rw.RLock()
	defer r.rw.RUnlock()
	r.rw.RUnlock()
	defer r.rw.RLock()
	r.rw.Lock()
	defer r.rw.Unlock()
}

func (c *readCache) deferredReadRestorationWrite() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.mu.RUnlock()
	defer c.mu.RLock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits++
}

func (c *readCache) deferredLockDoesNotProtect() {
	c.mu.RLock()
	defer c.mu.RUnlock()
	defer c.mu.Lock()
	c.hits++ // want "write while only the read lock .* is held"
}
