package resourceforward

import "resourcedep"

func NilResult() error           { return resourcedep.NilResult() }
func TypedNilResult() error      { return resourcedep.TypedNilResult() }
func TrueResult() bool           { return resourcedep.TrueResult() }
func FalseResult() bool          { return resourcedep.FalseResult() }
func MixedResult(yes bool) error { return resourcedep.MixedResult(yes) }
func MutableResult() error       { return resourcedep.MutableResult() }
func DeferredResult() error      { return resourcedep.DeferredResult() }
