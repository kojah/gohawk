// Package analyzerbase provides implementation infrastructure shared by
// gohawk's analyzer groups. It is not part of the public analyzer catalog API.
package analyzerbase

import (
	"fmt"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

// Check identifies one independently configurable diagnostic rule.
type Check string

const (
	CheckAPIParameterCount        Check = "apishape/parameter-count"
	CheckAPIMixedReceivers        Check = "apishape/mixed-receivers"
	CheckAPIAdjacentSameType      Check = "apishape/adjacent-same-type"
	CheckAPIAdjacentOptional      Check = "apishape/adjacent-optional-scalars"
	CheckContextFirst             Check = "contextpolicy/context-first"
	CheckContextStorage           Check = "contextpolicy/context-storage"
	CheckContextTestOwnership     Check = "contextpolicy/test-context"
	CheckContextNilArgument       Check = "contextpolicy/nil-context"
	CheckClosedStringDomain       Check = "closedomain/closed-string-domain"
	CheckWireKeyedLiteral         Check = "wirepolicy/keyed-literal"
	CheckWireSerializationTag     Check = "wirepolicy/serialization-tag"
	CheckCancellationRelease      Check = "cancellationownership/release"
	CheckChannelCapacityRationale Check = "channelpolicy/capacity-rationale"
	CheckChannelCallerClose       Check = "channelpolicy/caller-close"
	CheckChannelSendAfterClose    Check = "channelpolicy/send-after-close"
	CheckDeferCleanupInLoop       Check = "deferinloop/cleanup-lifetime"
	CheckExitSkipsDefer           Check = "exitpolicy/skipped-defer"
	CheckGoroutineJoin            Check = "goroutineownership/unjoined"
	CheckGoroutineDetached        Check = "goroutineownership/detached"
	CheckGoroutineProducerSend    Check = "goroutineownership/abandoned-send"
	CheckProcessWait              Check = "processownership/missing-wait"
	CheckResourceRelease          Check = "resourcelifetime/missing-release"
	CheckConcurrentCapture        Check = "concurrentcapture/shared-capture"
	CheckDeterministicMapOutput   Check = "determinism/map-output-order"
	CheckErrorLogAndReturn        Check = "errorownership/log-and-return"
	CheckErrorTextClassification  Check = "errorownership/text-classification"
	CheckErrorMismatchedInline    Check = "errorownership/mismatched-inline-error"
	CheckEvaluationOrder          Check = "evalorder/operand-mutation"
	CheckMutableGlobalState       Check = "globalstate/mutable-package-state"
	CheckLockMissingRelease       Check = "lockorder/missing-release"
	CheckLockRecursiveAcquire     Check = "lockorder/recursive-acquire"
	CheckLockContradictoryOrder   Check = "lockorder/contradictory-order"
	CheckOnceDiscardedWrapper     Check = "oncepolicy/discarded-wrapper"
	CheckSyncMapNonAtomicClaim    Check = "syncmapatomicity/non-atomic-claim"
	CheckTaintUntrustedSink       Check = "taintpolicy/untrusted-sink"
	CheckBlockingTestSend         Check = "blockingtest/send"
	CheckBlockingTestReceive      Check = "blockingtest/receive"
	CheckBlockingTestSelect       Check = "blockingtest/select"
	CheckTestHelperMarker         Check = "testpolicy/helper-marker"
)

// Reportf reports a diagnostic with a precise source range.
func Reportf(pass *analysis.Pass, check Check, position token.Pos, format string, args ...any) {
	source := analysisutil.SourceRange(pass, position)
	Report(pass, check, analysis.Diagnostic{
		Pos:     source.Pos(),
		End:     source.End(),
		Message: fmt.Sprintf(format, args...),
	})
}

// Report associates diagnostic with check before reporting it.
func Report(pass *analysis.Pass, check Check, diagnostic analysis.Diagnostic) {
	diagnostic.Category = string(check)
	pass.Report(diagnostic)
}

// ChoiceValue validates a flag whose value belongs to a closed set.
type ChoiceValue struct {
	value   *string
	allowed map[string]bool
}

// NewChoiceValue creates a validated single-choice flag value.
func NewChoiceValue(value *string, allowed ...string) *ChoiceValue {
	choices := make(map[string]bool, len(allowed))
	for _, choice := range allowed {
		choices[choice] = true
	}
	return &ChoiceValue{value: value, allowed: choices}
}

func (choice *ChoiceValue) String() string {
	if choice == nil || choice.value == nil {
		return ""
	}
	return *choice.value
}

func (choice *ChoiceValue) Set(value string) error {
	if !choice.allowed[value] {
		return fmt.Errorf("unknown value %q", value)
	}
	*choice.value = value
	return nil
}

// CommaSeparatedChoice validates each member of a comma-separated flag.
type CommaSeparatedChoice struct {
	value   *string
	allowed map[string]bool
}

// NewCommaSeparatedChoice creates a validated comma-separated flag value.
func NewCommaSeparatedChoice(value *string, allowed ...string) *CommaSeparatedChoice {
	choices := make(map[string]bool, len(allowed))
	for _, choice := range allowed {
		choices[choice] = true
	}
	return &CommaSeparatedChoice{value: value, allowed: choices}
}

func (choice *CommaSeparatedChoice) String() string {
	if choice == nil || choice.value == nil {
		return ""
	}
	return *choice.value
}

func (choice *CommaSeparatedChoice) Set(value string) error {
	for item := range CommaSeparatedSet(value) {
		if !choice.allowed[item] {
			return fmt.Errorf("unknown value %q", item)
		}
	}
	*choice.value = value
	return nil
}

// CommaSeparatedSet parses non-empty comma-separated items.
func CommaSeparatedSet(value string) map[string]bool {
	result := make(map[string]bool)
	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result[item] = true
		}
	}
	return result
}
