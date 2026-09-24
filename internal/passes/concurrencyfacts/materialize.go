package concurrencyfacts

import "golang.org/x/tools/go/ssa"

// Declaration templates can carry a receiver-relative field through methods
// that do not select it themselves. Query boundaries require a real address in
// the caller. Never invent an SSA instruction or equate an owner with its lock.
func (engine *Engine) materialize(summary Summary, function *ssa.Function) Summary {
	if len(summary.Paths) != 0 {
		paths := make([]Summary, 0, len(summary.Paths))
		for _, path := range summary.Paths {
			bound := engine.materialize(path, function)
			if !composableLinear(bound) && !bound.AlternativesComplete {
				return bound
			}
			paths = append(paths, bound)
		}
		summary.Paths = paths
		return summary
	}
	if !composableLinear(summary) && !summary.AlternativesComplete {
		return summary
	}
	summary = cloneEffects(summary)
	// Clone before rebinding every path; the cache and sibling graph variants
	// may still refer to the original immutable declaration templates.
	sequences := [][]Operation{summary.Operations}
	for index := range summary.Workers {
		sequences = append(sequences, summary.Workers[index].Operations)
		sequences = append(sequences, summary.Workers[index].Alternatives...)
	}
	for index := range summary.Choices {
		for arm := range summary.Choices[index].Arms {
			choice := &summary.Choices[index].Arms[arm]
			sequences = append(sequences, choice.Sequence)
		}
	}
	for _, sequence := range sequences {
		for index := range sequence {
			reference := sequence[index].Resource
			if reference.Projection.Depth == 0 {
				continue
			}
			value, found := engine.fieldAddress(function, reference.Projection)
			if !found {
				return Summary{Reason: "protocol-field-binding-unknown"}
			}
			sequence[index].Resource = Reference{Value: value}
		}
	}
	return summary
}
