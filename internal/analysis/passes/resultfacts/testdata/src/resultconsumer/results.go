package resultconsumer

import "resultforward"

func Nil() error            { return resultforward.Nil() }
func TypedNil() error       { return resultforward.TypedNil() }
func Pair() (bool, error)   { return resultforward.Pair() }
func Mixed(yes bool) error  { return resultforward.Mixed(yes) }
func Global() error         { return resultforward.Global() }
func Deferred() error       { return resultforward.Deferred() }
func StoredTrue() bool      { return resultforward.StoredTrue() }
func StoredTypedNil() error { return resultforward.StoredTypedNil() }
