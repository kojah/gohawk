package cli

import (
	"bytes"
	"testing"
)

func TestBudgetTopValidationBeforePackageLoading(t *testing.T) {
	for _, test := range []struct {
		name, top, want string
	}{
		{"negative", "-1", "-top must be nonnegative"},
		{"zero", "0", "at least one package pattern is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output, errorsOutput bytes.Buffer
			err := printBudget([]string{"-top=" + test.top}, &output, &errorsOutput)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if output.Len() != 0 || errorsOutput.Len() != 0 {
				t.Fatalf("validation emitted output: %q; stderr: %q", output.String(), errorsOutput.String())
			}
		})
	}
}
