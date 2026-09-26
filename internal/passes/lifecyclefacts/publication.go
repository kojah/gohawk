package lifecyclefacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/factcodec"
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
