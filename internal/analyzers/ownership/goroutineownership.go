package ownership

import (
	"github.com/kojah/gohawk/analysisutil/ssa"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func goroutineOwnershipAnalyzer() *analysis.Analyzer {
	config := goroutineOwnershipConfig{mode: goroutineModeContext}
	analyzer := &analysis.Analyzer{
		Name:     "goroutineownership",
		Doc:      "checks that explicit goroutines have a recognizable join handle or lifecycle owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
	}
	analyzer.Flags.Var(newChoiceValue(&config.mode, goroutineModeContext, goroutineModeLifecycle, goroutineModeJoin), "mode", "ownership policy: context, lifecycle, or join")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runGoroutineOwnership(pass, config)
	}
	return analyzer
}

type goroutineOwnershipConfig struct {
	mode string
}

const (
	goroutineModeContext   = "context"
	goroutineModeLifecycle = "lifecycle"
	goroutineModeJoin      = "join"
)

func runGoroutineOwnership(pass *analysis.Pass, config goroutineOwnershipConfig) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportAbandonedProducerSends(pass, function)
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				spawn, ok := instruction.(*ssa.Go)
				if !ok {
					continue
				}
				signals, groups := goroutineJoinValues(spawn)
				owners := goroutineLifecycleValues(spawn)
				// A received channel argument is a stop signal, not a completion
				// handle that the caller must join. Kubernetes informers commonly
				// express context ownership as Run(ctx.Done()):
				// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
				if config.mode == goroutineModeContext && (goroutineHasContextLifecycle(spawn) || goroutineHasStopLifecycle(spawn)) {
					continue
				}
				if (config.mode != goroutineModeJoin && (goroutineTransferredToCaller(function, spawn) || externallyOwnedLifecycle(owners) || externallyOwnedJoin(signals, groups))) || ownershipRegisteredBefore(spawn, signals, groups) {
					continue
				}
				// If spawning and receiving range over the same unchanged bound, any
				// execution that launches a worker also executes at least one join
				// iteration. This covers the common "N workers, then N receives"
				// pattern without assuming unrelated loops have matching counts.
				if matchingCountedJoin(function, spawn, signals) {
					continue
				}
				if nestedCallbackReceivesAny(function, signals) {
					continue
				}
				if ssautil.UnownedReturn(spawn, func(candidate ssa.Instruction) bool {
					if joinsGoroutine(candidate, signals, groups) || waitsForLifecycleOwner(candidate, owners) {
						return true
					}
					if config.mode == goroutineModeJoin {
						return transfersGoroutineOwnership(candidate, signals, groups, nil)
					}
					return ownsGoroutineLifecycle(candidate, owners) || transfersGoroutineOwnership(candidate, signals, groups, owners)
				}, func(returned *ssa.Return) bool {
					return returnedAliasesAny(returned, signals) || returnedAliasesAny(returned, groups) || config.mode != goroutineModeJoin && returnedAliasesAny(returned, owners)
				}) {
					check := checkGoroutineJoin
					// Without a completion signal or wait group, static analysis cannot
					// reliably distinguish a leak from intentional component work. Keep
					// that heuristic opt-in; reserve the default check for code that
					// exposes a recognizable join mechanism and fails to honor it.
					if config.mode != goroutineModeJoin && len(signals) == 0 && len(groups) == 0 {
						check = checkGoroutineDetached
					}
					reportf(pass, check, spawn.Pos(), "goroutine is not joined on every return path")
				}
			}
		}
	}
	return nil, nil
}
