// Package cfg provides bounded, structural control-flow mechanics over SSA:
// instruction ordering, raw reachability, keyed work lists and edge selection.
// It does not infer value identity, call effects or feasible path conditions.
// Callers supply their state and successor policy and retain cutoff availability.
package cfg
