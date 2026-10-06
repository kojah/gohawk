package consumer

import (
	"dependency"
	"forwarder"
	"sync"
)

func forward(a, b *sync.Mutex)            { forwarder.Pair(b, a) }
func reverse(a, b *sync.Mutex)            { forwarder.Pair(a, b) }
func opaque(a *sync.Mutex)                { dependency.Opaque(a) }
func conditional(a *sync.Mutex, yes bool) { dependency.Conditional(a, yes) }
func spawning(a *sync.Mutex)              { dependency.Spawn(a) }
func localOnly()                          { dependency.Local() }
func empty() int                          { return dependency.Empty(1) }
