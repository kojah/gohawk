// Package contracts provides API and data contract analyzers.
package contracts

import (
	"github.com/kojah/gohawk/internal/analyzerbase"
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

// Specs returns the API and data contract analyzer declarations in stable order.
func Specs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{
			Analyzer: apiShapeAnalyzer(), OptIn: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkAPIParameterCount, Doc: "Reports exported APIs with more than the configured maximum number of parameters."},
				{ID: checkAPIMixedReceivers, Doc: "Reports types that mix pointer and value receiver methods."},
				{ID: checkAPIAdjacentSameType, Doc: "Reports long adjacent runs of parameters that share one type."},
				{ID: checkAPIAdjacentOptional, Doc: "Reports adjacent optional scalar parameters that are easy to swap."},
			},
		},
		{
			Analyzer: contextPolicyAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkContextFirst, Doc: "Reports functions whose first context.Context parameter follows another parameter."},
				{ID: checkContextStorage, Doc: "Reports context.Context values stored in structs."},
				{ID: checkContextTestOwnership, Doc: "Reports tests that use context.Background instead of the testing handle's context.", OptIn: true},
				{ID: checkContextNilArgument, Doc: "Reports definitely nil context.Context arguments."},
			},
		},
		{
			Analyzer: closedDomainAnalyzer(), OptIn: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkClosedStringDomain, Doc: "Reports exported string fields used as small closed sets of values."},
			},
		},
		{
			Analyzer: wirePolicyAnalyzer(), OptIn: true, SuggestedFix: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkWireKeyedLiteral, Doc: "Reports positional composite literals for persisted or wire structs."},
				{ID: checkWireSerializationTag, Doc: "Reports exported wire fields without explicit JSON or TOML tags."},
			},
		},
	}
}
