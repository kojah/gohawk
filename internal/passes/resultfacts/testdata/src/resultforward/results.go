package resultforward

import "resultdependency"

func Nil() error            { return resultdependency.Nil() }
func TypedNil() error       { return resultdependency.TypedNil() }
func Pair() (bool, error)   { return resultdependency.Pair() }
func Mixed(yes bool) error  { return resultdependency.Mixed(yes) }
func Global() error         { return resultdependency.Global() }
func Deferred() error       { return resultdependency.Deferred() }
func StoredTrue() bool      { return resultdependency.StoredTrue() }
func StoredTypedNil() error { return resultdependency.StoredTypedNil() }
