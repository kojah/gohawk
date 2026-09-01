// Package resourcelifetime implements the resourcelifetime gohawk analyzer.
package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/analysispasses/lifecyclefacts"
	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/flagvalue"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func Analyzer() *analysis.Analyzer {
	config := resourceLifetimeConfig{contracts: "os,http,sql,time,compress", requireReaderClose: true}
	analyzer := &analysis.Analyzer{
		Name:     "resourcelifetime",
		Doc:      "checks owned files, SQL handles, HTTP responses, timers, and compressors are released on every path",
		Requires: []*analysis.Analyzer{buildssa.Analyzer, lifecyclefacts.Analyzer},
	}
	analyzer.Flags.Var(
		flagvalue.NewCommaSeparatedChoice(&config.contracts, "os", "http", "sql", "time", "compress"),
		"contracts",
		"comma-separated resource contract families: os,http,sql,time,compress",
	)
	analyzer.Flags.BoolVar(&config.requireReaderClose, "require-reader-close", true, "require gzip and zlib readers to be closed")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runResourceLifetime(pass, config)
	}
	return analyzer
}

type resourceLifetimeConfig struct {
	contracts          string
	requireReaderClose bool
}

type resourceLifetimeSettings struct {
	contracts          map[string]bool
	requireReaderClose bool
}

func runResourceLifetime(pass *analysis.Pass, config resourceLifetimeConfig) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	settings := resourceLifetimeSettings{contracts: flagvalue.CommaSeparatedSet(config.contracts), requireReaderClose: config.requireReaderClose}
	completeTimers := completeTimerLifecyclePositions(pass)
	// Acquisition contracts identify both the owned result and its required
	// cleanup action. Reporting is deferred until path analysis proves that the
	// action or a recognized ownership transfer is absent on a normal return.
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				contract, ok := resourceContractFor(call.Common(), settings)
				if !ok {
					continue
				}
				resource := ssautil.CallResult(call, contract.result)
				if resource == nil {
					continue
				}
				leaks := resourceLeaks(pass, call, resource, contract)
				completeTimer := completeTimers[call.Pos()]
				emitResourceDecision(pass, function, call, resource, contract, leaks, completeTimer)
				if leaks && !completeTimer {
					check.Reportf(
						pass,
						check.ResourceRelease,
						call.Pos(),
						"owned resource from %s.%s is not released on every return path",
						analysisutil.ShortPackageName(contract.packagePath),
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
	leaks, completeTimer bool,
) {
	checkID := string(check.ResourceRelease)
	if !analysisTrace.Enabled("resourcelifetime", checkID) {
		return
	}
	outcome, reason := analysisTrace.OutcomeAccepted, "release-proven"
	if completeTimer {
		reason = "complete-timer-lifecycle"
	} else if leaks {
		outcome, reason = analysisTrace.OutcomeRejected, "unowned-return"
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
			Reason:   reason,
			Outcome:  outcome,
			Pos:      call.Pos(),
			Function: function.String(),
			Details:  details,
		},
	)
}
