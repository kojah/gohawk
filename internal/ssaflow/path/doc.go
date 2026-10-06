// Package path proves feasible paths, exact counted regions and obligation
// coverage over SSA. It combines value identity and call evidence with CFG
// mechanics. Callers supply their actions and obligations; unavailable evidence
// never becomes a definite path or a reported violation by this package alone.
package path
