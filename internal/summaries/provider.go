// Package summaries brokers typed function-summary components selected before
// package analysis. Domain passes retain inference and fact publication;
// this package neither schedules analysis nor merges their proof semantics.
package summaries

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Requirements selects independently computed components at analyzer setup.
type Requirements struct {
	Results     bool
	Lifecycle   bool
	Concurrency bool
}

// Selection is an immutable declaration shared by Requires and Provider.
type Selection struct{ requirements Requirements }

// Select fixes the knowledge an analyzer may request from its provider.
func Select(requirements Requirements) Selection { return Selection{requirements: requirements} }

// Requires returns ordinary analysis prerequisites, including the shared SSA
// input. Unselected domain passes are not scheduled by this selection.
func (selection Selection) Requires() []*analysis.Analyzer {
	result := []*analysis.Analyzer{buildssa.Analyzer}
	if selection.requirements.Results {
		result = append(result, resultfacts.Analyzer)
	}
	if selection.requirements.Lifecycle {
		result = append(result, lifecyclefacts.Analyzer)
	}
	if selection.requirements.Concurrency {
		result = append(result, concurrencyfacts.Analyzer)
	}
	return result
}

// Availability describes access to one component, not function completeness.
type Availability uint8

const (
	NotRequested Availability = iota
	Unavailable
	Available
)

// Provider exposes only the selected prerequisite results. It never imports
// object facts from a sibling pass or retroactively requests dependency work.
type Provider struct {
	selection   Selection
	pass        *analysis.Pass
	results     *resultfacts.Engine
	lifecycle   lifecyclefacts.Summaries
	concurrency *concurrencyfacts.Engine
}

// Provider binds this setup-time selection to the completed prerequisite passes.
func (selection Selection) Provider(pass *analysis.Pass) *Provider {
	provider := &Provider{selection: selection, pass: pass}
	if pass == nil {
		return provider
	}
	if selection.requirements.Results {
		provider.results, _ = pass.ResultOf[resultfacts.Analyzer].(*resultfacts.Engine)
	}
	if selection.requirements.Lifecycle {
		provider.lifecycle, _ = pass.ResultOf[lifecyclefacts.Analyzer].(lifecyclefacts.Summaries)
	}
	if selection.requirements.Concurrency {
		provider.concurrency, _ = pass.ResultOf[concurrencyfacts.Analyzer].(*concurrencyfacts.Engine)
	}
	return provider
}

// Function is an uninstantiated view of one declaration's guarantees.
// Call-site effects and ownership evidence require their separate adapters.
type Function struct {
	provider *Provider
	function *ssa.Function
}

// ForFunction creates a view without computing unrequested knowledge.
func (provider *Provider) ForFunction(function *ssa.Function) Function {
	return Function{provider: provider, function: function}
}

// Results obtains unconditional per-result guarantees. Available summaries may
// still return Unknown for any or every result position.
func (function Function) Results(budget *ssaflow.SearchBudget) (resultfacts.Summary, Availability) {
	provider := function.provider
	if !provider.selection.requirements.Results {
		return resultfacts.Summary{}, NotRequested
	}
	if provider.results == nil {
		return resultfacts.Summary{}, Unavailable
	}
	result := provider.results.Function(function.function, budget)
	if !result.Available {
		return result, Unavailable
	}
	return result, Available
}

// Lifecycle obtains published parameter and result relationships. A missing
// relationship never implies the absence of an effect. Private-body questions
// remain available through LifecycleEvidence rather than fabricated facts.
func (function Function) Lifecycle() (lifecyclefacts.Fact, Availability) {
	provider := function.provider
	if !provider.selection.requirements.Lifecycle {
		return lifecyclefacts.Fact{}, NotRequested
	}
	fact, ok := provider.lifecycle[function.function]
	if !ok {
		return lifecyclefacts.Fact{}, Unavailable
	}
	return fact, Available
}

// Concurrency obtains complete formal-parameter effects, local or imported.
// Its completeness belongs only to this domain, never to the whole function.
func (function Function) Concurrency(budget *ssaflow.SearchBudget) (concurrencyfacts.Fact, Availability) {
	provider := function.provider
	if !provider.selection.requirements.Concurrency {
		return concurrencyfacts.Fact{}, NotRequested
	}
	if provider.concurrency == nil {
		return concurrencyfacts.Fact{}, Unavailable
	}
	fact, ok := provider.concurrency.Declaration(function.function, budget)
	if !ok {
		return fact, Unavailable
	}
	return fact, Available
}

// LifecycleEvidence provides existing exact call-site binding and local proof
// machinery under the selected lifecycle component. It does not equate a
// formal parameter guarantee with an instantiated caller obligation.
func (provider *Provider) LifecycleEvidence(analyzer, check string) (*lifecyclefacts.LifecycleEvidence, Availability) {
	if !provider.selection.requirements.Lifecycle {
		return nil, NotRequested
	}
	if provider.lifecycle == nil {
		return lifecyclefacts.NewLifecycleEvidence(provider.pass, analyzer, check), Unavailable
	}
	return lifecyclefacts.NewLifecycleEvidence(provider.pass, analyzer, check), Available
}

// Concurrency provides the selected domain engine for root/worker queries that
// cannot be represented by a single formal-parameter declaration. Its own
// completeness rules and bounded call-site binding remain authoritative.
func (provider *Provider) Concurrency() (*concurrencyfacts.Engine, Availability) {
	if !provider.selection.requirements.Concurrency {
		return nil, NotRequested
	}
	if provider.concurrency == nil {
		return nil, Unavailable
	}
	return provider.concurrency, Available
}

// ConcurrencyAtCall binds ordered effects through the existing domain engine.
// Missing or incomplete effects retain the engine's domain-specific Reason.
func (provider *Provider) ConcurrencyAtCall(call ssa.CallInstruction, budget *ssaflow.SearchBudget) (concurrencyfacts.Summary, Availability) {
	if !provider.selection.requirements.Concurrency {
		return concurrencyfacts.Summary{Reason: concurrencyfacts.ReasonComponentNotRequested}, NotRequested
	}
	if provider.concurrency == nil {
		return concurrencyfacts.Summary{Reason: concurrencyfacts.ReasonComponentUnavailable}, Unavailable
	}
	if function := call.Common().StaticCallee(); function != nil && len(function.Blocks) == 0 {
		if fact, availability := provider.ForFunction(function).Concurrency(budget); availability == Available {
			return provider.concurrency.BindDeclaration(call, fact, budget), Available
		}
	}
	return provider.concurrency.AtCall(call, budget), Available
}

// CallReturnsView binds a selected lifecycle declaration through the domain's
// existing exact call-site evidence policy.
func (provider *Provider) CallReturnsView(call *ssa.Call, target ssa.Value) bool {
	fact, availability := provider.ForFunction(call.Common().StaticCallee()).Lifecycle()
	return availability == Available && fact.ReturnsView(call, target)
}
