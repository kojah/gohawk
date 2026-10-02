package ssaflow

import "golang.org/x/tools/go/ssa"

// These probes expose the production decoders to external-package tests.
// Production consumers obtain guard identities through setup and transitions.
func GuardCondition(condition ssa.Value) (identity string, negated, stable, ok bool) {
	return guardConditionWithin(condition, nil)
}

func GuardAddressIdentity(address ssa.Value) (string, bool) {
	return guardAddressIdentityWithin(address, nil)
}
