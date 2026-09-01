// Package ssaflow provides mechanical control-flow, value, call, completion,
// and ownership-transfer facts for SSA-backed analyzers. It reports what the
// available SSA proves; each analyzer decides which facts satisfy its policy.
// Unknown evidence is never a positive result. Cross-package lifecycle
// summaries and imported provenance belong to internal/passes/lifecyclefacts.
// This internal API is unsupported and may change without notice.
package ssaflow
