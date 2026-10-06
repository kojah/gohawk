package opaquewriter

import (
	"sync"
	"sync/atomic"
)

type Writer struct{ sync.RWMutex }

var counter atomic.Uint64

func (w *Writer) Enter() {
	w.RWMutex.Lock()
	counter.Add(1)
}

func (w *Writer) Read() { counter.Add(1) }

func (w *Writer) Empty() {}
