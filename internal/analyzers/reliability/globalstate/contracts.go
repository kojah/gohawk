package globalstate

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

func qualifiedTypeName(value types.Type) string {
	if pointer, ok := value.(*types.Pointer); ok {
		value = pointer.Elem()
	}
	named, ok := value.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	packagePath := named.Obj().Pkg().Path()
	// GOPATH-style fixtures can expose the physical vendor prefix in the type
	// path. Production packages normally use the canonical import path, so
	// normalize both representations before applying package API contracts.
	if _, suffix, found := strings.Cut(packagePath, "/vendor/"); found {
		packagePath = suffix
	}
	packagePath = strings.TrimPrefix(packagePath, "vendor/")
	return packagePath + "." + named.Obj().Name()
}

func readOnlyCollectionMethodCall(pass *analysis.Pass, call *ast.CallExpr, target ast.Node) bool {
	// ResponseWriter.Write follows the io.Writer contract: it neither modifies
	// nor retains its byte-slice argument. Keep exact receiver identity so an
	// application-defined Write method cannot establish immutability by name.
	return collectionArgumentIndex(call, target) == 0 && syntax.IsCallTo(pass, call, syntax.PackageMethod(syntax.MethodSymbol{
		PackagePath: "net/http",
		Receiver:    "ResponseWriter",
		Name:        "Write",
	}))
}

func conventionalFrameworkBinding(pass *analysis.Pass, specification *ast.ValueSpec, index int) bool {
	if index >= len(specification.Values) {
		return false
	}
	selector, ok := specification.Values[index].(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "AddToScheme" {
		return false
	}
	qualified := qualifiedTypeName(pass.TypesInfo.TypeOf(selector.X))
	// AddToScheme is a method value exported by the controller-runtime
	// registration builder as the canonical package-level hook; the builder is
	// the owner already recognized above.
	return qualified == "k8s.io/apimachinery/pkg/runtime.SchemeBuilder" || qualified == "sigs.k8s.io/controller-runtime/pkg/scheme.Builder"
}

func benchmarkResultSink(pass *analysis.Pass, name *ast.Ident) bool {
	lower := strings.ToLower(name.Name)
	// Benchmark sinks deliberately escape results to defeat dead-code
	// elimination and are scoped to the test binary. ccLoad uses this standard
	// pattern for allocation-sensitive storage benchmarks:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/storage/cache_benchmark_test.go#L10-L15
	return !name.IsExported() && strings.HasSuffix(pass.Fset.Position(name.Pos()).Filename, "_test.go") && strings.Contains(lower, "benchmark") &&
		strings.HasSuffix(lower, "sink")
}

func conventionalFrameworkGlobal(value types.Type) bool {
	qualified := qualifiedTypeName(value)
	switch qualified {
	case "github.com/prometheus/client_golang/prometheus.Counter",
		"github.com/prometheus/client_golang/prometheus.CounterVec",
		"github.com/prometheus/client_golang/prometheus.Gauge",
		"github.com/prometheus/client_golang/prometheus.GaugeVec",
		"github.com/prometheus/client_golang/prometheus.Histogram",
		"github.com/prometheus/client_golang/prometheus.HistogramVec",
		"github.com/prometheus/client_golang/prometheus.Summary",
		"github.com/prometheus/client_golang/prometheus.SummaryVec",
		"github.com/spf13/cobra.Command",
		"go.uber.org/fx.Option",
		"k8s.io/apimachinery/pkg/runtime.Scheme",
		"k8s.io/apimachinery/pkg/runtime.SchemeBuilder",
		"sigs.k8s.io/controller-runtime/pkg/scheme.Builder":
		// These APIs intentionally construct process-wide collectors,
		// registration trees, or immutable dependency-injection descriptors.
		// Their packages own synchronization and one-time registration, so a
		// local wrapper would obscure rather than improve ownership.
		// Prometheus collectors in FRP Operator are representative:
		// https://github.com/zufardhiyaulhaq/frp-operator/blob/1864d2a2926edd6396cde9030672bd1a4329c37e/pkg/metrics/metrics.go#L20-L57
		return true
	default:
		return false
	}
}

func conventionalAnalyzerSingleton(pass *analysis.Pass, object types.Object, value types.Type, usage globalStateUsage) bool {
	if qualifiedTypeName(value) != "golang.org/x/tools/go/analysis.Analyzer" {
		return false
	}
	// analysis.Analyzer pointers are stable identities used as ResultOf keys by
	// x/tools prerequisites. Reassignment or taking the binding's address breaks
	// that singleton evidence and makes it ordinary mutable package state:
	// https://github.com/golang/tools/blob/18332fec72972efbb8ab9881984fec2d8cfc2b58/go/analysis/passes/buildssa/buildssa.go#L22-L28
	return !globalObjectReassignedOrAddressed(pass, object, usage)
}

func immutableRuntimeDescriptor(pass *analysis.Pass, object types.Object, value types.Type, usage globalStateUsage) bool {
	if qualifiedTypeName(value) != "reflect.Type" {
		return false
	}
	// reflect.Type is implemented by the standard library's immutable runtime
	// descriptors; its unexported methods prevent application implementations.
	// A stable package binding is therefore immutable evidence, while assignment
	// or taking its address still invalidates that evidence. Echo caches these
	// descriptors for multipart binding:
	// https://github.com/labstack/echo/blob/07b3c4a4d4cc077a653b4e7a5de5d4acb121ce7b/bind.go#L498-L502
	return !globalObjectReassignedOrAddressed(pass, object, usage)
}

func immutableStringReplacer(
	pass *analysis.Pass,
	name *ast.Ident,
	object types.Object,
	specification *ast.ValueSpec,
	index int,
	usage globalStateUsage,
) bool {
	if name.IsExported() || index >= len(specification.Values) {
		return false
	}
	initializer, ok := syntax.Unparen(specification.Values[index]).(*ast.CallExpr)
	if !ok || !syntax.IsCallTo(pass, initializer, syntax.PackageFunction("strings", "NewReplacer")) ||
		globalObjectReassignedOrAddressed(pass, object, usage) {
		return false
	}
	used := false
	for _, file := range usage.files {
		unsafe := false
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, identifierOK := node.(*ast.Ident)
			if !identifierOK || pass.TypesInfo.Uses[identifier] != object {
				return true
			}
			used = true
			if !directStringReplacerCall(pass, identifier, usage.parents) {
				unsafe = true
				return false
			}
			return true
		})
		if unsafe {
			return false
		}
	}
	if !used {
		return false
	}
	// strings.Replacer copies its constructor input and documents concurrent
	// safety. Restrict acceptance to its two exact observational operations so
	// aliases or future API expansion cannot silently widen this contract.
	// Aerospike uses one stable replacer to encode JSON Pointer components:
	// https://github.com/aerospike/aerospike-kubernetes-operator/blob/7c00e3a9d57d7fc65f00930e32d7540bf4a9a18f/pkg/jsonpatch/jsonpatch.go#L153-L156
	traceImmutableStringReplacer(pass, name)
	return true
}

func directStringReplacerCall(pass *analysis.Pass, identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	receiver, parent := unparenthesizedUse(identifier, parents)
	selector, ok := parent.(*ast.SelectorExpr)
	if !ok || selector.X != receiver {
		return false
	}
	selected, parent := unparenthesizedUse(selector, parents)
	call, ok := parent.(*ast.CallExpr)
	if !ok || call.Fun != selected {
		return false
	}
	for _, method := range []string{"Replace", "WriteString"} {
		if syntax.IsCallTo(pass, call, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "strings", Receiver: "Replacer", Name: method})) {
			return true
		}
	}
	return false
}

func traceImmutableStringReplacer(pass *analysis.Pass, name *ast.Ident) {
	traceAcceptedGlobal(pass, name, "immutable-stdlib-replacer")
}

// traceAcceptedGlobal records why a package-level variable is accepted as
// effectively immutable.
func traceAcceptedGlobal(pass *analysis.Pass, name *ast.Ident, reason string) {
	checkID := string(check.MutableGlobalState)
	analysisTrace.For(pass, "globalstate", checkID, name.Pos()).Evidence(analysisTrace.Step{
		Reason:  reason,
		Outcome: analysisTrace.OutcomeAccepted,
		Pos:     name.Pos(),
	})
}

func conventionalErrorSentinel(
	pass *analysis.Pass,
	name *ast.Ident,
	object types.Object,
	specification *ast.ValueSpec,
	index int,
	usage globalStateUsage,
) bool {
	if index >= len(specification.Values) || !errorSentinelName(name.Name) || !syntax.IsErrorType(object.Type()) {
		return false
	}
	initializer := specification.Values[index]
	if !syntax.IsErrorType(pass.TypesInfo.TypeOf(initializer)) {
		return false
	}
	// ErrFoo and errFoo are Go's conventional names for stable sentinel
	// identities. Recognizing the contract rather than individual constructors
	// covers standard and third-party error packages without an expanding
	// allowlist. ElasticKV uses github.com/cockroachdb/errors this way:
	// https://github.com/bootjp/elastickv/blob/ddbb0a5b60a691890cb5595c185cdb16fee478b3/adapter/admin_backup.go#L45-L46
	// A later assignment or taking the variable's address invalidates that
	// evidence and makes it ordinary mutable package state again.
	return !globalObjectReassignedOrAddressed(pass, object, usage)
}

func errorSentinelName(name string) bool {
	if name == "Err" || name == "err" {
		return true
	}
	return len(name) > 3 && (strings.HasPrefix(name, "Err") || strings.HasPrefix(name, "err")) && name[3] >= 'A' && name[3] <= 'Z'
}

func globalObjectReassignedOrAddressed(pass *analysis.Pass, object types.Object, usage globalStateUsage) bool {
	for _, file := range usage.files {
		unsafe := false
		ast.Inspect(file, func(node ast.Node) bool {
			if unsafe {
				return false
			}
			unsafe = globalUseIsUnsafe(pass, node, object, usage.parents)
			return !unsafe
		})
		if unsafe {
			return true
		}
	}
	return false
}

func globalUseIsUnsafe(pass *analysis.Pass, node ast.Node, object types.Object, parents map[ast.Node]ast.Node) bool {
	identifier, ok := node.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[identifier] != object {
		return false
	}
	current, parent := unparenthesizedUse(identifier, parents)
	switch typed := parent.(type) {
	case *ast.AssignStmt:
		return assignmentTargetsNode(typed, current)
	case *ast.IncDecStmt:
		return typed.X == current
	case *ast.RangeStmt:
		return typed.Key == current || typed.Value == current
	case *ast.UnaryExpr:
		return typed.Op == token.AND
	default:
		return false
	}
}

func assignmentTargetsNode(assignment *ast.AssignStmt, node ast.Node) bool {
	for _, target := range assignment.Lhs {
		if target == node {
			return true
		}
	}
	return false
}

func unparenthesizedUse(node ast.Node, parents map[ast.Node]ast.Node) (ast.Node, ast.Node) {
	current, parent := node, parents[node]
	for {
		parenthesized, ok := parent.(*ast.ParenExpr)
		if !ok {
			return current, parent
		}
		current, parent = parenthesized, parents[parenthesized]
	}
}

func documentedTestSeam(name *ast.Ident, object types.Object, declaration *ast.GenDecl, specification *ast.ValueSpec) bool {
	if name.IsExported() {
		return false
	}
	switch object.Type().Underlying().(type) {
	case *types.Signature, *types.Interface:
	default:
		return false
	}
	comment := strings.ToLower(globalDeclarationComment(declaration, specification))
	if !strings.Contains(comment, "test") {
		return false
	}
	for _, contract := range []string{"fake", "override", "pin", "replace", "seam", "stub"} {
		if strings.Contains(comment, contract) {
			// A documented, unexported function or interface replacement is an
			// intentional dependency-injection boundary rather than ambient
			// application state. Network Doctor uses this contract for testable
			// OS integration:
			// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/cmd/netdoc-sim/clipboard.go#L25-L29
			return true
		}
	}
	return false
}

func globalDeclarationComment(declaration *ast.GenDecl, specification *ast.ValueSpec) string {
	var result strings.Builder
	for _, group := range []*ast.CommentGroup{declaration.Doc, specification.Doc, specification.Comment} {
		if group == nil {
			continue
		}
		result.WriteString(group.Text())
		result.WriteByte('\n')
	}
	return result.String()
}
