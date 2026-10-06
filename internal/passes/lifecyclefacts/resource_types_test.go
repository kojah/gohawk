package lifecyclefacts

import (
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func resourceTestType(packagePath, name string) *types.Named {
	pkg := types.NewPackage(packagePath, "fixture")
	object := types.NewTypeName(token.NoPos, pkg, name, nil)
	return types.NewNamed(object, types.NewStruct(nil, nil), nil)
}

func TestResourceCleanupIdentityAndOwnership(t *testing.T) {
	for _, test := range []struct {
		packagePath string
		name        string
		methods     []string
	}{
		{"os", "File", []string{"Close"}},
		{"database/sql", "Tx", []string{"Commit", "Rollback"}},
		{"database/sql", "Rows", []string{"Close"}},
		{"database/sql", "Stmt", []string{"Close"}},
		{"net/http", "Response", []string{"Close"}},
		{"compress/gzip", "Reader", []string{"Close"}},
		{"compress/gzip", "Writer", []string{"Close"}},
		{"compress/zlib", "Writer", []string{"Close"}},
	} {
		named := resourceTestType(test.packagePath, test.name)
		for _, value := range []types.Type{named, types.NewPointer(named)} {
			methods, known := ResourceCleanup(value)
			if !known || !slices.Equal(methods, test.methods) {
				t.Fatalf("%s.%s cleanup %v, known %t", test.packagePath, test.name, methods, known)
			}
			if pkg, known := resourcePackage(value); !known || pkg != test.packagePath {
				t.Fatalf("%s.%s package %q, known %t", test.packagePath, test.name, pkg, known)
			}
			for index := range methods {
				methods[index] = "Changed"
			}
			methods = append(methods, "Added")
			fresh, _ := ResourceCleanup(value)
			if !slices.Equal(fresh, test.methods) {
				t.Fatalf("caller mutation %v changed later cleanup: %v", methods, fresh)
			}
		}
	}
	for _, value := range []types.Type{
		nil, types.Typ[types.Int], resourceTestType("example.com/user", "File"),
		resourceTestType("os", "Other"), resourceTestType("time", "Timer"),
		types.NewPointer(types.NewPointer(resourceTestType("os", "File"))),
	} {
		if methods, known := ResourceCleanup(value); known || methods != nil {
			t.Fatalf("unknown type %v gained cleanup %v", value, methods)
		}
		if pkg, known := resourcePackage(value); known || pkg != "" {
			t.Fatalf("unknown type %v gained package %q", value, pkg)
		}
	}
}

func BenchmarkResourceVocabulary(b *testing.B) {
	for _, test := range []struct {
		name  string
		value types.Type
	}{
		{"basic", types.Typ[types.Int]},
		{"unknown_named", resourceTestType("example.com/user", "File")},
		{"file", types.NewPointer(resourceTestType("os", "File"))},
		{"transaction", types.NewPointer(resourceTestType("database/sql", "Tx"))},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.Run("cleanup", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = ResourceCleanup(test.value)
				}
			})
			b.Run("package", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = resourcePackage(test.value)
				}
			})
		})
	}
}

func borrowedResourceValues(tb testing.TB) []ssa.Value {
	tb.Helper()
	pkg := ssaflowtest.BuildPackage(tb, "os", `package os
type File struct{}
func parameter(file *File) *File { return file }
func load(cell **File) *File { return *cell }
func zero() *File { return new(File) }
`)
	return []ssa.Value{
		pkg.Func("parameter").Params[0],
		ssaflow.InstructionsOf[*ssa.UnOp](pkg.Func("load"))[0],
		ssaflow.InstructionsOf[*ssa.Alloc](pkg.Func("zero"))[0],
	}
}

func TestResourceTypesDoNotEstablishAcquisition(t *testing.T) {
	for _, value := range borrowedResourceValues(t) {
		if _, known := ResourceCleanup(value.Type()); !known {
			t.Fatalf("fixture lost known resource type: %v", value)
		}
		if acquiredResource(nil, value) {
			t.Fatalf("non-call value gained acquisition evidence: %T %v", value, value)
		}
	}
}

func BenchmarkNonCallResourceAcquisition(b *testing.B) {
	values := borrowedResourceValues(b)
	for _, value := range values {
		b.Run(value.Name(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if acquiredResource(nil, value) {
					b.Fatal("non-call value gained acquisition evidence")
				}
			}
		})
	}
}
