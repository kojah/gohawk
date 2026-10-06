package lifecyclefacts

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/engine/syntax"
)

// Fact descriptions render existing claims for dumps and evidence tracing.
// Parameter, field and result masks keep distinct index spaces; text labels
// describe guarantees but never participate in their inference.

// traceDetails names the claims a summary makes, so a trace shows what a
// function was summarized as without printing every mask.
func (fact *Fact) traceDetails() map[string]string {
	// The trace lists claim names rather than bits: a reader compares which
	// claims a summary makes across runs, and the index spaces differ between
	// parameter, field, and result masks, so raw bits would mislead.
	named := []struct {
		name string
		mask ParameterMask
	}{
		{"invoked", fact.InvokedParameters()},
		{"synchronously-invoked", fact.SynchronouslyInvoked()},
		{"closed", fact.MethodMask("Close")},
		{"finalized", fact.MethodMask("Finalize")},
		{"released", fact.MethodMask("Release")},
		{"shutdown", fact.MethodMask("Shutdown")},
		{"stopped", fact.MethodMask("Stop")},
		{"waited", fact.MethodMask("Wait")},
		{"committed", fact.MethodMask("Commit")},
		{"rolled-back", fact.MethodMask("Rollback")},
		{"returned-owner", fact.ReturnedOwner()},
		{"returned-view", fact.Must.ReturnedView},
		{"retained", fact.Retained()},
		{"stored", fact.Stored()},
		{"kept", fact.KeptParameters()},
		{"loop-released", fact.May.LoopReleased},
		{"discharges", fact.DischargedParameters()},
		{"owned-fields", ParameterMask(fact.Must.OwnedFields)},
		{"released-fields", ParameterMask(fact.Must.ReleasedFields)},
		{"owned-results", ParameterMask(fact.Must.OwnedResults)},
		{"retaining-results", ParameterMask(fact.Must.RetainingResults)},
		{"receiver-store", fact.ReceiverStore()},
	}
	// Structured claims have no mask; they are named only when they carry
	// an effect, so an empty summary still reads "none".
	claims := make([]string, 0, len(named))
	for _, claim := range named {
		if claim.mask != 0 {
			claims = append(claims, claim.name)
		}
	}
	if len(fact.caseDischarges()) != 0 {
		claims = append(claims, "conditional")
	}
	if fact.ReturnedCleanup != nil && len(fact.ReturnedCleanup.Effects) != 0 {
		claims = append(claims, "returned-cleanup")
	}
	if len(claims) == 0 {
		return map[string]string{"claims": "none"}
	}
	return map[string]string{"claims": strings.Join(claims, ",")}
}

// DescribeFact renders the summary for the fact dump: one line per parameter
// that some mask covers, named from the function's signature. Mask positions
// follow SSA parameters, so a method's receiver is position zero.
func (fact *Fact) DescribeFact(object types.Object) []string {
	function, ok := object.(*types.Func)
	if !ok {
		return nil
	}
	// A decoded fact carries no signature; the dump supplies it so the
	// claims read from the heap projection are listed too.
	if fact.signature == nil {
		fact.signature = function.Signature()
	}
	var names []string
	signature := function.Signature()
	if signature.Recv() != nil {
		names = append(names, signature.Recv().Name())
	}
	for parameter := range signature.Params().Variables() {
		names = append(names, parameter.Name())
	}
	// Each mask family is indexed differently: parameters by position,
	// field masks by struct field, and result masks by result position. The
	// dump names each index in its own space so a field bit is never read as
	// a parameter.
	var lines []string
	for index, name := range names {
		if masks := fact.parameterMasks(index); len(masks) > 0 {
			lines = append(lines, fmt.Sprintf("%d %s: %s", index, name, strings.Join(masks, ", ")))
		}
	}
	for _, mask := range fieldMasks {
		if fields := fact.fieldNames(*mask.field(fact), signature); len(fields) > 0 {
			lines = append(lines, fmt.Sprintf("%s: %s", mask.name, strings.Join(fields, ", ")))
		}
	}
	if fact.Must.OwnedResults != 0 {
		lines = append(lines, "OwnedResults: "+fact.resultNames(fact.Must.OwnedResults, signature))
	}
	if fact.Must.RetainingResults != 0 {
		lines = append(lines, "RetainingResults: "+fact.resultNames(fact.Must.RetainingResults, signature))
	}
	for _, discharge := range fact.unconditionalDischarges() {
		if discharge.Path != "" && discharge.Parameter < len(names) {
			lines = append(lines, fmt.Sprintf("%d %s: %s at %s", discharge.Parameter, names[discharge.Parameter], discharge.Method, discharge.Path))
		}
	}
	lines = append(lines, fact.conditionalDescriptions()...)
	lines = append(lines, fact.returnedCleanupDescriptions()...)
	if len(lines) == 0 {
		lines = []string{"no parameter is proven on every return"}
	}
	return lines
}

// DescribeHeap renders the heap projection for the fact dump, which prints it
// after the claims or on its own.
func (fact *Fact) DescribeHeap(types.Object) []string {
	return fact.heapDescriptions()
}

// heapDescriptions renders the heap projection one entry per line, after
// the mask claims: the masks say what the function proved about its
// parameters, the projection says what the caller's objects look like
// afterwards, and a reader of the dump wants the proof before the picture.
func (fact *Fact) heapDescriptions() []string {
	if fact.Heap == nil {
		return nil
	}
	var lines []string
	for line := range strings.SplitSeq(fact.Heap.String(), "\n") {
		if line != "" {
			lines = append(lines, "heap "+line)
		}
	}
	return lines
}

// resultNames renders a result mask as result positions with their types.
func (fact *Fact) resultNames(mask ResultMask, signature *types.Signature) string {
	var names []string
	for index := range signature.Results().Len() {
		if mask.contains(index) {
			names = append(names, fmt.Sprintf("%d %s", index, signature.Results().At(index).Type()))
		}
	}
	return strings.Join(names, ", ")
}

// fieldNames renders a field mask against the method's receiver struct or,
// for a function, the struct behind its first pointer result.
func (fact *Fact) fieldNames(mask FieldMask, signature *types.Signature) []string {
	if mask == 0 {
		return nil
	}
	var structure *types.Struct
	if signature.Recv() != nil {
		structure = syntax.PointerStruct(signature.Recv().Type())
	} else {
		for result := range signature.Results().Variables() {
			if structure = syntax.PointerStruct(result.Type()); structure != nil {
				break
			}
		}
	}
	if structure == nil {
		return nil
	}
	var names []string
	for index := range structure.NumFields() {
		if mask.contains(index) {
			names = append(names, structure.Field(index).Name())
		}
	}
	return names
}

func (fact *Fact) parameterMasks(index int) []string {
	var names []string
	for _, discharge := range dischargeNames {
		if fact.MethodMask(discharge.method).contains(index) {
			names = append(names, discharge.name)
		}
	}
	for _, mask := range lifecycleMasks {
		if mask.mask(fact).contains(index) {
			names = append(names, mask.name)
		}
	}
	return names
}

// String decodes the masks by parameter position so the fact is readable in
// analysis debug output.
func (fact *Fact) String() string {
	var parts []string
	for index := range 64 {
		if masks := fact.parameterMasks(index); len(masks) > 0 {
			parts = append(parts, fmt.Sprintf("%d:%s", index, strings.Join(masks, "+")))
		}
	}
	for _, mask := range fieldMasks {
		if value := *mask.field(fact); value != 0 {
			parts = append(parts, fmt.Sprintf("%s:%#x", mask.name, uint64(value)))
		}
	}
	parts = append(parts, fact.conditionalDescriptions()...)
	parts = append(parts, fact.returnedCleanupDescriptions()...)
	if len(parts) == 0 {
		return "lifecycle summary: none"
	}
	return "lifecycle summary: " + strings.Join(parts, " ")
}

// lifecycleMask names one parameter claim for the fact dump and the trace.
type lifecycleMask struct {
	name string
	mask func(*Fact) ParameterMask
}

var lifecycleMasks = []lifecycleMask{
	{name: "SynchronouslyInvoked", mask: (*Fact).SynchronouslyInvoked},
	{name: "ReturnedOwner", mask: (*Fact).ReturnedOwner},
	{name: "ReturnedView", mask: func(fact *Fact) ParameterMask { return fact.Must.ReturnedView }},
	{name: "ReceiverStore", mask: (*Fact).ReceiverStore},
	{name: "Retained", mask: (*Fact).Retained},
	{name: "Stored", mask: (*Fact).Stored},
	{name: "LoopReleased", mask: func(fact *Fact) ParameterMask { return fact.May.LoopReleased }},
}

// dischargeNames labels empty-path discharges in the fact dump, in the order
// the dump has always listed them.
var dischargeNames = []struct{ name, method string }{
	{"Invoked", InvokeMethod},
	{"Closed", "Close"},
	{"Finalized", "Finalize"},
	{"Released", "Release"},
	{"Shutdown", "Shutdown"},
	{"Stopped", "Stop"},
	{"Waited", "Wait"},
	{"Committed", "Commit"},
	{"RolledBack", "Rollback"},
}

// fieldMasks are indexed by struct field of the result or receiver type.
var fieldMasks = []struct {
	name  string
	field func(*Fact) *FieldMask
}{
	{name: "OwnedFields", field: func(fact *Fact) *FieldMask { return &fact.Must.OwnedFields }},
	{name: "ReleasedFields", field: func(fact *Fact) *FieldMask { return &fact.Must.ReleasedFields }},
}
