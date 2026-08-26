package globalstate

import (
	"errors"
	"regexp"
	"sync"
)

var values = map[string]string{} // want "mutable package state values"

var replaceable = func() {} // want "mutable package state replaceable"

var (
	errSentinel = errors.New("sentinel")
	pattern     = regexp.MustCompile("x")
	once        sync.Once
)
