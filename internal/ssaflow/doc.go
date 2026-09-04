// Package ssaflow provides mechanical control-flow, value, call, completion,
// and ownership-transfer facts for SSA-backed analyzers. It reports what the
// available SSA proves; each analyzer decides which facts satisfy its policy.
// Unknown evidence is never a positive result. Cross-package lifecycle
// summaries and imported provenance belong to internal/passes/lifecyclefacts.
// This internal API is unsupported and may change without notice.
//
// Files are named for the family they belong to, and the families are layered:
//
//	proof < value < call < flow < store < completion < evidence
//
// A file may use anything from a family to its left and nothing from a family
// to its right, which TestSSAFlowFamiliesLayerDownward enforces. The prefix
// therefore records what a file depends on, not what it is about: transfer
// evidence reads a call instruction but is named store because it rests on
// storage and control flow, and moving it to the call family would make it
// reach upward. A new file joins the family of the highest layer it needs.
package ssaflow
