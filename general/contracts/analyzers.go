// Package contracts provides API and data contract analyzers.
package contracts

import (
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
)

const (
	checkAPIParameterCount    = analyzerbase.CheckAPIParameterCount
	checkAPIMixedReceivers    = analyzerbase.CheckAPIMixedReceivers
	checkAPIAdjacentSameType  = analyzerbase.CheckAPIAdjacentSameType
	checkAPIAdjacentOptional  = analyzerbase.CheckAPIAdjacentOptional
	checkContextFirst         = analyzerbase.CheckContextFirst
	checkContextStorage       = analyzerbase.CheckContextStorage
	checkContextTestOwnership = analyzerbase.CheckContextTestOwnership
	checkContextNilArgument   = analyzerbase.CheckContextNilArgument
	checkClosedStringDomain   = analyzerbase.CheckClosedStringDomain
	checkWireKeyedLiteral     = analyzerbase.CheckWireKeyedLiteral
	checkWireSerializationTag = analyzerbase.CheckWireSerializationTag
)

var reportf = analyzerbase.Reportf
var report = analyzerbase.Report

// Analyzers returns the API and data contract analyzers in stable order.
func Analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		apiShapeAnalyzer(),
		contextPolicyAnalyzer(),
		closedDomainAnalyzer(),
		wirePolicyAnalyzer(),
	}
}
