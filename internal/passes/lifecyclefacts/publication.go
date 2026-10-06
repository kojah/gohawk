package lifecyclefacts

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Publication is separate from the semantic summary. An opaque envelope keeps
// gob from recursively describing the heap schema in every inherited-fact stream.
// Domain validation and the distinction between missing and empty stay here.
// The unexported alias also hides the envelope's own type descriptor from gob.
type publication[T any] = factcodec.Envelope[T]

type publishedFact struct {
	publication[Fact]
}

type publishedCleanup struct {
	publication[CleanupFact]
}

type publishedPackage struct {
	publication[SummarizedPackage]
}

func publish(fact Fact) *publishedFact { return &publishedFact{factcodec.Wrap(fact)} }

func (fact *publishedFact) DescribeFact(object types.Object) []string {
	value := fact.Value()
	return value.DescribeFact(object)
}

// DescribeHeap renders the published summary's heap projection for the dump.
func (fact *publishedFact) DescribeHeap(object types.Object) []string {
	value := fact.Value()
	return value.DescribeHeap(object)
}

func (fact *publishedCleanup) DescribeFact(object types.Object) []string {
	value := fact.Value()
	return value.DescribeFact(object)
}

// Imported lookup validates published lifecycle facts and attaches declaration
// signatures. A valid package marker distinguishes a known empty summary from
// an unavailable one; bodiless and incompatible declarations remain unknown.

// importFact imports the summary attached to a static callee.
func importFact(pass *analysis.Pass, instruction ssa.Instruction) (Fact, bool) {
	return factForFunction(pass, ssacall.ResolvedCallee(ssaflow.InstructionCall(instruction)))
}

// factForFunction returns the summary recorded for a function. It is the one
// place that reads an imported summary, so every proof asks the question the
// same way: a generic instantiation is answered by its origin, which is the
// object the summary was recorded against, and a function with no object,
// such as a literal, has no summary to find.
func factForFunction(pass *analysis.Pass, function *ssa.Function) (Fact, bool) {
	resolved := ssacall.ResolvedFunction(function)
	if pass == nil || resolved == nil {
		return Fact{}, false
	}
	object := resolved.Object()
	if object == nil {
		return Fact{}, false
	}
	var published publishedFact
	if pass.ImportObjectFact(object, &published) {
		fact := published.Value()
		// An older heap fact may describe a returned field address as the
		// field's contents. None of its derived claims are safe to import
		// under the newer edge semantics.
		if fact.Heap != nil && fact.Heap.Version != heapmodel.SummaryVersion {
			return Fact{}, false
		}
		fact.signature = resolved.Signature
		return fact, true
	}
	// No summary of its own: proven to do nothing if its package was
	// summarized and the function was in scope for summarizing, which is
	// the same condition the pass applies before summarizing.
	var marker publishedPackage
	if object.Pkg() == nil || !object.Exported() || pass.ImportPackageFact == nil || !pass.ImportPackageFact(object.Pkg(), &marker) {
		return Fact{}, false
	}
	if signature, ok := object.Type().(*types.Signature); !ok || signature.Params().Len()+receiverCount(signature) > 64 {
		return Fact{}, false
	}
	if slices.Contains(marker.Value().Bodiless, object.Name()) {
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
