// A reference to main prevents proving a single invocation, even when the
// referencing helper is not known to run. Cancellation remains reportable.
package main

import "context"

func main() {
	ctx, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called on every return path"
	_, _ = ctx, cancel
}

func again() { main() }
