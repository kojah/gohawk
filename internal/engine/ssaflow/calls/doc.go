// Package calls resolves callees and argument/capture bindings and supplies
// reusable call contracts, effect summaries and bounded call-graph memoization.
// It consumes value and structural CFG mechanics; feasible path and obligation
// proofs belong to the higher path layer, and reporting policy to analyzers.
package calls
