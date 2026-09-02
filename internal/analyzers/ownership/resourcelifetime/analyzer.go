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
	config := resourceLifetimeConfig{contracts: "os,http,sql,time,compress,owned", requireReaderClose: true}
	analyzer := &analysis.Analyzer{
		Name:     "resourcelifetime",
		Doc:      "checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, lifecyclefacts.Analyzer},
	}
	analyzer.Flags.Var(
		flagvalue.NewCommaSeparatedChoice(&config.contracts, "os", "http", "sql", "time", "compress", "owned"),
		"contracts",
		"comma-separated resource contract families: os,http,sql,time,compress,owned",
	)
	analyzer.Flags.BoolVar(&config.requireReaderClose, "require-reader-close", true, "require gzip and zlib readers to be closed")
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
	requireReaderClose       bool
	requireMemoryWriterClose bool
}

type resourceLifetimeSettings struct {
	contracts                map[string]bool
	catalog                  []resourceContract
	requireReaderClose       bool
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
		requireReaderClose:       config.requireReaderClose,
		requireMemoryWriterClose: config.requireMemoryWriterClose,
	}
	completeTimers := completeTimerLifecyclePositions(pass)
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
				if resource == nil || memoryWriterExempt(call, contract, settings) {
					continue
				}
				result := evaluateResourceLifetime(pass, evidence, call, resource, contract, completeTimers[call.Pos()])
				emitResourceDecision(pass, function, call, resource, contract, result)
				reportUsesAfterRelease(pass, function, call, resource, contract)
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
	checkID := string(check.ResourceRelease)
	if !analysisTrace.Enabled("resourcelifetime", checkID) {
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
	analysisTrace.Emit(
		pass,
		analysisTrace.Event{
			Analyzer: "resourcelifetime",
			Check:    checkID,
			Phase:    "decision",
			Reason:   string(result.reason),
			Outcome:  outcome,
			Pos:      call.Pos(),
			Function: function.String(),
			Details:  details,
		},
	)
}
