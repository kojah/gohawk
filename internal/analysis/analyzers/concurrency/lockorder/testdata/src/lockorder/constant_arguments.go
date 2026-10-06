package lockorder

// A callee's acquisition counts at a call only if the call can reach it. A
// constant Boolean argument decides a branch on that parameter, so a lock
// taken only when reconnecting is false is not taken by a call that passes
// true. The same callee called with false still orders its lock.
// https://github.com/ovn-kubernetes/libovsdb/blob/6acd868996b9393b932a1eeeec1ea4e6c722ebe8/client/client.go#L286-L299

import "sync"

type reconnectingClient struct{ rpc, monitors sync.Mutex }

func (c *reconnectingClient) monitor(reconnecting bool) {
	if !reconnecting {
		c.rpc.Lock()
		defer c.rpc.Unlock()
	}
}

func (c *reconnectingClient) connect() {
	c.rpc.Lock()
	defer c.rpc.Unlock()
	c.monitors.Lock()
	c.monitor(true)
	c.monitors.Unlock()
}

type monitoringClient struct{ rpc, monitors sync.Mutex }

func (c *monitoringClient) monitor(reconnecting bool) {
	if !reconnecting {
		c.rpc.Lock()
		defer c.rpc.Unlock()
	}
}

func (c *monitoringClient) connect() {
	c.rpc.Lock()
	defer c.rpc.Unlock()
	c.monitors.Lock()
	c.monitor(true)
	c.monitors.Unlock()
}

func (c *monitoringClient) Monitor() {
	c.monitors.Lock()
	c.monitor(false) // want "contradictory lock order: .*monitoringClient.rpc and .*monitoringClient.monitors"
	c.monitors.Unlock()
}

// A retry that forwards the constant keeps it: the recursive call is still
// made with reconnecting true.
type retryingClient struct{ rpc, monitors sync.Mutex }

func (c *retryingClient) monitor(reconnecting bool, attempts int) {
	if !reconnecting {
		c.rpc.Lock()
		defer c.rpc.Unlock()
	}
	if attempts > 0 {
		c.monitor(reconnecting, attempts-1)
	}
}

func (c *retryingClient) connect() {
	c.rpc.Lock()
	defer c.rpc.Unlock()
	c.monitors.Lock()
	c.monitor(true, 3)
	c.monitors.Unlock()
}
