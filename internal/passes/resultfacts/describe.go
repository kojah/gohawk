package resultfacts

import (
	"fmt"
	"go/types"
)

// Fact dumps render each claim as one sentence, positions counting the
// receiver first as the claims do, so gohawk facts shows what a caller can
// rely on rather than the serialized encoding.

// DescribeFact renders the published fact for gohawk facts.
func (fact *publishedFact) DescribeFact(object types.Object) []string {
	return fact.Value().describe(object)
}

func (fact Fact) describe(object types.Object) []string {
	var lines []string
	if fact.NeverReturns {
		lines = append(lines, "never returns normally")
	}
	for index, guarantee := range fact.Results {
		if guarantee != Unknown {
			lines = append(lines, fmt.Sprintf("result %d%s is %s on every return", index, resultName(object, index), guarantee))
		}
	}
	for _, proven := range fact.Cases {
		lines = append(lines, fmt.Sprintf("result %d%s is %s when %s",
			proven.Result, resultName(object, proven.Result), proven.Outcome, proven.Condition))
	}
	for _, returned := range fact.Returned {
		lines = append(lines, fmt.Sprintf("result %d%s is parameter %d itself", returned.Result, resultName(object, returned.Result), returned.Parameter))
	}
	if len(lines) == 0 {
		lines = []string{"no result guarantee"}
	}
	return lines
}

// resultName names a result slot from the function's signature, when it is
// named or its type is short enough to say.
func resultName(object types.Object, index int) string {
	function, ok := object.(*types.Func)
	if !ok {
		return ""
	}
	results := function.Signature().Results()
	if index >= results.Len() {
		return ""
	}
	if name := results.At(index).Name(); name != "" {
		return " (" + name + ")"
	}
	qualifier := func(pkg *types.Package) string {
		if pkg == function.Pkg() {
			return ""
		}
		return pkg.Name()
	}
	return " (" + types.TypeString(results.At(index).Type(), qualifier) + ")"
}

// String names the guarantee.
func (guarantee Guarantee) String() string {
	switch guarantee {
	case AlwaysNil:
		return "nil"
	case AlwaysNonNil:
		return "non-nil"
	case AlwaysTrue:
		return "true"
	case AlwaysFalse:
		return "false"
	case Unknown:
	}
	return "unknown"
}
