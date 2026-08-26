package general

import "golang.org/x/tools/go/analysis"

// Analyzers returns framework-neutral Go policy analyzers.
func Analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		apiShapeAnalyzer(),
		contextPolicyAnalyzer(),
		globalStateAnalyzer(),
		wirePolicyAnalyzer(),
		testPolicyAnalyzer(),
		blockingTestAnalyzer(),
		goroutineOwnershipAnalyzer(),
		errorOwnershipAnalyzer(),
		channelPolicyAnalyzer(),
		processOwnershipAnalyzer(),
		closedDomainAnalyzer(),
		taintPolicyAnalyzer(),
		lockOrderAnalyzer(),
		resourceLifetimeAnalyzer(),
		determinismAnalyzer(),
		cancellationOwnershipAnalyzer(),
	}
}
