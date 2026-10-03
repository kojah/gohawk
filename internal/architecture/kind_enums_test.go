package architecture

import (
	"go/ast"
	"slices"
	"strings"
	"testing"
)

// Kind and family are internal discriminators. Text at an output boundary should name
// its role (a noun, label, or prefix), rather than masquerading as an enum.
// This syntax gate covers raw fields, parameters, results and literal discriminator
// assignments, including tests, and raw phase/mode parameters. Serialized phase fields
// remain text at the output boundary. Named string domains are checked below.
func TestNoRawKindEnums(t *testing.T) {
	t.Parallel()
	assertNoStringEnums(t, rawKindEnum)
}

func rawKindEnum(node ast.Node) bool {
	isDiscriminator := func(name *ast.Ident) bool { return classificationDiscriminatorName(name.Name) }
	switch node := node.(type) {
	case *ast.FuncType:
		return classificationParameter(node, func(expression ast.Expr) bool {
			underlying, ok := expression.(*ast.Ident)
			return ok && underlying.Name == "string"
		})
	case *ast.Field:
		underlying, ok := node.Type.(*ast.Ident)
		if !ok || underlying.Name != "string" {
			return false
		}
		return slices.ContainsFunc(node.Names, isDiscriminator)
	case *ast.ValueSpec:
		underlying, explicit := node.Type.(*ast.Ident)
		for index, name := range node.Names {
			if !isDiscriminator(name) {
				continue
			}
			if explicit && underlying.Name == "string" {
				return true
			}
			if len(node.Values) == len(node.Names) && reasonStringLiteral(node.Values[index]) {
				return true
			}
		}
	case *ast.AssignStmt:
		if len(node.Lhs) != len(node.Rhs) {
			return false
		}
		for index, left := range node.Lhs {
			name, ok := left.(*ast.Ident)
			if ok && isDiscriminator(name) && reasonStringLiteral(node.Rhs[index]) {
				return true
			}
		}
	}
	return false
}

func TestRawKindEnumMatcher(t *testing.T) {
	for _, test := range []struct {
		source string
		want   bool
	}{
		{`func f(kind string){}`, true},
		{`func f(phase string){}`, true},
		{`func f(mode string){}`, true},
		{`func f(family string){}`, true},
		{`type contract struct{family string}`, true},
		{`type resourceFamily uint8;func f(family resourceFamily){}`, false},
		{`type record struct{Mode string}`, false},
		{`type lockMode uint8;func f(mode lockMode){}`, false},
		{`type record struct{Phase string}`, false},
		{`type queryPhase uint8;func f(phase queryPhase){}`, false},
		{`func f()(kind,title string){return "",""}`, true},
		{`type item struct{Kind string}`, true},
		{`var kind string`, true},
		{`func f(){kind:="value";_=kind}`, true},
		{`type queryKind uint8;func f(kind queryKind){}`, false},
		{`type words struct{noun string}`, false},
		{`func f(prefix string){}`, false},
		{`func f(){kind:=valueKind;_=kind}`, false},
		{`const fixture = "func f(kind string){}"`, false},
	} {
		assertReasonMatcher(t, test.source, test.want, rawKindEnum)
	}
}

// Classification suffixes name closed decision domains, whose strings belong
// only at output boundaries. The typed gate supplements this syntax check for
// indirect aliases; neither infers semantic roles from arbitrary names.
func TestNoNamedStringEnums(t *testing.T) {
	t.Parallel()
	assertNoStringEnums(t, namedStringEnum)
}

func namedStringEnum(node ast.Node) bool {
	declaration, ok := node.(*ast.TypeSpec)
	if !ok {
		return false
	}
	underlying, ok := declaration.Type.(*ast.Ident)
	if !ok || underlying.Name != "string" {
		return false
	}
	return classificationDomainName(declaration.Name.Name)
}

func classificationDomainName(name string) bool {
	suffixes := []string{"Kind", "Tier", "Outcome", "Provenance", "Reason", "State", "Mode", "Action", "Phase", "Family"}
	return slices.ContainsFunc(suffixes, func(suffix string) bool {
		return strings.HasSuffix(name, suffix) || strings.EqualFold(name, suffix)
	})
}

func TestNamedStringEnumMatcher(t *testing.T) {
	cases := []classificationFixture{
		{`type CheckKind string`, true},
		{`type CheckTier = string`, true},
		{`type Outcome string`, true},
		{`type Phase string`, true},
		{`type resourceFamily string`, true},
		{`type resourceFamily uint8`, false},
		{`type EvidenceProvenance string`, true},
		{`type reason string`, true},
		{`type CheckKind uint8`, false},
		{`type CheckKind = catalog.CheckKind`, false},
		{`type AnalyzerID string`, false},
		{`const fixture="type Outcome string"`, false},
	}
	assertClassificationFixtures(t, cases, namedStringEnum)
}

func assertNoStringEnums(t *testing.T, matches func(ast.Node) bool) {
	t.Helper()
	for _, source := range newRepositorySourceInventory(t).authoredGoFiles(t, ".") {
		ast.Inspect(source.file, func(node ast.Node) bool {
			if matches(node) {
				t.Errorf("%s:%d: classification domains must use numeric enums", source.repositoryPath, source.fileSet.Position(node.Pos()).Line)
			}
			return true
		})
	}
}

// classificationParameter shares phase/mode/family parameter selection between syntax and type evidence.
// Serialized fields are not parameters and retain their wire vocabulary.
func classificationParameter(function *ast.FuncType, isString func(ast.Expr) bool) bool {
	if function.Params == nil {
		return false
	}
	return slices.ContainsFunc(function.Params.List, func(field *ast.Field) bool {
		return isString(field.Type) && slices.ContainsFunc(field.Names, func(name *ast.Ident) bool {
			return classificationDiscriminatorName(name.Name) || strings.EqualFold(name.Name, "phase") || strings.EqualFold(name.Name, "mode")
		})
	})
}

// classificationDiscriminatorName selects closed fields and local declarations.
// Phase and mode have textual wire fields, so only their parameters use this gate.
func classificationDiscriminatorName(name string) bool {
	return strings.EqualFold(name, "kind") || strings.EqualFold(name, "family")
}
