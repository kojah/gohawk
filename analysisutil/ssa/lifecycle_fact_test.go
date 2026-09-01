package ssautil

import "testing"

func TestParameterMask(t *testing.T) {
	mask := ParameterMaskFor(1) | ParameterMaskFor(63)
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
		if got := mask.Contains(test.index); got != test.want {
			t.Errorf("mask.Contains(%d) = %t, want %t", test.index, got, test.want)
		}
	}
}
