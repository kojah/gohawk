// Package goroutineownershiplifecycle supplies imported callback wrappers
// for the goroutineownership fixtures: one calls its callback, the other
// starts it on a goroutine.
package goroutineownershiplifecycle

func InvokeSynchronously(callback func()) { callback() }

func InvokeAsynchronously(callback func()) {
	go callback()
}
