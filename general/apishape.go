package general

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func apiShapeAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "apishape",
		Doc:  "checks exported API parameter and receiver shape",
		Run:  runAPIShape,
	}
}

func runAPIShape(pass *analysis.Pass) (any, error) {
	receivers := map[string]uint8{}
	receiverPositions := map[string]token.Pos{}
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
			parameters := parameterTypes(pass, declaration.Type.Params)
			if len(parameters) >= 5 {
				pass.Reportf(declaration.Name.Pos(), "exported API has %d parameters; use an Input or config struct", len(parameters))
			}
			reportAdjacentParameters(pass, declaration.Name.Pos(), parameters)
			return false
		})
	}
	for name, forms := range receivers {
		if forms == 3 {
			pass.Reportf(receiverPositions[name], "type %s mixes pointer and value receivers", name)
		}
	}
	return nil, nil
}

func recordReceiver(declaration *ast.FuncDecl, receivers map[string]uint8, positions map[string]token.Pos) {
	if declaration.Recv == nil || codecMethod(declaration.Name.Name) {
		return
	}
	name, pointer := receiverName(declaration.Recv.List[0].Type)
	if name == "" {
		return
	}
	if pointer {
		receivers[name] |= 2
	} else {
		receivers[name] |= 1
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

func parameterTypes(pass *analysis.Pass, fields *ast.FieldList) []types.Type {
	if fields == nil {
		return nil
	}
	var result []types.Type
	for _, field := range fields.List {
		count := max(1, len(field.Names))
		for range count {
			result = append(result, pass.TypesInfo.TypeOf(field.Type))
		}
	}
	return result
}

func reportAdjacentParameters(pass *analysis.Pass, position token.Pos, parameters []types.Type) {
	for start := 0; start < len(parameters); {
		end := start + 1
		for end < len(parameters) && types.Identical(parameters[start], parameters[end]) {
			end++
		}
		if end-start >= 3 {
			pass.Reportf(position, "%d adjacent parameters share type %s; use an Input struct", end-start, types.TypeString(parameters[start], nil))
		} else if end-start >= 2 && optionalScalar(parameters[start]) {
			pass.Reportf(position, "adjacent optional scalar parameters are easy to swap; use an Input struct")
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
