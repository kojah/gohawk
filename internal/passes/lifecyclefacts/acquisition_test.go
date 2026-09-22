package lifecyclefacts

import "testing"

func TestFreshResourceAcquisitionContracts(t *testing.T) {
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
type Manager struct { handles map[string]*os.File }
type Shared struct { file *os.File }
func (m *Manager) Open(path string) (*Shared, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	m.handles[path] = f
	return &Shared{file:f}, nil
}
func (o *Shared) Close() error { return o.file.Close() }
type Local struct { file *os.File }
func LocalOpen(path string) (*Local, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	local := map[string]*os.File{path:f}
	_ = local
	return &Local{file:f}, nil
}
func (o *Local) Close() error { return o.file.Close() }
`)
	for name, wantOwned := range map[string]bool{"Holder": false, "FileOwner": true, "Shared": false, "Local": true} {
		got := contracts[name]
		if owned := got != nil && got.Owned.contains(0); owned != wantOwned {
			t.Errorf("%s ownership = %v (%v), want %v", name, owned, got, wantOwned)
		}
	}
}
