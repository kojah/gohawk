package globalstate

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"golang.org/x/tools/go/analysis"
	"k8s.io/apimachinery/pkg/runtime"
	controllerscheme "sigs.k8s.io/controller-runtime/pkg/scheme"
)

var values = map[string]string{} // want "mutable package state values"

var metric = new(prometheus.GaugeVec)
var rootCommand = new(cobra.Command)
var module fx.Option
var runtimeScheme = new(runtime.Scheme)
var schemeBuilder runtime.SchemeBuilder
var controllerSchemeBuilder = new(controllerscheme.Builder)
var addToScheme = controllerSchemeBuilder.AddToScheme
var analysisPass = new(analysis.Analyzer)
var reassignedAnalysisPass = new(analysis.Analyzer) // want "mutable package state reassignedAnalysisPass"

func replaceAnalysisPass() { reassignedAnalysisPass = new(analysis.Analyzer) }

var immutableBytes = []byte("value")

func bytesValue() byte { return immutableBytes[0] }

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

type fixtureSentinelError string

func (e fixtureSentinelError) Error() string { return string(e) }

func newFixtureSentinel(message string) error { return fixtureSentinelError(message) }

var ErrExternalStyle = newFixtureSentinel("external")

var errInternalStyle = newFixtureSentinel("internal")

var currentError = errors.New("mutable slot") // want "mutable package state currentError"

var errorState = errors.New("not conventionally named") // want "mutable package state errorState"

var ErrReassigned = errors.New("initial") // want "mutable package state ErrReassigned"

func replaceSentinel() { ErrReassigned = errors.New("replacement") }

var ErrAddressed = errors.New("initial") // want "mutable package state ErrAddressed"

func sentinelAddress() *error { return &ErrAddressed }

var ErrNil error // want "mutable package state ErrNil"

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
