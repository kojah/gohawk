package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
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

// Type checking resolves aliases and inferred declarations in the current build.
// The syntax gate still covers authored files excluded by the active build tags.
func TestNoTypedStringEnums(t *testing.T) {
	t.Parallel()
	inventory := newRepositorySourceInventory(t)
	authored := make(map[string]string)
	for _, source := range inventory.authoredGoFiles(t, ".") {
		authored[source.absolutePath] = source.repositoryPath
	}
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: inventory.root, Tests: true,
	}, "./...")
	if err != nil {
		t.Fatal(err)
	}
	if count := packages.PrintErrors(loaded); count > 0 {
		t.Fatalf("load authored packages: %d errors", count)
	}
	seen := make(map[token.Position]bool)
	for _, pkg := range loaded {
		for index, file := range pkg.Syntax {
			if index >= len(pkg.CompiledGoFiles) {
				continue
			}
			path, ok := authored[stableAbsolutePath(pkg.CompiledGoFiles[index])]
			if !ok {
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if !typedStringEnum(node, pkg.TypesInfo) {
					return true
				}
				position := pkg.Fset.Position(node.Pos())
				if !seen[position] {
					t.Errorf("%s:%d: classification domains must use numeric enums", path, position.Line)
					seen[position] = true
				}
				return true
			})
		}
	}
}

func typedStringEnum(node ast.Node, info *types.Info) bool {
	switch node := node.(type) {
	case *ast.FuncType:
		return classificationParameter(node, func(expression ast.Expr) bool { return stringUnderlying(info.TypeOf(expression)) })
	case *ast.TypeSpec:
		return classificationDomainName(node.Name.Name) && stringUnderlying(info.TypeOf(node.Name))
	case *ast.Field:
		for _, name := range node.Names {
			if classificationDiscriminatorName(name.Name) && stringUnderlying(info.TypeOf(node.Type)) {
				return true
			}
		}
	case *ast.ValueSpec:
		for _, name := range node.Names {
			if rawTypedDiscriminator(name, info) {
				return true
			}
		}
	case *ast.AssignStmt:
		for _, left := range node.Lhs {
			if name, ok := left.(*ast.Ident); ok && rawTypedDiscriminator(name, info) {
				return true
			}
		}
	case *ast.RangeStmt:
		for _, expression := range []ast.Expr{node.Key, node.Value} {
			if name, ok := expression.(*ast.Ident); ok && rawTypedDiscriminator(name, info) {
				return true
			}
		}
	}
	return false
}

func rawTypedDiscriminator(name *ast.Ident, info *types.Info) bool {
	return classificationDiscriminatorName(name.Name) && stringUnderlying(info.TypeOf(name))
}

func stringUnderlying(value types.Type) bool {
	if value == nil {
		return false
	}
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsString != 0
}

func TestTypedStringEnumMatcher(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         bool
	}{
		{"local defined domain", `type text string; type CheckKind text`, true},
		{"local alias domain", `type text = string; type CheckMode = text`, true},
		{"imported string domain", `type CheckState = wire.Text`, true},
		{"aliased field", `type item struct{ Kind wire.Text }`, true},
		{"aliased phase parameter", `func f(phase wire.Text){}`, true},
		{"aliased mode parameter", `func f(mode wire.Text){}`, true},
		{"aliased family parameter", `func f(family wire.Text){}`, true},
		{"aliased family field", `type contract struct{family wire.Text}`, true},
		{"indirect family domain", `type resourceFamily wire.Text`, true},
		{"numeric family parameter", `func f(family wire.Number){}`, false},
		{"serialized mode field", `type record struct{Mode wire.Text}`, false},
		{"numeric mode parameter", `func f(mode wire.Number){}`, false},
		{"serialized phase field", `type record struct{Phase wire.Text}`, false},
		{"indirect phase domain", `type QueryPhase wire.Text`, true},
		{"aliased parameter", `func f(kind wire.Text){}`, true},
		{"aliased result", `func f()(kind wire.Text){return ""}`, true},
		{"inferred variable", `func text()string{return ""}; var kind = text()`, true},
		{"inferred assignment", `func text()string{return ""}; func f(){kind:=text();_=kind}`, true},
		{"tuple assignment", `func text()(string,int){return "",0}; func f(){kind,n:=text();_,_=kind,n}`, true},
		{"range declaration", `func f(){for _,kind:=range []string{"a"}{_=kind}}`, true},
		{"numeric import", `type CheckKind = wire.Number; func f(kind CheckKind){}`, false},
		{"numeric inferred", `func number()wire.Number{return 0}; func f(){kind:=number();_=kind}`, false},
		{"text identity", `type AnalyzerID wire.Text; func f(prefix wire.Text){}`, false},
		{"fixture text", `const fixture="type Outcome string"`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fileSet := token.NewFileSet()
			source := "package fixture\n"
			if strings.Contains(test.source, "wire.") {
				source += "import \"wire\"\n"
			}
			file, err := parser.ParseFile(fileSet, "fixture.go", source+test.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{
				Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object),
			}
			config := types.Config{Importer: enumFixtureImporter{}}
			if _, err := config.Check("fixture", fileSet, []*ast.File{file}, info); err != nil {
				t.Fatal(err)
			}
			found := false
			ast.Inspect(file, func(node ast.Node) bool {
				found = found || typedStringEnum(node, info)
				return true
			})
			if found != test.want {
				t.Errorf("typedStringEnum() = %t, want %t", found, test.want)
			}
		})
	}
}

type enumFixtureImporter struct{}

func (enumFixtureImporter) Import(path string) (*types.Package, error) {
	pkg := types.NewPackage(path, "wire")
	for name, basic := range map[string]*types.Basic{"Text": types.Typ[types.String], "Number": types.Typ[types.Uint8]} {
		object := types.NewTypeName(token.NoPos, pkg, name, nil)
		types.NewNamed(object, basic, nil)
		pkg.Scope().Insert(object)
	}
	pkg.MarkComplete()
	return pkg, nil
}

func reasonEnumViolation(node ast.Node) bool {
	switch node := node.(type) {
	case *ast.TypeSpec:
		if !strings.HasSuffix(node.Name.Name, "Reason") {
			return false
		}
		underlying, ok := node.Type.(*ast.Ident)
		if !ok || node.Assign.IsValid() {
			return true
		}
		switch underlying.Name {
		case "uint8", "uint16", "uint32", "uint64", "uint", "int8", "int16", "int32", "int64", "int":
			return false
		}
		return true
	case *ast.StructType:
		for _, field := range node.Fields.List {
			underlying, ok := field.Type.(*ast.Ident)
			if !ok || underlying.Name != "string" {
				continue
			}
			for _, name := range field.Names {
				if name.Name == "reason" || strings.HasSuffix(name.Name, "Reason") {
					return true
				}
			}
		}
	}
	return false
}

func TestReasonEnumBoundaryMatcher(t *testing.T) {
	for _, test := range []struct {
		source string
		want   bool
	}{
		{"type QueryReason uint8", false},
		{"type QueryReason string", true},
		{"type QueryReason = uint8", true},
		{"type Proof struct { Reason string }", true},
		{"type Proof struct { reason string }", true},
		{"type Proof struct { Reason QueryReason }", false},
		{"type Observer func(reason string)", false},
		{"func (reason QueryReason) String() string { return \"unknown\" }", false},
	} {
		assertReasonMatcher(t, test.source, test.want, reasonEnumViolation)
	}
}

func assertReasonMatcher(t *testing.T, source string, want bool, match func(ast.Node) bool) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "reason.go", "package fixture\n"+source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	ast.Inspect(file, func(node ast.Node) bool {
		found = found || match(node)
		return true
	})
	if found != want {
		t.Errorf("%s: violation=%t, want %t", source, found, want)
	}
}

func reasonTextBoundary(path string) bool {
	return path == "internal/reporting/trace/trace.go" || path == "internal/engine/proof/observer.go"
}

// Raw reason classifications are forbidden outside the textual output boundary.
// Syntax checks cover common forms; review must also catch unnamed return
// values and synthesized codes whose semantic role is not evident from syntax.
func TestNoRawReasonClassifications(t *testing.T) {
	t.Parallel()
	for _, source := range newRepositorySourceInventory(t).productionGoFiles(t, ".") {
		if reasonTextBoundary(source.repositoryPath) {
			continue
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			if reasonEnumViolation(node) || rawReasonClassification(node) {
				t.Errorf("%s:%d: reason classifications must use domain-owned enums", source.repositoryPath, source.fileSet.Position(node.Pos()).Line)
			}
			return true
		})
	}
}

func rawReasonClassification(node ast.Node) bool {
	switch node := node.(type) {
	case *ast.TypeSpec:
		return reasonEnumViolation(node)
	case *ast.Field:
		return rawReasonField(node)
	case *ast.KeyValueExpr:
		key, ok := node.Key.(*ast.Ident)
		return ok && (key.Name == "Reason" || key.Name == "reason") && reasonStringLiteral(node.Value)
	case *ast.ValueSpec:
		if len(node.Names) != len(node.Values) {
			return false
		}
		for index, name := range node.Names {
			if reasonAssignmentTarget(name) && reasonStringLiteral(node.Values[index]) {
				return true
			}
		}
	case *ast.AssignStmt:
		if len(node.Lhs) != len(node.Rhs) {
			return false
		}
		for index, left := range node.Lhs {
			if reasonAssignmentTarget(left) && reasonStringLiteral(node.Rhs[index]) {
				return true
			}
		}
	}
	return false
}

func rawReasonField(field *ast.Field) bool {
	kind, ok := field.Type.(*ast.Ident)
	if !ok || kind.Name != "string" {
		return false
	}
	for _, name := range field.Names {
		if name.Name == "reason" || strings.HasSuffix(name.Name, "Reason") {
			return true
		}
	}
	return false
}

func reasonAssignmentTarget(expression ast.Expr) bool {
	switch expression := expression.(type) {
	case *ast.Ident:
		return strings.HasPrefix(expression.Name, "reason") || strings.HasSuffix(expression.Name, "Reason")
	case *ast.SelectorExpr:
		return expression.Sel.Name == "Reason" || expression.Sel.Name == "reason"
	}
	return false
}

func reasonStringLiteral(expression ast.Expr) bool {
	if binary, ok := ast.Unparen(expression).(*ast.BinaryExpr); ok && binary.Op == token.ADD {
		return reasonStringLiteral(binary.X) || reasonStringLiteral(binary.Y)
	}
	literal, ok := ast.Unparen(expression).(*ast.BasicLit)
	return ok && literal.Kind == token.STRING
}

func TestRawReasonClassificationMatcher(t *testing.T) {
	cases := []classificationFixture{
		{"type Proof struct { Reason string }", true},
		{"type Reason string", true},
		{"func f(reason string) {}", true},
		{"func f() { reason := \"unknown\" }", true},
		{"func f() { proof.Reason = \"unknown\" }", true},
		{"const reasonUnknown = \"unknown\"", true},
		{"func f() { reason := \"prefix-\" + suffix }", true},
		{"var missingReason = \"unknown\"", true},
		{"var proof = Proof{Reason: \"unknown\"}", true},
		{"type QueryReason uint8; type Proof struct { Reason QueryReason }", false},
		{"func f() { proof.Reason = UnknownReason }", false},
		{"func f() string { return \"display text\" }", false},
	}
	assertClassificationFixtures(t, cases, rawReasonClassification)
}

type classificationFixture struct {
	source string
	want   bool
}

func assertClassificationFixtures(t *testing.T, cases []classificationFixture, matches func(ast.Node) bool) {
	t.Helper()
	for _, test := range cases {
		assertReasonMatcher(t, test.source, test.want, matches)
	}
}
