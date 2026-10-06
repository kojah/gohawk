// Package ssaflow provides shared SSA value provenance, structural identity,
// source metadata and natural-loop mechanics. It uses proof for evidence and
// budgets and cfg for structural control flow. Calls and path proofs consume
// these mechanics from their own packages; this layer never depends on them.
package ssaflow
