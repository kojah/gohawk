package globalstateconfig

type Registry map[string]string

var cache = map[string]string{}
var registry = Registry{}
var remaining = map[string]string{} // want "mutable package state remaining"
