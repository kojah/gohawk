package lockorder

import (
	"opaquewriter"
	"sync"
)

type opaqueWriterOwner struct {
	mu     sync.RWMutex
	writer *opaquewriter.Writer
	count  int
}

func (o *opaqueWriterOwner) heldImportedWriter() {
	o.mu.RLock()
	defer o.mu.RUnlock()
	o.writer.Enter()
	defer o.writer.Unlock()
	o.count++
}

func (o *opaqueWriterOwner) unrelatedOpaqueCall(other *opaquewriter.Writer) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	other.Read()
	defer o.writer.Unlock()
	o.count++ // want "write while only the read lock .* is held"
}

func (o *opaqueWriterOwner) knownEmptyCall() {
	o.mu.RLock()
	defer o.mu.RUnlock()
	o.writer.Empty()
	defer o.writer.Unlock()
	o.count++ // want "write while only the read lock .* is held"
}

func (o *opaqueWriterOwner) releasedImportedWriter() {
	o.mu.RLock()
	defer o.mu.RUnlock()
	o.writer.Enter()
	o.writer.Unlock()
	o.count++ // want "write while only the read lock .* is held"
}

func (o *opaqueWriterOwner) possibleWriterAfterWrite() {
	o.mu.RLock()
	defer o.mu.RUnlock()
	o.count++ // want "write while only the read lock .* is held"
	o.writer.Enter()
	defer o.writer.Unlock()
}

func (o *opaqueWriterOwner) earlyWriterReleaseBeforeWrite() {
	o.mu.RLock()
	defer o.mu.RUnlock()
	o.writer.Enter()
	defer o.writer.Unlock()
	o.writer.Unlock()
	o.count++ // want "write while only the read lock .* is held"
}
