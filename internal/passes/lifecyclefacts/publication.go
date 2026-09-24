package lifecyclefacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/factcodec"
)

// Publication is separate from the semantic summary. An opaque envelope keeps
// gob from recursively describing the heap schema in every inherited-fact stream.
// Domain validation and the distinction between missing and empty stay here.
type publishedFact struct {
	factcodec.Envelope[Fact]
}

type publishedCleanup struct {
	factcodec.Envelope[CleanupFact]
}

type publishedPackage struct {
	factcodec.Envelope[SummarizedPackage]
}

func publish(fact Fact) *publishedFact { return &publishedFact{factcodec.Wrap(fact)} }

func (fact *publishedFact) DescribeFact(object types.Object) []string {
	value := fact.Value()
	return value.DescribeFact(object)
}

func (fact *publishedCleanup) DescribeFact(object types.Object) []string {
	value := fact.Value()
	return value.DescribeFact(object)
}
