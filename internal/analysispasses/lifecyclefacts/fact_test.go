package lifecyclefacts

import "testing"

func TestParameterMask(t *testing.T) {
	mask := parameterMaskFor(1) | parameterMaskFor(63)
	for _, test := range []struct {
		index int
		want  bool
	}{
		{index: -1},
		{index: 0},
		{index: 1, want: true},
		{index: 63, want: true},
		{index: 64},
	} {
		if got := mask.contains(test.index); got != test.want {
			t.Errorf("mask.contains(%d) = %t, want %t", test.index, got, test.want)
		}
	}
}
