package ssaflow

import (
	"go/constant"
	"go/token"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Error-identity evidence defines which error-producing SSA forms preserve an
// existing identity. Arbitrary calls and tuple extracts remain opaque.

func collectErrorIdentitySources(value ssa.Value, sources, seen map[ssa.Value]bool) {
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		collectValueSources(inner, sources, seen)
		return
	}
	switch typed := value.(type) {
	case *ssa.Call:
		wrapped, ok := fmtErrorfWrappedValue(typed)
		if ok {
			collectValueSources(wrapped, sources, seen)
		}
		return
	case *ssa.Extract:
		// A tuple result is an identity produced by the call, not a value
		// derived from every ordinary argument passed to that call. This
		// distinction prevents successful data that happens to implement
		// error from joining independent failures:
		// https://github.com/esnet/gdg/blob/9476ad07b076f8cbda713342b7c34854ec4e5d4c/internal/adapter/grafana/api/alerting_contactpoints.go#L52-L96
		return
	case *ssa.Parameter, *ssa.FreeVar:
		return
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			collectValueSources(edge, sources, seen)
		}
		return
	case *ssa.UnOp:
		if typed.Op != token.MUL {
			return
		}
		collectStoredSources(typed.X, sources, seen, map[ssa.Value]bool{})
		return
	}
}

func fmtErrorfWrappedValue(call *ssa.Call) (ssa.Value, bool) { //nolint:ireturn // The wrapped operand may have any SSA form.
	common := call.Common()
	if !CallMatchesSymbol(common, syntax.PackageFunction("fmt", "Errorf")) || len(common.Args) != 2 {
		return nil, false
	}
	format, ok := common.Args[0].(*ssa.Const)
	if !ok || format.Value == nil || format.Value.Kind() != constant.String || !hasSolePlainWrapDirective(constant.StringVal(format.Value)) {
		return nil, false
	}
	return soleVariadicValue(common.Args[1])
}

func hasSolePlainWrapDirective(format string) bool {
	wraps := 0
	for index := 0; index < len(format); index++ {
		if format[index] != '%' {
			continue
		}
		index++
		if index >= len(format) {
			return false
		}
		switch format[index] {
		case '%':
			continue
		case 'w':
			wraps++
		default:
			return false
		}
	}
	return wraps == 1
}

func soleVariadicValue(value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // Variadic operands may have any SSA form.
	values, ok := storedAggregateValues(value, map[ssa.Value]bool{})
	if !ok || len(values) != 1 {
		return nil, false
	}
	return values[0], true
}

func storedAggregateValues(value ssa.Value, seen map[ssa.Value]bool) ([]ssa.Value, bool) {
	if value == nil || seen[value] {
		return nil, false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Slice:
		return storedAggregateValues(typed.X, seen)
	case *ssa.Alloc:
		return valuesStoredThroughAddresses(typed, seen)
	case *ssa.IndexAddr, *ssa.FieldAddr:
		return valuesStoredAtAddress(value, seen)
	default:
		return nil, false
	}
}

func valuesStoredThroughAddresses(value ssa.Value, seen map[ssa.Value]bool) ([]ssa.Value, bool) {
	if value.Referrers() == nil {
		return nil, false
	}
	var values []ssa.Value
	for _, reference := range *value.Referrers() {
		address, ok := reference.(ssa.Value)
		if !ok {
			continue
		}
		switch address.(type) {
		case *ssa.IndexAddr, *ssa.FieldAddr:
			stored, storedOK := storedAggregateValues(address, seen)
			if !storedOK {
				return nil, false
			}
			values = append(values, stored...)
		}
	}
	return values, len(values) > 0
}

func valuesStoredAtAddress(address ssa.Value, seen map[ssa.Value]bool) ([]ssa.Value, bool) {
	if address.Referrers() == nil {
		return nil, false
	}
	var values []ssa.Value
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if SameValue(typed.Addr, address) {
				values = append(values, typed.Val)
			}
		case *ssa.IndexAddr, *ssa.FieldAddr:
			nested, ok := reference.(ssa.Value)
			if !ok {
				return nil, false
			}
			stored, ok := storedAggregateValues(nested, seen)
			if !ok {
				return nil, false
			}
			values = append(values, stored...)
		}
	}
	return values, len(values) > 0
}

func ValueSources(value ssa.Value) map[ssa.Value]bool {
	sources := map[ssa.Value]bool{}
	collectValueSources(value, sources, map[ssa.Value]bool{})
	return sources
}

func ValuesShareErrorSource(left, right ssa.Value) bool {
	leftSources := ValueSources(left)
	for source := range ValueSources(right) {
		if leftSources[source] {
			return true
		}
	}
	return false
}

func collectValueSources(value ssa.Value, sources, seen map[ssa.Value]bool) {
	if value == nil || seen[value] {
		return
	}
	seen[value] = true
	if syntax.IsErrorType(value.Type()) {
		sources[value] = true
		collectErrorIdentitySources(value, sources, seen)
		return
	}
	switch typed := value.(type) {
	case *ssa.Call:
		if typed.Common().IsInvoke() {
			collectValueSources(typed.Common().Value, sources, seen)
		}
		for _, argument := range typed.Common().Args {
			collectValueSources(argument, sources, seen)
		}
		return
	case *ssa.Parameter, *ssa.FreeVar:
		return
	}
	instruction, ok := value.(ssa.Instruction)
	if !ok {
		return
	}
	var operands []*ssa.Value
	operands = instruction.Operands(operands)
	for _, operand := range operands {
		if operand != nil {
			collectValueSources(*operand, sources, seen)
		}
	}
	collectStoredSources(value, sources, seen, map[ssa.Value]bool{})
}

func collectStoredSources(address ssa.Value, sources, seen, memorySeen map[ssa.Value]bool) {
	// Variadic logging and wrapping calls lower arguments into temporary arrays.
	// Following stores recovers original error value instead of losing identity.
	if address == nil || memorySeen[address] || address.Referrers() == nil {
		return
	}
	memorySeen[address] = true
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			collectValueSources(typed.Val, sources, seen)
		case *ssa.FieldAddr:
			collectStoredSources(typed, sources, seen, memorySeen)
		case *ssa.IndexAddr:
			collectStoredSources(typed, sources, seen, memorySeen)
		}
	}
}
