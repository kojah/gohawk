package namedentry

import "context"

// A main name outside package main supplies no process-lifetime evidence.
func main() {
	ctx, cancel := context.WithCancel(context.Background()) // want "cancel function from context.WithCancel is not called on every return path"
	_, _ = ctx, cancel
}
