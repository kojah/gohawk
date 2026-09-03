package ssaflow

import "golang.org/x/tools/go/ssa"

// Callee resolution is one question with one answer: which function does this
// call actually reach, in the form that carries evidence about it. A generic
// call may point at an instantiated wrapper, and that wrapper is the wrong
// place to look twice over. Its body may not survive instantiation, and a
// lifecycle summary is recorded against the origin's object, so a proof that
// reads the instantiation finds neither the source nor the facts.

// ResolvedCallee returns the callee a call reaches, answering a generic
// instantiation with its origin. The origin keeps the same parameter
// positions, so an argument index means the same thing in either form.
func ResolvedCallee(common *ssa.CallCommon) *ssa.Function {
	if common == nil {
		return nil
	}
	callee := common.StaticCallee()
	if callee == nil {
		return nil
	}
	return ResolvedFunction(callee)
}

// ResolvedFunction answers an instantiation with its origin for a function the
// caller already holds, such as the literal a launch names.
func ResolvedFunction(function *ssa.Function) *ssa.Function {
	if function == nil {
		return nil
	}
	if origin := function.Origin(); origin != nil {
		return origin
	}
	return function
}
