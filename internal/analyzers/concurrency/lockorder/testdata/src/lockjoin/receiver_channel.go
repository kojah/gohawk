package lockjoin

import "sync"

type channelOwner struct {
	mu   sync.Mutex
	done chan struct{}
}

func (o *channelOwner) run()   { o.mu.Lock(); close(o.done); o.mu.Unlock() }
func (o *channelOwner) start() { go o.run() }
func (o *channelOwner) stop() {
	<-o.done // want "waits for a worker that needs the held lock"
	o.mu.Unlock()
}

func receiverChannel() {
	var o channelOwner
	o.done = make(chan struct{})
	o.mu.Lock()
	o.start()
	o.stop()
}

func receiverChannelChanged() {
	var o channelOwner
	o.done = make(chan struct{})
	o.mu.Lock()
	o.start()
	o.done = make(chan struct{})
	o.stop()
}

func receiverChannelOpaque(f func(*channelOwner)) {
	var o channelOwner
	o.done = make(chan struct{})
	o.mu.Lock()
	o.start()
	f(&o)
	o.stop()
}
