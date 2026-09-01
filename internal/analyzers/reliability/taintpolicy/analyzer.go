// Package taintpolicy implements the taintpolicy gohawk analyzer.
package taintpolicy

import (
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/flagvalue"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

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
	analyzer.Flags.Var(
		flagvalue.NewCommaSeparatedChoice(&config.sinks, "filesystem", "process", "terminal", "log"),
		"sinks",
		"comma-separated sink families: filesystem,process,terminal,log",
	)
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

type taintSinkContract struct {
	symbol         syntax.Symbol
	kind           string
	display        string
	argumentStart  int
	argumentCount  int
	terminalWriter bool
}

var taintSinkContracts = []taintSinkContract{
	taintFunction("filesystem", "os", "Chdir", 0, 1),
	taintFunction("filesystem", "os", "Chmod", 0, 1),
	taintFunction("filesystem", "os", "Chown", 0, 1),
	taintFunction("filesystem", "os", "Create", 0, 1),
	taintFunction("filesystem", "os", "CreateTemp", 0, 1),
	taintFunction("filesystem", "os", "Lchown", 0, 1),
	taintFunction("filesystem", "os", "Lstat", 0, 1),
	taintFunction("filesystem", "os", "Mkdir", 0, 1),
	taintFunction("filesystem", "os", "MkdirAll", 0, 1),
	taintFunction("filesystem", "os", "Open", 0, 1),
	taintFunction("filesystem", "os", "OpenFile", 0, 1),
	taintFunction("filesystem", "os", "ReadFile", 0, 1),
	taintFunction("filesystem", "os", "Readlink", 0, 1),
	taintFunction("filesystem", "os", "Remove", 0, 1),
	taintFunction("filesystem", "os", "RemoveAll", 0, 1),
	taintFunction("filesystem", "os", "Rename", 0, 1),
	taintFunction("filesystem", "os", "Stat", 0, 1),
	taintFunction("filesystem", "os", "Symlink", 0, 1),
	taintFunction("filesystem", "os", "Truncate", 0, 1),
	taintFunction("filesystem", "os", "WriteFile", 0, 1),
	taintFunction("process", "os/exec", "Command", 0, 0),
	taintFunction("process", "os/exec", "CommandContext", 1, 0),
	terminalTaintFunction("fmt", "Fprint"),
	terminalTaintFunction("fmt", "Fprintf"),
	terminalTaintFunction("fmt", "Fprintln"),
	taintFunction("log", "log", "Print", 0, 0),
	taintFunction("log", "log", "Printf", 0, 0),
	taintFunction("log", "log", "Println", 0, 0),
	logTaintMethod("log", "Print"),
	logTaintMethod("log", "Printf"),
	logTaintMethod("log", "Println"),
	taintFunction("log", "log/slog", "DebugContext", 0, 0),
	taintFunction("log", "log/slog", "InfoContext", 0, 0),
	taintFunction("log", "log/slog", "WarnContext", 0, 0),
	taintFunction("log", "log/slog", "ErrorContext", 0, 0),
	taintFunction("log", "log/slog", "Log", 0, 0),
	logTaintMethod("log/slog", "DebugContext"),
	logTaintMethod("log/slog", "InfoContext"),
	logTaintMethod("log/slog", "WarnContext"),
	logTaintMethod("log/slog", "ErrorContext"),
	logTaintMethod("log/slog", "Log"),
}

func taintFunction(kind, packagePath, name string, argumentStart, argumentCount int) taintSinkContract {
	return taintSinkContract{
		symbol:        syntax.PackageFunction(packagePath, name),
		kind:          kind,
		display:       syntax.ShortPackageName(packagePath) + "." + name,
		argumentStart: argumentStart,
		argumentCount: argumentCount,
	}
}

func logTaintMethod(packagePath, name string) taintSinkContract {
	contract := taintFunction("log", packagePath, name, 0, 0)
	contract.symbol = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: packagePath, Receiver: "Logger", Name: name})
	return contract
}

func terminalTaintFunction(packagePath, name string) taintSinkContract {
	contract := taintFunction("terminal", packagePath, name, 1, 0)
	contract.terminalWriter = true
	return contract
}

func runTaintPolicy(pass *analysis.Pass, config taintPolicyConfig) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
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
	arguments := sourceCallArguments(common)
	for _, contract := range taintSinkContracts {
		if !sinks[contract.kind] || !ssaflow.CallMatchesSymbol(common, contract.symbol) || contract.argumentStart >= len(arguments) {
			continue
		}
		if contract.terminalWriter && !terminalWriter(arguments[0]) {
			continue
		}
		end := len(arguments)
		if contract.argumentCount > 0 {
			end = min(end, contract.argumentStart+contract.argumentCount)
		}
		return contract.kind, contract.display, arguments[contract.argumentStart:end]
	}
	return "", "", nil
}

func sourceCallArguments(common *ssa.CallCommon) []ssa.Value {
	arguments := common.Args
	if !common.IsInvoke() && common.Signature() != nil && common.Signature().Recv() != nil && len(arguments) > 0 {
		arguments = arguments[1:]
	}
	return arguments
}

func terminalWriter(value ssa.Value) bool {
	return terminalWriterSeen(value, map[ssa.Value]bool{})
}

func terminalWriterSeen(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if ssaflow.ValueMatchesSymbol(value, syntax.PackageVariable("os", "Stdout")) ||
		ssaflow.ValueMatchesSymbol(value, syntax.PackageVariable("os", "Stderr")) {
		return true
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
		if ssaflow.ValueMatchesSymbol(index.X, syntax.PackageVariable("os", "Args")) {
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
	return ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("os", "Getenv")) ||
		ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("os", "LookupEnv"))
}

func trustedSanitizer(common *ssa.CallCommon, configured map[string]bool) bool {
	if common == nil || common.StaticCallee() == nil || common.StaticCallee().Pkg == nil {
		return false
	}
	qualified := ssaflow.CallPackage(common) + "." + ssaflow.CallName(common)
	return configured[qualified]
}
