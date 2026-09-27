package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintHeap(t *testing.T) {
	directory := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com/heapdump\n\ngo 1.25\n",
		"lib.go": `package heapdump

import "errors"

type box struct{ err error }

// Wrap stores a fresh error in a fresh box.
func Wrap() *box { return &box{err: errors.New("wrapped")} }

// Reset is a method of a type never converted to an interface, which
// ssautil.AllFunctions does not list.
func (b *box) Reset() { b.err = nil }
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
		without   []string
	}{
		{[]string{"-func", "Wrap", "."}, []string{
			"// example.com/heapdump.Wrap", "// regions:", "// stores at return:", "local:t0 field:0 -> opaque:t2", "applied errors.New", "summary ",
		}, []string{"func Wrap"}},
		{[]string{"-bare", "-func", "Wrap", "."}, []string{"// regions:"}, []string{"applied errors.New", "summary "}},
		{[]string{"-bare", "-func", "Reset", "."}, []string{"// (*example.com/heapdump.box).Reset"}, nil},
		{[]string{"-func", "Reset", "."}, []string{"// (*example.com/heapdump.box).Reset"}, nil},
		{[]string{"-ssa", "-func", "Wrap", "."}, []string{"func Wrap() *box:", "// regions:"}, nil},
	} {
		var output, errorsOutput bytes.Buffer
		if err := printHeap(test.arguments, &output, &errorsOutput); err != nil {
			t.Fatalf("%v: printHeap() error = %v, stderr %s", test.arguments, err, errorsOutput.String())
		}
		for _, want := range test.want {
			if !strings.Contains(output.String(), want) {
				t.Errorf("%v: dump lacks %q:\n%s", test.arguments, want, output.String())
			}
		}
		for _, unwanted := range test.without {
			if strings.Contains(output.String(), unwanted) {
				t.Errorf("%v: dump has %q:\n%s", test.arguments, unwanted, output.String())
			}
		}
	}
}
