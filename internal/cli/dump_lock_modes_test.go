package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDumpLockReadModes(t *testing.T) {
	directory := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/lockmodes\n\ngo 1.25\n",
		"lib.go": `package lockmodes
import "sync"
var reader sync.RWMutex
var writer, next sync.Mutex
func readFirst(){reader.RLock();writer.Lock();writer.Unlock();reader.RUnlock()}
func readSecond(){writer.Lock();reader.RLock();reader.RUnlock();writer.Unlock()}
func exclusive(){writer.Lock();next.Lock();next.Unlock();writer.Unlock()}
`,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(directory)
	for _, test := range []struct {
		arguments []string
		want      []string
		without   string
	}{
		{[]string{"."}, []string{"reader (read) -> writer", "writer -> reader (read)", "writer -> next"}, "next (read)"},
		{[]string{"-dot", "."}, []string{
			`"reader" -> "writer" [label="lib.go:5", color=red, style=dashed]`,
			`"writer" -> "reader" [label="lib.go:6", color=red, style=dashed]`,
			`"writer" -> "next" [label="lib.go:7"]`,
		}, `"writer" -> "next" [label="lib.go:7", style=dashed]`},
	} {
		var output, errorsOutput bytes.Buffer
		if err := printLocks(test.arguments, &output, &errorsOutput); err != nil {
			t.Fatalf("printLocks(%v): %v; stderr %s", test.arguments, err, errorsOutput.String())
		}
		for _, want := range test.want {
			if !strings.Contains(output.String(), want) {
				t.Errorf("dump lacks %q:\n%s", want, output.String())
			}
		}
		if strings.Contains(output.String(), test.without) {
			t.Errorf("dump has %q:\n%s", test.without, output.String())
		}
	}
}
