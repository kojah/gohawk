// Package taintpolicy implements the taintpolicy gohawk analyzer.
package taintpolicy

import (
	"strings"

	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/flagvalue"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	config := taintPolicyConfig{sinks: "filesystem,process,terminal,log"}
	analyzer := &analysis.Analyzer{
		Name:     "taintpolicy",
		Doc:      "checks untrusted environment and argument data reaching sensitive sinks",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
	}
	analyzer.Flags.Var(flagvalue.NewCommaSeparatedChoice(&config.sinks, "filesystem", "process", "terminal", "log"), "sinks", "comma-separated sink families: filesystem,process,terminal,log")
	analyzer.Flags.StringVar(&config.sanitizers, "sanitizers", "", "comma-separated fully-qualified sanitizer functions")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runTaintPolicy(pass, config)
	}
	return analyzer
}

type taintPolicyConfig struct {
	sinks      string
	sanitizers string
}

type taintPolicySettings struct {
	sinks      map[string]bool
	sanitizers map[string]bool
}

func runTaintPolicy(pass *analysis.Pass, config taintPolicyConfig) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	settings := taintPolicySettings{sinks: flagvalue.CommaSeparatedSet(config.sinks), sanitizers: flagvalue.CommaSeparatedSet(config.sanitizers)}
	for _, function := range functions {
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
				kind, display, arguments := taintSink(call.Common(), settings.sinks)
				for _, argument := range arguments {
					if taintedValue(argument, map[ssa.Value]bool{}, map[ssa.Value]bool{}, settings) {
						check.Reportf(pass, check.TaintUntrustedSink, call.Pos(), "untrusted data reaches %s sink %s", kind, display)
						break
					}
				}
			}
		}
	}
	return nil, nil
}

func taintSink(common *ssa.CallCommon, sinks map[string]bool) (string, string, []ssa.Value) {
	if common == nil {
		return "", "", nil
	}
	packagePath, name := ssautil.CallPackage(common), ssautil.CallName(common)
	switch packagePath {
	case "os":
		if sinks["filesystem"] && filesystemSink(name) && len(common.Args) > 0 {
			return "filesystem", "os." + name, common.Args[:1]
		}
	case "os/exec":
		if !sinks["process"] {
			break
		}
		if name == "Command" {
			return "process", "exec.Command", common.Args
		}
		if name == "CommandContext" && len(common.Args) > 1 {
			return "process", "exec.CommandContext", common.Args[1:]
		}
	case "fmt":
		if sinks["terminal"] && strings.HasPrefix(name, "Fprint") && len(common.Args) > 1 && terminalWriter(common.Args[0]) {
			return "terminal", "fmt." + name, common.Args[1:]
		}
	case "log", "log/slog":
		if sinks["log"] && (strings.HasPrefix(name, "Print") || strings.HasSuffix(name, "Context") || name == "Log") {
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

func taintedValue(value ssa.Value, seen, memorySeen map[ssa.Value]bool, settings taintPolicySettings) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if call, ok := value.(*ssa.Call); ok {
		if trustedSanitizer(call.Common(), settings.sanitizers) {
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
			if operand != nil && taintedValue(*operand, seen, memorySeen, settings) {
				return true
			}
		}
	}
	return taintedStoredValue(value, seen, memorySeen, settings)
}

func taintedStoredValue(address ssa.Value, seen, memorySeen map[ssa.Value]bool, settings taintPolicySettings) bool {
	if address == nil || memorySeen[address] || address.Referrers() == nil {
		return false
	}
	memorySeen[address] = true
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if taintedValue(typed.Val, seen, memorySeen, settings) {
				return true
			}
		case *ssa.FieldAddr:
			if taintedStoredValue(typed, seen, memorySeen, settings) {
				return true
			}
		case *ssa.IndexAddr:
			if taintedStoredValue(typed, seen, memorySeen, settings) {
				return true
			}
		}
	}
	return false
}

func taintSource(common *ssa.CallCommon) bool {
	return ssautil.CallPackage(common) == "os" && (ssautil.CallName(common) == "Getenv" || ssautil.CallName(common) == "LookupEnv")
}

func trustedSanitizer(common *ssa.CallCommon, configured map[string]bool) bool {
	if common == nil || common.StaticCallee() == nil || common.StaticCallee().Pkg == nil {
		return false
	}
	qualified := ssautil.CallPackage(common) + "." + ssautil.CallName(common)
	return configured[qualified]
}
