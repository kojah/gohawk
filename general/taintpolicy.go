package general

import (
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func taintPolicyAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "taintpolicy",
		Doc:      "checks untrusted environment and argument data reaching sensitive sinks",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runTaintPolicy,
	}
}

func runTaintPolicy(pass *analysis.Pass) (any, error) {
	for _, function := range analysisutil.SourceSSAFunctions(pass) {
		// Test helpers deliberately echo and persist hostile fixture values. Their
		// process is isolated; production sinks remain policy-owned here.
		if strings.HasSuffix(pass.Fset.Position(function.Pos()).Filename, "_test.go") {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(*ssa.Call)
				if !ok {
					continue
				}
				kind, display, arguments := taintSink(call.Common())
				for _, argument := range arguments {
					if taintedValue(argument, map[ssa.Value]bool{}, map[ssa.Value]bool{}) {
						pass.Reportf(call.Pos(), "untrusted data reaches %s sink %s", kind, display)
						break
					}
				}
			}
		}
	}
	return nil, nil
}

func taintSink(common *ssa.CallCommon) (string, string, []ssa.Value) {
	if common == nil {
		return "", "", nil
	}
	packagePath, name := analysisutil.CallPackage(common), analysisutil.CallName(common)
	switch packagePath {
	case "os":
		if filesystemSink(name) && len(common.Args) > 0 {
			return "filesystem", "os." + name, common.Args[:1]
		}
	case "os/exec":
		if name == "Command" {
			return "process", "exec.Command", common.Args
		}
		if name == "CommandContext" && len(common.Args) > 1 {
			return "process", "exec.CommandContext", common.Args[1:]
		}
	case "fmt":
		if strings.HasPrefix(name, "Fprint") && len(common.Args) > 1 && terminalWriter(common.Args[0]) {
			return "terminal", "fmt." + name, common.Args[1:]
		}
	case "log", "log/slog":
		if strings.HasPrefix(name, "Print") || strings.HasSuffix(name, "Context") || name == "Log" {
			return "log", packagePath + "." + name, common.Args
		}
	}
	return "", "", nil
}

func filesystemSink(name string) bool {
	switch name {
	case "Chdir", "Chmod", "Chown", "Create", "CreateTemp", "Lchown", "Lstat", "Mkdir", "MkdirAll", "Open", "OpenFile", "ReadFile", "Readlink", "Remove", "RemoveAll", "Rename", "Stat", "Symlink", "Truncate", "WriteFile":
		return true
	default:
		return false
	}
}

func terminalWriter(value ssa.Value) bool {
	return terminalWriterSeen(value, map[ssa.Value]bool{})
}

func terminalWriterSeen(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	global, ok := value.(*ssa.Global)
	if ok {
		return global.Pkg != nil && global.Pkg.Pkg.Path() == "os" && (global.Name() == "Stdout" || global.Name() == "Stderr")
	}
	switch typed := value.(type) {
	case *ssa.ChangeInterface:
		return terminalWriterSeen(typed.X, seen)
	case *ssa.MakeInterface:
		return terminalWriterSeen(typed.X, seen)
	case *ssa.UnOp:
		return terminalWriterSeen(typed.X, seen)
	}
	return false
}

func taintedValue(value ssa.Value, seen, memorySeen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if call, ok := value.(*ssa.Call); ok {
		if trustedSanitizer(call.Common()) {
			return false
		}
		if taintSource(call.Common()) {
			return true
		}
	}
	if index, ok := value.(*ssa.Index); ok {
		if global, globalOK := index.X.(*ssa.Global); globalOK && global.Pkg != nil && global.Pkg.Pkg.Path() == "os" && global.Name() == "Args" {
			return true
		}
	}
	instruction, ok := value.(ssa.Instruction)
	if ok {
		var operands []*ssa.Value
		for _, operand := range instruction.Operands(operands) {
			if operand != nil && taintedValue(*operand, seen, memorySeen) {
				return true
			}
		}
	}
	return taintedStoredValue(value, seen, memorySeen)
}

func taintedStoredValue(address ssa.Value, seen, memorySeen map[ssa.Value]bool) bool {
	if address == nil || memorySeen[address] || address.Referrers() == nil {
		return false
	}
	memorySeen[address] = true
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if taintedValue(typed.Val, seen, memorySeen) {
				return true
			}
		case *ssa.FieldAddr:
			if taintedStoredValue(typed, seen, memorySeen) {
				return true
			}
		case *ssa.IndexAddr:
			if taintedStoredValue(typed, seen, memorySeen) {
				return true
			}
		}
	}
	return false
}

func taintSource(common *ssa.CallCommon) bool {
	return analysisutil.CallPackage(common) == "os" && (analysisutil.CallName(common) == "Getenv" || analysisutil.CallName(common) == "LookupEnv")
}

func trustedSanitizer(common *ssa.CallCommon) bool {
	if common == nil || common.StaticCallee() == nil || common.StaticCallee().Pkg == nil {
		return false
	}
	name := strings.ToLower(analysisutil.CallName(common))
	return strings.Contains(name, "validate") || strings.Contains(name, "sanitize") || strings.Contains(name, "escape") || strings.Contains(name, "confine")
}
