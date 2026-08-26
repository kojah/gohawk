package globalstate

import (
	"errors"
	"regexp"
	"sync"
)

var values = map[string]string{} // want "mutable package state values"

var replaceable = func() {} // want "mutable package state replaceable"

var _ = func() {}

//gohawk:ignore globalstate fixture intentionally shares a lookup table
var genericallyAllowed = map[string]string{}

//gohawk:ignore globalstate
var genericMissingRationale = map[string]string{} // want "mutable package state genericMissingRationale"

//gohawk:globalstate test fixture intentionally exercises shared state
var allowedForProcessFixture = map[string]string{}

//gohawk:globalstate
var missingRationale = map[string]string{} // want "mutable package state missingRationale"

var (
	errSentinel = errors.New("sentinel")
	pattern     = regexp.MustCompile("x")
	once        sync.Once

	//gohawk:globalstate guarded by fixture lifecycle
	allowedInGroup = []string{}
)
