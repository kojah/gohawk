package lockorder

// A helper that locks and unlocks one mutex of an owner does not release a
// sibling mutex of the same owner that its caller holds, so the caller still
// orders the helper's lock after the one it holds. A helper that unlocks the
// held mutex itself still hands it off.
// https://github.com/ovn-kubernetes/libovsdb/blob/6acd868996b9393b932a1eeeec1ea4e6c722ebe8/client/client.go#L286-L299

import "sync"

type siblingClient struct{ rpc, monitors sync.Mutex }

func (c *siblingClient) callRPC() {
	c.rpc.Lock()
	defer c.rpc.Unlock()
}

func (c *siblingClient) reconnect() {
	c.rpc.Lock()
	defer c.rpc.Unlock()
	c.monitors.Lock()
	c.monitors.Unlock()
}

func (c *siblingClient) refresh() {
	c.monitors.Lock()
	c.callRPC() // want "contradictory lock order: .*siblingClient.rpc and .*siblingClient.monitors"
	c.monitors.Unlock()
}

type handoffClient struct{ rpc, monitors sync.Mutex }

func (c *handoffClient) unlockMonitors() { c.monitors.Unlock() }

func (c *handoffClient) reconnect() {
	c.rpc.Lock()
	defer c.rpc.Unlock()
	c.monitors.Lock()
	c.monitors.Unlock()
}

func (c *handoffClient) handoff() {
	c.monitors.Lock()
	c.unlockMonitors()
	c.rpc.Lock()
	c.rpc.Unlock()
}
