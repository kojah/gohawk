package lifecyclefacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestReleasesEachElement(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "elementprobe", `package elementprobe
type File struct{}
func (*File) Close() error { return nil }
var kept [][]*File
func Each(files []*File) { for _, f := range files { _ = f.Close() } }
func First(files []*File) { if len(files) > 0 { _ = files[0].Close() } }
func Until(files []*File) error { for _, f := range files { if err := f.Close(); err != nil { return err } }; return nil }
func Skip(files []*File, skip func(*File) bool) { for _, f := range files { if skip(f) { continue }; _ = f.Close() } }
func Keep(files []*File) { kept = append(kept, files); for _, f := range files { _ = f.Close() } }
func Early(files []*File, early bool) { if early { return }; for _, f := range files { _ = f.Close() } }
func Twice(files []*File) { for _, f := range files { _ = f.Close() }; for _, f := range files { _ = f.Close() } }
`)
	for name, want := range map[string]bool{
		"Each": true, "First": false, "Until": false, "Skip": false, "Keep": false, "Early": false, "Twice": true,
	} {
		function := pkg.Func(name)
		if got := releasesEachElement(function, function.Params[0], "Close"); got != want {
			t.Errorf("%s: releasesEachElement = %t, want %t", name, got, want)
		}
	}
}
