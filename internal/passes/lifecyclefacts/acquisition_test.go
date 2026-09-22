package lifecyclefacts

import "testing"

func TestCustomCloserDoesNotEstablishAcquisition(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest
import "os"
type Lazy struct { file *os.File }
func NewLazy() *Lazy { return &Lazy{} }
func (l *Lazy) Close() error {
	if l.file != nil { return l.file.Close() }
	return nil
}
type Holder struct { lazy *Lazy }
func NewHolder() *Holder { return &Holder{lazy:NewLazy()} }
func (h *Holder) Close() error { return h.lazy.Close() }
type FileOwner struct { file *os.File }
func NewFileOwner(path string) (*FileOwner, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	return &FileOwner{file:f}, nil
}
func (o *FileOwner) Close() error { return o.file.Close() }
`)
	if got := contracts["Holder"]; got != nil {
		t.Errorf("unacquired custom closer invented an owner contract: %v", got)
	}
	if got := contracts["FileOwner"]; got == nil || !got.Owned.contains(0) {
		t.Errorf("direct file acquisition contract = %v, want owned field 0", got)
	}
}
