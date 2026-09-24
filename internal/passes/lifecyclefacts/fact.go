package lifecyclefacts

import (
	"fmt"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"

	"github.com/kojah/gohawk/internal/lifecycle"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Fact is the compact cross-package ownership summary exported for a
// function. Each bit identifies an SSA parameter position. This package is
// internal analysis infrastructure, not a public extension API.
type Fact struct {
	Invoked              ParameterMask
	SynchronouslyInvoked ParameterMask
	Closed               ParameterMask
	Finalized            ParameterMask
	Released             ParameterMask
	Shutdown             ParameterMask
	Stopped              ParameterMask
	Waited               ParameterMask
	Committed            ParameterMask
	RolledBack           ParameterMask
	ReturnedOwner        ParameterMask
	// ReturnedView narrows ReturnedOwner: the parameter is stored in the
	// returned struct, but no method of that type releases the field, so the
	// caller keeps the obligation. See fields.go.
	ReturnedView ParameterMask
	// Retained marks parameters the callee may keep beyond the call; see
	// retention.go for the over-approximation it deliberately makes.
	Retained ParameterMask
	// Stored is the strict form of Retained: positive structural evidence that
	// the callee keeps the parameter, safe to treat as an ownership transfer.
	Stored ParameterMask
	// Kept widens Retained to what is loaded out of a struct-shaped
	// parameter, by access path, so a caller can ask whether the resource it
	// stored at one path may outlive the call. See contents.go.
	Kept []Kept
	// LoopReleased marks parameters whose derived values the callee releases
	// inside a loop, as a variadic close helper does to each of its files. It
	// is a may-claim: which element an iteration releases is decided by
	// iteration, so a consumer treats the call as unknown, never as settled.
	LoopReleased ParameterMask
	// OwnedFields and ReleasedFields are indexed by struct field, not
	// parameter; see fields.go for the constructor and method summaries.
	OwnedFields    ParameterMask
	ReleasedFields ParameterMask
	// OwnedResults is indexed by result position: the function hands back a
	// fresh resource it acquired itself, and the caller owes its cleanup.
	// See owned_results.go for the freshness the proof requires.
	OwnedResults ParameterMask
	// Discharges are the exact cleanup claims: which method is called, on
	// which parameter, at which access path beneath it, on every normal
	// return. The method masks above are the empty-path discharges; a
	// cleanup of a field or element is recorded here and nowhere else, so a
	// caller matches the resource it stored at that path rather than any
	// resource the argument contains.
	Discharges    []Discharge
	ReceiverStore ParameterMask
	// Conditional holds positive, result-specific guarantees. It never widens
	// an unconditional mask, and missing entries do not establish no effect.
	Conditional *ConditionalSummary
	// Heap is the projection of the function's points-to graph onto what a
	// caller can name: where each parameter, result, and global slot may
	// point at exit, how each object escaped or was released, what was
	// read, and where the projection was cut. See heap.go.
	Heap *heapmodel.HeapSummary
	// ReturnedCleanup relates an invoked callback result to an exact factory
	// parameter or sibling result. Merely returning the callback does not clean up.
	ReturnedCleanup *ReturnedCleanupSummary
}

// traceDetails names the claims a summary makes, so a trace shows what a
// function was summarized as without printing every mask.
func (fact *Fact) traceDetails() map[string]string {
	named := []struct {
		name string
		mask ParameterMask
	}{
		{"invoked", fact.Invoked},
		{"synchronously-invoked", fact.SynchronouslyInvoked},
		{"closed", fact.Closed},
		{"finalized", fact.Finalized},
		{"released", fact.Released},
		{"shutdown", fact.Shutdown},
		{"stopped", fact.Stopped},
		{"waited", fact.Waited},
		{"committed", fact.Committed},
		{"rolled-back", fact.RolledBack},
		{"returned-owner", fact.ReturnedOwner},
		{"returned-view", fact.ReturnedView},
		{"retained", fact.Retained},
		{"stored", fact.Stored},
		{"kept", fact.KeptParameters()},
		{"loop-released", fact.LoopReleased},
		{"discharges", fact.DischargedParameters()},
		{"owned-fields", fact.OwnedFields},
		{"released-fields", fact.ReleasedFields},
		{"owned-results", fact.OwnedResults},
		{"receiver-store", fact.ReceiverStore},
	}
	claims := make([]string, 0, len(named))
	for _, claim := range named {
		if claim.mask != 0 {
			claims = append(claims, claim.name)
		}
	}
	if fact.Conditional != nil && len(fact.Conditional.Effects) != 0 {
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

// Discharge is one exact cleanup claim: Method is called on the value at
// Path beneath Parameter on every normal return. Path is a joined access
// path, empty for the parameter itself.
type Discharge struct {
	Parameter int
	Method    string
	Path      string
}

// DischargedParameters returns the parameters with any discharge, at any
// path, for a consumer that only asks whether the callee releases part of
// what it was handed.
func (fact *Fact) DischargedParameters() ParameterMask {
	var mask ParameterMask
	for _, discharge := range fact.Discharges {
		mask |= parameterMaskFor(discharge.Parameter)
	}
	return mask
}

// KeptParameters returns the parameters with a kept-contents claim at any
// path.
func (fact *Fact) KeptParameters() ParameterMask {
	var mask ParameterMask
	for _, kept := range fact.Kept {
		mask |= parameterMaskFor(kept.Parameter)
	}
	return mask
}

// keepsContentsAt reports whether the summary claims that the contents at
// path beneath the parameter at index may be kept beyond the call. The
// empty path asks about the whole parameter.
func (fact *Fact) keepsContentsAt(index int, path string) bool {
	var kept []string
	for _, claim := range fact.Kept {
		if claim.Parameter == index {
			kept = append(kept, claim.Path)
		}
	}
	return keepsContentsAt(kept, path)
}

// dischargesArgument reports whether the call's static callee is summarized
// as calling method on exactly the target: the target is the argument
// itself for an empty-path discharge, or the value the caller stored at the
// discharge's path beneath the argument. Containment alone proves nothing
// here; that is the whole point of the path.
func (fact *Fact) dischargesArgument(instruction ssa.Instruction, target ssa.Value, method string, observer ssaflow.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for _, discharge := range fact.Discharges {
		if discharge.Method != method || discharge.Parameter >= len(common.Args) {
			continue
		}
		argument := common.Args[discharge.Parameter]
		storage := heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget).Observed(observer))
		path := ssaflow.SplitAccessPath(discharge.Path)
		if stored, ok := heapmodel.ValueAtPath(argument, path, instruction); ok && storage.Same(stored, target).Proven() {
			return true
		}
		// A resource that is an owner, such as an http.Response, is released
		// through its cleanup-bearing field: a helper closing resp.Body has
		// released resp. Only a direct resource-typed field qualifies; a
		// deeper path or an ordinary field is not the owner's cleanup.
		if len(path) == 1 && storage.Same(argument, target).Proven() && cleanupFieldPath(target.Type(), path[0]) {
			return true
		}
	}
	return false
}

// cleanupFieldPath reports whether step selects a field of owner whose type
// carries a cleanup obligation of its own.
func cleanupFieldPath(owner types.Type, step string) bool {
	index, ok := strings.CutPrefix(step, "field:")
	if !ok {
		return false
	}
	structure := structBehind(owner)
	if structure == nil {
		return false
	}
	for field := range structure.NumFields() {
		if strconv.Itoa(field) == index {
			_, cleanup := typeCleanup(structure.Field(field).Type())
			return cleanup
		}
	}
	return false
}

// Claim names what a summary can say about one parameter. The masks it
// selects are the vocabulary every proof shares, so a proof asks for the claim
// it needs rather than knowing which fields spell it. Releasing is a union
// because the settling action depends on the resource: a file is closed, a
// ticker stopped, a transaction committed or rolled back.
type Claim uint8

const (
	ClaimReturnsOwner Claim = iota
	ClaimReturnsView
	ClaimRetains
	ClaimStores
	ClaimReleases
	ClaimSynchronouslyInvokes
	ClaimReleasesInLoop
)

// Claim returns the parameters this summary makes the claim about.
func (fact *Fact) Claim(claim Claim) ParameterMask {
	switch claim {
	case ClaimReturnsOwner:
		return fact.ReturnedOwner
	case ClaimReturnsView:
		return fact.ReturnedView
	case ClaimRetains:
		return fact.Retained
	case ClaimStores:
		return fact.Stored
	case ClaimReleases:
		return fact.Closed | fact.Finalized | fact.Released | fact.Shutdown | fact.Stopped |
			fact.Committed | fact.RolledBack | fact.DischargedParameters()
	case ClaimSynchronouslyInvokes:
		return fact.SynchronouslyInvoked
	case ClaimReleasesInLoop:
		return fact.LoopReleased
	}
	return 0
}

// ParameterMask is a set of SSA parameter positions in a lifecycle summary.
type ParameterMask uint64

// parameterMaskFor returns the mask containing index, or an empty mask when
// index cannot be represented by a lifecycle summary.
func parameterMaskFor(index int) ParameterMask {
	if index < 0 || index >= 64 {
		return 0
	}
	return ParameterMask(1) << index
}

// contains reports whether mask contains index.
func (mask ParameterMask) contains(index int) bool {
	return mask&parameterMaskFor(index) != 0
}

// Summaries is the pass result: the summary of every exported source function
// in the package plus the imported summary of every static callee.
type Summaries map[*ssa.Function]Fact

// DescribeFact renders the summary for the fact dump: one line per parameter
// that some mask covers, named from the function's signature. Mask positions
// follow SSA parameters, so a method's receiver is position zero.
func (fact *Fact) DescribeFact(object types.Object) []string {
	function, ok := object.(*types.Func)
	if !ok {
		return nil
	}
	var names []string
	signature := function.Signature()
	if signature.Recv() != nil {
		names = append(names, signature.Recv().Name())
	}
	for parameter := range signature.Params().Variables() {
		names = append(names, parameter.Name())
	}
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
	if fact.OwnedResults != 0 {
		lines = append(lines, "OwnedResults: "+fact.resultNames(fact.OwnedResults, signature))
	}
	for _, discharge := range fact.Discharges {
		if discharge.Path != "" && discharge.Parameter < len(names) {
			lines = append(lines, fmt.Sprintf("%d %s: %s at %s", discharge.Parameter, names[discharge.Parameter], discharge.Method, discharge.Path))
		}
	}
	lines = append(lines, fact.conditionalDescriptions()...)
	lines = append(lines, fact.returnedCleanupDescriptions()...)
	if len(lines) == 0 {
		lines = []string{"no parameter is proven on every return"}
	}
	return append(lines, fact.heapDescriptions()...)
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
func (fact *Fact) resultNames(mask ParameterMask, signature *types.Signature) string {
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
func (fact *Fact) fieldNames(mask ParameterMask, signature *types.Signature) []string {
	if mask == 0 {
		return nil
	}
	var structure *types.Struct
	if signature.Recv() != nil {
		structure = structBehind(signature.Recv().Type())
	} else {
		for result := range signature.Results().Variables() {
			if structure = structBehind(result.Type()); structure != nil {
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

func structBehind(value types.Type) *types.Struct {
	pointer, ok := value.Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	structure, _ := pointer.Elem().Underlying().(*types.Struct)
	return structure
}

func (fact *Fact) parameterMasks(index int) []string {
	var names []string
	for _, mask := range lifecycleMasks {
		if mask.field(fact).contains(index) {
			names = append(names, mask.name)
		}
	}
	return names
}

func (*Fact) AFact() {}

// GobEncode encodes the fact through factcodec.
func (fact *Fact) GobEncode() ([]byte, error) { return factcodec.Encode(fact) }

// GobDecode decodes the fact through factcodec.
func (fact *Fact) GobDecode(data []byte) error { return factcodec.Decode(data, fact) }

// SummarizedPackage marks a package whose exported functions with bodies
// were all summarized. A function of such a package with no summary of its
// own was proven to do nothing with its parameters; only a function of a
// package without the marker, or one listed as bodiless, is unknown. It
// exists so an empty summary need not be serialized for every function of
// every dependency, which the analysis framework would otherwise decode
// once per dependent package.
type SummarizedPackage struct {
	Bodiless []string
}

// AFact marks SummarizedPackage as an analysis fact.
func (*SummarizedPackage) AFact() {}

// GobEncode encodes the fact through factcodec.
func (fact *SummarizedPackage) GobEncode() ([]byte, error) { return factcodec.Encode(fact) }

// GobDecode decodes the fact through factcodec.
func (fact *SummarizedPackage) GobDecode(data []byte) error { return factcodec.Decode(data, fact) }

// empty reports whether the summary claims nothing.
func (fact *Fact) empty() bool {
	masks := fact.Invoked | fact.SynchronouslyInvoked | fact.Closed | fact.Finalized | fact.Released | fact.Shutdown |
		fact.Stopped | fact.Waited | fact.Committed | fact.RolledBack | fact.ReturnedOwner | fact.ReturnedView |
		fact.Retained | fact.Stored | fact.LoopReleased | fact.OwnedFields | fact.ReleasedFields | fact.OwnedResults |
		fact.ReceiverStore
	return masks == 0 && len(fact.Kept) == 0 && len(fact.Discharges) == 0 &&
		(fact.Conditional == nil || len(fact.Conditional.Effects) == 0) &&
		(fact.ReturnedCleanup == nil || len(fact.ReturnedCleanup.Effects) == 0) &&
		fact.heapEmpty()
}

// heapEmpty reports whether the heap projection claims nothing a caller
// must react to: no edges, no truncation, and no effect beyond handing an
// object to a call. Reads are not exported, and a call escape alone says
// nothing an importer acts on: the summary's truncation already carries
// what an unresolved callee may have done, and a resolved one carries its
// own summary.
func (fact *Fact) heapEmpty() bool {
	if fact.Heap == nil {
		return true
	}
	if len(fact.Heap.Edges) != 0 || len(fact.Heap.Truncated) != 0 || len(fact.Heap.Requires) != 0 {
		return false
	}
	for _, effect := range fact.Heap.Effects {
		if effect.Release != "" || effect.Escape&^heapmodel.HeapEscapedCall != 0 {
			return false
		}
	}
	return true
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

// importFact imports the summary attached to a static callee.
func importFact(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	return factForFunction(pass, ssaflow.ResolvedCallee(ssaflow.InstructionCall(instruction)))
}

// factForFunction returns the summary recorded for a function. It is the one
// place that reads an imported summary, so every proof asks the question the
// same way: a generic instantiation is answered by its origin, which is the
// object the summary was recorded against, and a function with no object,
// such as a literal, has no summary to find.
func factForFunction(pass *analysis.Pass, function *ssa.Function) (Fact, bool) {
	resolved := ssaflow.ResolvedFunction(function)
	if pass == nil || resolved == nil {
		return Fact{}, false
	}
	object := resolved.Object()
	if object == nil {
		return Fact{}, false
	}
	var fact Fact
	if pass.ImportObjectFact(object, &fact) {
		// An older heap fact may describe a returned field address as the
		// field's contents. None of its derived claims are safe to import
		// under the newer edge semantics.
		if fact.Heap != nil && fact.Heap.Version != heapmodel.SummaryVersion {
			return Fact{}, false
		}
		return fact, true
	}
	// No summary of its own: proven to do nothing if its package was
	// summarized and the function was in scope for summarizing, which is
	// the same condition the pass applies before summarizing.
	var marker SummarizedPackage
	if object.Pkg() == nil || !object.Exported() || pass.ImportPackageFact == nil || !pass.ImportPackageFact(object.Pkg(), &marker) {
		return Fact{}, false
	}
	if signature, ok := object.Type().(*types.Signature); !ok || signature.Params().Len()+receiverCount(signature) > 64 {
		return Fact{}, false
	}
	if slices.Contains(marker.Bodiless, object.Name()) {
		return Fact{}, false
	}
	return Fact{}, true
}

func receiverCount(signature *types.Signature) int {
	if signature.Recv() != nil {
		return 1
	}
	return 0
}

// factOwnsArgument reports whether mask covers the argument which contains
// target at this callsite.
// factOwnsExactArgument is factOwnsArgument without containment: only the
// target itself passed as the masked argument counts, so a literal that
// captured the target is not mistaken for it.
func factOwnsExactArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask) bool {
	return factArgumentMatches(instruction, target, mask, func(value, target ssa.Value) bool {
		return heapmodel.NewStorage(nil).Same(value, target).Proven()
	})
}

func factArgumentMatches(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, matches func(ssa.Value, ssa.Value) bool) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if mask.contains(index) && matches(argument, target) {
			return true
		}
	}
	return false
}

func factOwnsArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer ssaflow.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		if !mask.contains(index) {
			continue
		}
		if heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget).Observed(observer)).Same(argument, target).Proven() {
			return true
		}
		// Containment must not turn an ambiguous phi or a storage-history
		// match into a guarantee about this target.
		if !heapmodel.MayAlias(argument, target) && lifecycle.MayContainValue(argument, target) {
			return true
		}
	}
	return false
}

func factOwnsProjectedArgument(instruction ssa.Instruction, target ssa.Value, mask ParameterMask, observer ssaflow.Observer) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	for index, argument := range common.Args {
		storage := heapmodel.NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget).Observed(observer))
		if mask.contains(index) && storage.Projection(argument, target, instruction).Proven() {
			return true
		}
	}
	return false
}

// lifecycleMask names one mask and, for method masks, the lifecycle method
// whose call on every return sets it. This table is the one catalog shared by
// summarization, imported-mask selection, and the fact dump.
type lifecycleMask struct {
	name   string
	method string
	field  func(*Fact) *ParameterMask
}

var lifecycleMasks = []lifecycleMask{
	{name: "Invoked", field: func(fact *Fact) *ParameterMask { return &fact.Invoked }},
	{name: "SynchronouslyInvoked", field: func(fact *Fact) *ParameterMask { return &fact.SynchronouslyInvoked }},
	{name: "Closed", method: "Close", field: func(fact *Fact) *ParameterMask { return &fact.Closed }},
	{name: "Finalized", method: "Finalize", field: func(fact *Fact) *ParameterMask { return &fact.Finalized }},
	{name: "Released", method: "Release", field: func(fact *Fact) *ParameterMask { return &fact.Released }},
	{name: "Shutdown", method: "Shutdown", field: func(fact *Fact) *ParameterMask { return &fact.Shutdown }},
	{name: "Stopped", method: "Stop", field: func(fact *Fact) *ParameterMask { return &fact.Stopped }},
	{name: "Waited", method: "Wait", field: func(fact *Fact) *ParameterMask { return &fact.Waited }},
	{name: "Committed", method: "Commit", field: func(fact *Fact) *ParameterMask { return &fact.Committed }},
	{name: "RolledBack", method: "Rollback", field: func(fact *Fact) *ParameterMask { return &fact.RolledBack }},
	{name: "ReturnedOwner", field: func(fact *Fact) *ParameterMask { return &fact.ReturnedOwner }},
	{name: "ReturnedView", field: func(fact *Fact) *ParameterMask { return &fact.ReturnedView }},
	{name: "ReceiverStore", field: func(fact *Fact) *ParameterMask { return &fact.ReceiverStore }},
	{name: "Retained", field: func(fact *Fact) *ParameterMask { return &fact.Retained }},
	{name: "Stored", field: func(fact *Fact) *ParameterMask { return &fact.Stored }},
	{name: "LoopReleased", field: func(fact *Fact) *ParameterMask { return &fact.LoopReleased }},
}

// fieldMasks are indexed by struct field of the result or receiver type.
var fieldMasks = []lifecycleMask{
	{name: "OwnedFields", field: func(fact *Fact) *ParameterMask { return &fact.OwnedFields }},
	{name: "ReleasedFields", field: func(fact *Fact) *ParameterMask { return &fact.ReleasedFields }},
}

// MethodMask selects the parameter mask for a lifecycle method.
func (fact *Fact) MethodMask(method string) ParameterMask {
	for _, mask := range lifecycleMasks {
		if mask.method == method && mask.method != "" {
			return *mask.field(fact)
		}
	}
	return 0
}
