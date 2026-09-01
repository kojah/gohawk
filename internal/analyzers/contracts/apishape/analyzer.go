// Package apishape implements the apishape gohawk analyzer.
package apishape

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/analysisutil"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	config := apiShapeConfig{
		maxParameters:       4,
		maxAdjacentSameType: 2,
	}
	analyzer := &analysis.Analyzer{
		Name: "apishape",
		Doc:  "checks exported API parameter and receiver shape",
	}
	analyzer.Flags.IntVar(&config.maxParameters, "max-parameters", 4, "maximum exported function parameters; 0 disables the check")
	analyzer.Flags.IntVar(&config.maxAdjacentSameType, "max-adjacent-same-type", 2, "maximum adjacent parameters of one type; 0 disables the check")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runAPIShape(pass, config)
	}
	return analyzer
}

type apiShapeConfig struct {
	maxParameters       int
	maxAdjacentSameType int
}

type receiverForms uint8

const (
	valueReceiver receiverForms = 1 << iota
	pointerReceiver
)

func runAPIShape(pass *analysis.Pass, config apiShapeConfig) (any, error) {
	receivers := map[string]receiverForms{}
	receiverPositions := map[string]token.Pos{}
	interfaces := apiShapeInterfaces(pass)
	callbacks := apiShapeCallbacks(pass)
	// Interface methods and values passed as callbacks have externally imposed
	// signatures. Excluding them before applying shape limits avoids suggesting
	// an API change that would break the contract supplying the declaration.
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.FuncDecl)
			if !ok {
				return true
			}
			recordReceiver(declaration, receivers, receiverPositions)
			if !declaration.Name.IsExported() {
				return false
			}
			if constrainedSignature(pass, declaration, interfaces, callbacks) {
				return false
			}
			parameters := analysisutil.ParameterTypes(pass, declaration.Type.Params)
			if config.maxParameters > 0 && len(parameters) > config.maxParameters {
				check.Reportf(
					pass,
					check.APIParameterCount,
					declaration.Name.Pos(),
					"exported API has %d parameters; use an Input or config struct",
					len(parameters),
				)
			}
			reportAdjacentParameters(pass, declaration.Name.Pos(), parameters, config)
			return false
		})
	}
	for name, forms := range receivers {
		if forms == valueReceiver|pointerReceiver {
			check.Reportf(pass, check.APIMixedReceivers, receiverPositions[name], "type %s mixes pointer and value receivers", name)
		}
	}
	return nil, nil
}

func constrainedSignature(pass *analysis.Pass, declaration *ast.FuncDecl, interfaces []*types.Interface, callbacks map[types.Object]bool) bool {
	if hasBlankParameter(declaration.Type.Params) || methodRequiredByInterface(pass, declaration, interfaces) {
		return true
	}
	return callbacks[pass.TypesInfo.Defs[declaration.Name]]
}

func apiShapeCallbacks(pass *analysis.Pass) map[types.Object]bool {
	result := map[types.Object]bool{}
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			functionType := pass.TypesInfo.TypeOf(call.Fun)
			if functionType == nil {
				return true
			}
			signature, signatureOK := functionType.Underlying().(*types.Signature)
			if !signatureOK {
				return true
			}
			for index, argument := range call.Args {
				if !functionParameterIsCallback(signature, index) {
					continue
				}
				identifier, identifierOK := analysisutil.Unparen(argument).(*ast.Ident)
				if !identifierOK {
					continue
				}
				if function, functionOK := pass.TypesInfo.Uses[identifier].(*types.Func); functionOK {
					// An actual function-value use is stronger evidence than a matching
					// shape alone: changing this declaration would break its callback
					// consumer. ccLoad's protocol adapter uses this contract:
					// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/protocol/builtin/cliproxy_adapter.go#L143
					result[function] = true
				}
			}
			return true
		})
	}
	return result
}

func hasBlankParameter(parameters *ast.FieldList) bool {
	if parameters == nil {
		return false
	}
	for _, field := range parameters.List {
		for _, name := range field.Names {
			if name.Name == "_" {
				return true
			}
		}
	}
	return false
}

func methodRequiredByInterface(pass *analysis.Pass, declaration *ast.FuncDecl, interfaces []*types.Interface) bool {
	if declaration.Recv == nil {
		return false
	}
	function, ok := pass.TypesInfo.Defs[declaration.Name].(*types.Func)
	if !ok {
		return false
	}
	receiver := function.Signature().Recv().Type()
	for _, iface := range interfaces {
		if apiShapeInterfaceMethod(iface, function.Name()) != nil && types.Implements(receiver, iface) {
			return true
		}
	}
	return false
}

func apiShapeInterfaceMethod(iface *types.Interface, name string) *types.Func {
	iface.Complete()
	for selection := range iface.Methods() {
		method := selection
		if method.Name() == name {
			return method
		}
	}
	return nil
}

func apiShapeInterfaces(pass *analysis.Pass) []*types.Interface {
	// Include anonymous interfaces appearing in expressions as well as named
	// declarations: either can impose a method signature on a concrete receiver.
	seen := map[*types.Interface]bool{}
	var result []*types.Interface
	add := func(value types.Type) {
		if value == nil {
			return
		}
		iface, ok := value.Underlying().(*types.Interface)
		if !ok || seen[iface] {
			return
		}
		seen[iface] = true
		result = append(result, iface)
	}
	for _, object := range pass.TypesInfo.Defs {
		if object != nil {
			add(object.Type())
		}
	}
	for expression := range pass.TypesInfo.Types {
		add(pass.TypesInfo.TypeOf(expression))
	}
	return result
}

func functionParameterIsCallback(signature *types.Signature, argument int) bool {
	parameters := signature.Params()
	if signature.Variadic() && argument >= parameters.Len()-1 {
		argument = parameters.Len() - 1
	}
	if argument < 0 || argument >= parameters.Len() {
		return false
	}
	parameter := parameters.At(argument).Type()
	if signature.Variadic() {
		if slice, ok := parameter.(*types.Slice); ok {
			parameter = slice.Elem()
		}
	}
	_, ok := parameter.Underlying().(*types.Signature)
	return ok
}

func recordReceiver(declaration *ast.FuncDecl, receivers map[string]receiverForms, positions map[string]token.Pos) {
	if declaration.Recv == nil || codecMethod(declaration.Name.Name) {
		return
	}
	name, pointer := receiverName(declaration.Recv.List[0].Type)
	if name == "" {
		return
	}
	if pointer {
		receivers[name] |= pointerReceiver
	} else {
		receivers[name] |= valueReceiver
	}
	positions[name] = declaration.Name.Pos()
}

func codecMethod(name string) bool {
	switch name {
	case "MarshalJSON", "UnmarshalJSON", "MarshalText", "UnmarshalText", "MarshalBinary", "UnmarshalBinary":
		return true
	default:
		return false
	}
}

func reportAdjacentParameters(pass *analysis.Pass, position token.Pos, parameters []types.Type, config apiShapeConfig) {
	for start := 0; start < len(parameters); {
		end := start + 1
		for end < len(parameters) && types.Identical(parameters[start], parameters[end]) {
			end++
		}
		if config.maxAdjacentSameType > 0 && end-start > config.maxAdjacentSameType {
			check.Reportf(
				pass,
				check.APIAdjacentSameType,
				position,
				"%d adjacent parameters share type %s; use an Input struct",
				end-start,
				types.TypeString(parameters[start], nil),
			)
		} else if end-start >= 2 && optionalScalar(parameters[start]) {
			check.Reportf(pass, check.APIAdjacentOptional, position, "adjacent optional scalar parameters are easy to swap; use an Input struct")
		}
		start = end
	}
}

func optionalScalar(value types.Type) bool {
	pointer, ok := value.(*types.Pointer)
	if !ok {
		return false
	}
	basic, ok := pointer.Elem().Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsBoolean|types.IsString|types.IsInteger|types.IsFloat) != 0
}

func receiverName(expression ast.Expr) (string, bool) {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name, false
	case *ast.StarExpr:
		name, _ := receiverName(typed.X)
		return name, true
	case *ast.IndexExpr:
		return receiverName(typed.X)
	case *ast.IndexListExpr:
		return receiverName(typed.X)
	default:
		return "", false
	}
}
