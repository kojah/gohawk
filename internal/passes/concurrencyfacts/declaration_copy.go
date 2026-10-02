package concurrencyfacts

import "github.com/kojah/gohawk/internal/ssaflow"

// Declaration copying owns the boundary between immutable imported cache data
// and caller-owned metadata. Linear and alternative bodies use the same copy
// path. Each visited element spends the lookup allowance before allocation or
// copying; a cutoff discards the entire declaration without changing the cache.
func cloneDeclaration(fact Fact, budget *ssaflow.SearchBudget) (Fact, bool) {
	copied, ok := cloneDeclarationBody(fact, budget)
	if !ok {
		return Fact{}, false
	}
	copied.Alternatives, ok = cloneDeclarationSlice(fact.Alternatives, budget)
	if !ok {
		return Fact{}, false
	}
	for index, alternative := range copied.Alternatives {
		body, ok := cloneDeclarationBody(Fact{
			Effects: alternative.Effects, Workers: alternative.Workers, CancellationInputs: alternative.CancellationInputs,
		}, budget)
		if !ok {
			return Fact{}, false
		}
		alternative.Effects, alternative.Workers, alternative.CancellationInputs = body.Effects, body.Workers, body.CancellationInputs
		alternative.Conditions, ok = cloneDeclarationSlice(alternative.Conditions, budget)
		if !ok {
			return Fact{}, false
		}
		alternative.Returned, ok = cloneDeclarationSlice(alternative.Returned, budget)
		if !ok {
			return Fact{}, false
		}
		copied.Alternatives[index] = alternative
	}
	return copied, true
}

func cloneDeclarationBody(fact Fact, budget *ssaflow.SearchBudget) (Fact, bool) {
	var ok bool
	fact.Effects, ok = cloneDeclarationEffects(fact.Effects, budget)
	if !ok {
		return Fact{}, false
	}
	fact.CancellationInputs, ok = cloneDeclarationSlice(fact.CancellationInputs, budget)
	if !ok {
		return Fact{}, false
	}
	fact.Workers, ok = cloneDeclarationSlice(fact.Workers, budget)
	if !ok {
		return Fact{}, false
	}
	for index := range fact.Workers {
		fact.Workers[index].Effects, ok = cloneDeclarationEffects(fact.Workers[index].Effects, budget)
		if !ok {
			return Fact{}, false
		}
	}
	return fact, true
}

func cloneDeclarationEffects(effects []Effect, budget *ssaflow.SearchBudget) ([]Effect, bool) {
	copied, ok := cloneDeclarationSlice(effects, budget)
	if !ok {
		return nil, false
	}
	for index := range copied {
		copied[index].Fields, ok = cloneDeclarationSlice(copied[index].Fields, budget)
		if !ok {
			return nil, false
		}
	}
	return copied, true
}

// Strings and scalar metadata are immutable values. Only slice elements need
// ownership and a charge here; no up-front allocation uses an unvisited length.
func cloneDeclarationSlice[T any](values []T, budget *ssaflow.SearchBudget) ([]T, bool) {
	if budget.Exhausted() {
		return nil, false
	}
	if values == nil {
		return nil, true
	}
	copied := make([]T, 0)
	for _, value := range values {
		if !budget.Spend() {
			return nil, false
		}
		copied = append(copied, value)
	}
	return copied, true
}
