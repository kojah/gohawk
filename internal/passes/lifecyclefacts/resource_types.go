package lifecyclefacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// The resource type vocabulary names the standard-library types whose values
// carry a cleanup obligation. It is the only positive evidence that a value
// is a resource by its type alone; everything else must be proven by facts.

// resourceType names a type whose values carry a lifecycle obligation and the
// method that discharges it. This is a type vocabulary, distinct from the
// acquisition contracts an analyzer matches at call sites.
type resourceType struct {
	packagePath string
	name        string
	cleanup     []string
}

func resourceTypes() []resourceType {
	return []resourceType{
		{"os", "File", []string{"Close"}},
		{"database/sql", "Tx", []string{"Commit", "Rollback"}},
		{"database/sql", "Rows", []string{"Close"}},
		{"database/sql", "Stmt", []string{"Close"}},
		{"net/http", "Response", []string{"Close"}},
		{"compress/gzip", "Reader", []string{"Close"}},
		{"compress/gzip", "Writer", []string{"Close"}},
		{"compress/zlib", "Writer", []string{"Close"}},
		// Channel timers are GC-managed under modern Go semantics. Treating
		// their types as obligations would recreate missing-Stop reports via
		// inferred timer-only owners after the acquisition contract declined
		// them. A Timer type alone cannot prove an AfterFunc callback either.
	}
}

// ResponseBodyField recognizes a direct load of the Body field of a
// net/http Response. The body is the resource a response carries: closing it
// is the response's cleanup, so the type identity, not the field name alone,
// is what makes the load evidence. Whether the response is the one a caller
// acquired, and whether its Body was replaced, is the caller's policy.
func ResponseBodyField(value ssa.Value) *ssa.FieldAddr {
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok || !syntax.NamedType(field.X.Type(), "net/http", "Response") {
		return nil
	}
	structure := syntax.PointerStruct(field.X.Type())
	if structure == nil || structure.Field(field.Field).Name() != "Body" {
		return nil
	}
	return field
}

// ResourceCleanup returns the cleanup methods of a resource type, or false
// when the type carries no obligation this vocabulary knows.
func ResourceCleanup(value types.Type) ([]string, bool) {
	for _, entry := range resourceTypes() {
		if syntax.NamedType(value, entry.packagePath, entry.name) {
			return entry.cleanup, true
		}
	}
	return nil, false
}
