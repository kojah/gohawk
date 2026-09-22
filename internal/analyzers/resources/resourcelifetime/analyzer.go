// Package resourcelifetime implements the resourcelifetime gohawk analyzer.
package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/flagvalue"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func Analyzer() *analysis.Analyzer {
	config := resourceLifetimeConfig{contracts: "os,http,sql,compress,owned"}
	analyzer := &analysis.Analyzer{
		Name:     "resourcelifetime",
		Doc:      "checks owned files, SQL handles, HTTP responses, and compressors are released on every path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, lifecyclefacts.Analyzer},
	}
	analyzer.Flags.Var(
		flagvalue.NewCommaSeparatedChoice(&config.contracts, "os", "http", "sql", "compress", "owned"),
		"contracts",
		"comma-separated resource contract families: os,http,sql,compress,owned",
	)
	analyzer.Flags.BoolVar(
		&config.requireMemoryWriterClose,
		"require-memory-writer-close",
		false,
		"report gzip and zlib writers over an in-memory buffer that are not closed on every path",
	)
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runResourceLifetime(pass, config)
	}
	return analyzer
}

type resourceLifetimeConfig struct {
	contracts                string
	requireMemoryWriterClose bool
}

type resourceLifetimeSettings struct {
	contracts                map[string]bool
	catalog                  []resourceContract
	requireMemoryWriterClose bool
}

func runResourceLifetime(pass *analysis.Pass, config resourceLifetimeConfig) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	settings := resourceLifetimeSettings{
		contracts:                flagvalue.CommaSeparatedSet(config.contracts),
		catalog:                  resourceContracts(),
		requireMemoryWriterClose: config.requireMemoryWriterClose,
	}
	// Acquisition contracts identify both the owned result and its required
	// cleanup action. Reporting is deferred until path analysis proves that the
	// action or a recognized ownership transfer is absent on a normal return.
	for _, function := range functions {
		evidence := lifecyclefacts.NewLifecycleEvidence(pass, "resourcelifetime", string(check.ResourceRelease))
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				contract, ok := resourceContractFor(call.Common(), settings)
				if !ok {
					contract, ok = ownedResultContract(evidence, call, settings)
				}
				if !ok {
					continue
				}
				resource := ssaflow.CallResult(call, contract.result)
				if resource == nil {
					continue
				}
				// Exemption from leak cleanup does not make a closed in-memory
				// writer usable again. Invalidation has its own API contract.
				reportUsesAfterRelease(pass, function, call, resource, contract)
				if memoryWriterExempt(call, contract, settings) {
					continue
				}
				evidence.ForCandidate(call.Pos())
				result := evaluateResourceFlow(pass, evidence, call, resource, contract)
				emitResourceDecision(pass, function, call, resource, contract, result)
				if result.report {
					check.Reportf(
						pass,
						check.ResourceRelease,
						call.Pos(),
						"owned resource from %s.%s is not released on every return path",
						syntax.ShortPackageName(contract.packagePath),
						contract.name,
					)
				}
			}
		}
	}
	return nil, nil
}

func emitResourceDecision(
	pass *analysis.Pass,
	function *ssa.Function,
	call *ssa.Call,
	resource ssa.Value,
	contract resourceContract,
	result resourceLifetimePolicyResult,
) {
	probe := analysisTrace.For(pass, "resourcelifetime", string(check.ResourceRelease), call.Pos())
	if !probe.Enabled() {
		return
	}
	outcome := analysisTrace.OutcomeAccepted
	if result.report {
		outcome = analysisTrace.OutcomeRejected
	}
	details := map[string]string{"acquisition": contract.packagePath + "." + contract.name}
	if resource != nil && resource.Type() != nil {
		details["resource_type"] = resource.Type().String()
	}
	probe.Decision(analysisTrace.Step{
		Reason:   string(result.reason),
		Outcome:  outcome,
		Pos:      call.Pos(),
		Function: function.String(),
		Details:  details,
	})
}
