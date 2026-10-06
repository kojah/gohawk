package lifecyclefacts

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// An alias changes spelling, not the error contract. Failed construction
// returns may contain errors, while unrelated owner results still prevent
// the body query from promising ownership on every successful return. Tuple
// delegation stays outside this query's result-specific containment proof.
func TestReturnedOwnerErrorAliases(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type resource struct{}
func (*resource) Close() error { return nil }
type owner struct { resource *resource }
type ErrorAlias = error
type ErrorLike interface { Error() string }
func Plain(p *resource, failure error, fail bool) (*owner, error) {
	if fail { return nil, failure }
	return &owner{resource: p}, nil
}
func Alias(p *resource, failure ErrorAlias, fail bool) (*owner, ErrorAlias) {
	if fail { return nil, failure }
	return &owner{resource: p}, nil
}
func DelegatedAlias(p *resource, failure ErrorAlias, fail bool) (*owner, ErrorAlias) {
	return Alias(p, failure, fail)
}
func DelegatedPlain(p *resource, failure error, fail bool) (*owner, error) {
	return Plain(p, failure, fail)
}
func UnrelatedOwner(p *resource, failure ErrorAlias, fail bool) (*owner, ErrorAlias) {
	if fail { return new(owner), failure }
	return &owner{resource: p}, nil
}
func DistinctError(p *resource, failure ErrorLike, fail bool) (*owner, ErrorLike) {
	if fail { return nil, failure }
	return &owner{resource: p}, nil
}
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]bool{
		"Plain":          true,
		"Alias":          true,
		"DelegatedAlias": false,
		"DelegatedPlain": false,
		"UnrelatedOwner": false,
		"DistinctError":  false,
	} {
		function := pkg.Func(name)
		if got := returnedOwnerOnEveryReturn(pass, function, function.Params[0]); got != want {
			t.Errorf("%s: error type %s, returned owner = %t, want %t", name, function.Signature.Results().At(1).Type(), got, want)
		}
	}
}
