package ssaflow

import "testing"

func TestHasSolePlainWrapDirective(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{name: "plain wrap", format: "operation failed: %w", want: true},
		{name: "escaped percent", format: "operation 100%% failed: %w", want: true},
		{name: "no wrap", format: "operation failed: %v"},
		{name: "multiple wraps", format: "%w: %w"},
		{name: "additional directive", format: "%w: %v"},
		{name: "flagged wrap", format: "%+w"},
		{name: "indexed wrap", format: "%[1]w"},
		{name: "incomplete directive", format: "operation failed: %"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasSolePlainWrapDirective(test.format); got != test.want {
				t.Errorf("hasSolePlainWrapDirective(%q) = %t, want %t", test.format, got, test.want)
			}
		})
	}
}
