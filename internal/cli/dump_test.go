package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestDumpListsViewsAndRejectsUnknownOnes(t *testing.T) {
	var output, errorsOutput bytes.Buffer
	err := runDump([]string{"bogus"}, &output, &errorsOutput)
	if err == nil || !strings.Contains(err.Error(), `unknown view "bogus"`) {
		t.Fatalf("runDump(bogus) error = %v, want unknown view", err)
	}
	for _, view := range dumpViews() {
		if !strings.Contains(errorsOutput.String(), "  "+view.name+" ") {
			t.Errorf("usage lacks view %q:\n%s", view.name, errorsOutput.String())
		}
	}
	if err := runDump(nil, &output, &errorsOutput); err == nil {
		t.Error("runDump() without a view succeeded, want error")
	}
}
