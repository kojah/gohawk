package testlifecycle

import (
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestSupportsTestingContext(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "go1.15", want: false},
		{version: "go1.23.9", want: false},
		{version: "1.23.9", want: false},
		{version: "go1.24", want: true},
		{version: "1.24.0", want: true},
		{version: "go1.25.0", want: true},
		{version: "", want: true},
	}
	for _, test := range tests {
		pass := &analysis.Pass{Module: &analysis.Module{GoVersion: test.version}}
		if got := supportsTestingContext(pass); got != test.want {
			t.Errorf("supportsTestingContext(%q) = %t, want %t", test.version, got, test.want)
		}
	}
	if !supportsTestingContext(&analysis.Pass{}) {
		t.Error("supportsTestingContext without module = false, want true")
	}
}
