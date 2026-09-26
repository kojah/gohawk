package concurrencyfacts

import (
	"slices"
	"testing"
)

func TestDescribeFactRendersAlternatives(t *testing.T) {
	fact := Fact{Alternatives: []FactAlternative{
		{
			Effects:    []Effect{{Kind: Close, Parameter: 0}},
			Conditions: []FactCondition{{Parameter: 2, Constant: FactConstant{Kind: FactConstantInt, Exact: "1"}, Holds: true}},
		},
		{
			Effects:    []Effect{{Kind: Lock, Parameter: 1, Fields: []int{0}}},
			Conditions: []FactCondition{{Parameter: -1, Internal: 0}},
			Returned:   []FactReturned{{Index: 0, Constant: FactConstant{Kind: FactConstantNil}}},
		},
	}}
	want := []string{
		"path 1 when parameter 2 == 1:",
		"  close parameter 0",
		"path 2 when its own test 0 does not hold, returning result 0 nil:",
		"  Lock parameter 1 field 0",
	}
	if got := fact.describe(); !slices.Equal(got, want) {
		t.Errorf("describe() = %q, want %q", got, want)
	}
	if got := (Fact{}).describe(); !slices.Equal(got, []string{"no synchronization effects"}) {
		t.Errorf("empty describe() = %q", got)
	}
}
