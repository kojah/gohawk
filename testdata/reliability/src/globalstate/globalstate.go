package globalstate

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
)

var values = map[string]string{} // want "mutable package state values"

func mutateValues() { values["key"] = "value" }

var immutableLookup = map[string]string{"key": "value"}

func lookupValue(key string) (string, bool) {
	value, ok := immutableLookup[key]
	return value, ok
}

var immutableList = []string{"first", "second"}

func listContains(value string) bool {
	for _, candidate := range immutableList {
		if candidate == value {
			return true
		}
	}
	return false
}

var immutableStandardCall = []string{"first", "second"}

func standardListContains(value string) bool {
	return slices.Contains(immutableStandardCall, value)
}

var immutableCloneSource = []string{"first", "second"}

func clonedList() []string {
	return slices.Clone(immutableCloneSource)
}

var immutableAppendSource = []string{"first", "second"}

func appendedCopy() []string {
	return append([]string(nil), immutableAppendSource...)
}

var immutableJoinedList = []string{"first", "second"}

func joinedList() string {
	return strings.Join(immutableJoinedList, ",")
}

var immutableHandlers = map[string]func(){"first": func() {}}

func runHandler(name string) {
	immutableHandlers[name]()
}

var immutableNestedLookup = map[string][]string{"first": {"value"}}

func clonedNestedValue(name string) []string {
	return slices.Clone(immutableNestedLookup[name])
}

var escapedNestedLookup = map[string][]string{"first": {"value"}} // want "mutable package state escapedNestedLookup"

func escapedNestedValue(name string) []string {
	return escapedNestedLookup[name]
}

var rangedNestedLookup = map[string][]string{"first": {"value"}} // want "mutable package state rangedNestedLookup"

func mutateRangedNestedValue() {
	for _, values := range rangedNestedLookup {
		values[0] = "changed"
	}
}

var immutableViaHelper = map[string]string{"key": "value"}

func readLookup(values map[string]string, key string) (string, bool) {
	value, ok := values[key]
	return value, ok
}

func indirectLookup(key string) (string, bool) {
	return readLookup(immutableViaHelper, key)
}

var mutatedViaHelper = map[string]string{"key": "value"} // want "mutable package state mutatedViaHelper"

func writeLookup(values map[string]string) { values["key"] = "changed" }

func mutateIndirectly() { writeLookup(mutatedViaHelper) }

var mutatedLookup = map[string]string{"key": "value"} // want "mutable package state mutatedLookup"

func mutateLookup() { delete(mutatedLookup, "key") }

var escapedLookup = map[string]string{"key": "value"} // want "mutable package state escapedLookup"

func lookupAlias() map[string]string { return escapedLookup }

var ExportedLookup = map[string]string{"key": "value"} // want "mutable package state ExportedLookup"

var nestedMutableLookup = map[string][]string{"key": {"value"}} // want "mutable package state nestedMutableLookup"

var replaceable = func() {} // want "mutable package state replaceable"

// testClock is a seam tests replace with a deterministic clock.
var testClock = func() {}

// testInput is stubbed in tests that exercise terminal behavior.
var testInput any = struct{}{}

// testedFunction is called by tests but remains mutable application state.
var testedFunction = func() {} // want "mutable package state testedFunction"

var _ = func() {}

//gohawk:ignore globalstate fixture intentionally shares a lookup table
var genericallyAllowed = map[string]string{}

//gohawk:ignore globalstate
var genericWithoutReason = map[string]string{}

//gohawk:globalstate test fixture intentionally exercises shared state
var legacyWithReason = map[string]string{} // want "mutable package state legacyWithReason"

func mutateLegacyWithReason() { legacyWithReason["key"] = "value" }

//gohawk:globalstate
var legacyWithoutReason = map[string]string{} // want "mutable package state legacyWithoutReason"

func mutateLegacyWithoutReason() { legacyWithoutReason["key"] = "value" }

var (
	errSentinel = errors.New("sentinel")
	pattern     = regexp.MustCompile("x")
	once        sync.Once

	//gohawk:globalstate guarded by fixture lifecycle
	legacyInGroup = []string{} // want "mutable package state legacyInGroup"
)

func mutateLegacyInGroup() { legacyInGroup = append(legacyInGroup, "value") }
