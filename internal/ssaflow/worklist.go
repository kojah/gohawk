package ssaflow

// WalkStates drives a keyed work list over path-sensitive states. The caller
// owns the state type and the transfer policy: step consumes one state and
// returns the successor states it produces, or false to end the walk early.
// The driver owns termination: a state is expanded only when its key has not
// been expanded before, which bounds the walk on loops while still letting a
// block be revisited under a different obligation state. The caller's key must
// therefore capture every part of the state that changes what step does.
func WalkStates[S any, K comparable](initial []S, key func(S) K, step func(S) ([]S, bool)) {
	queue := initial
	expanded := map[K]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		identity := key(state)
		if expanded[identity] {
			continue
		}
		expanded[identity] = true
		successors, ok := step(state)
		if !ok {
			return
		}
		queue = append(queue, successors...)
	}
}
