package concurrencyfacts

import (
	"fmt"
	"go/types"
	"strconv"
	"strings"
)

// Fact dumps render a concurrency fact as its ordered effects, or as its path
// alternatives with the conditions that select each and what each returns,
// positions counting the receiver first as the facts do.

// DescribeFact renders the published fact for gohawk facts.
func (fact *publishedFact) DescribeFact(types.Object) []string {
	return fact.Value().describe()
}

func (fact Fact) describe() []string {
	if len(fact.Alternatives) == 0 {
		return describeSequence("", fact.Effects, fact.Workers, fact.CancellationInputs)
	}
	var lines []string
	for index, alternative := range fact.Alternatives {
		header := fmt.Sprintf("path %d", index+1)
		if conditions := describeConditions(alternative.Conditions); conditions != "" {
			header += " when " + conditions
		}
		if returned := describeReturned(alternative.Returned); returned != "" {
			header += ", returning " + returned
		}
		lines = append(lines, header+":")
		lines = append(lines, describeSequence("  ", alternative.Effects, alternative.Workers, alternative.CancellationInputs)...)
	}
	return lines
}

func describeSequence(indent string, effects []Effect, workers []WorkerEffect, cancellation []int) []string {
	var lines []string
	if len(effects) == 0 && len(workers) == 0 {
		lines = append(lines, indent+"no synchronization effects")
	}
	if len(effects) != 0 {
		lines = append(lines, indent+describeEffects(effects))
	}
	for _, worker := range workers {
		lines = append(lines, fmt.Sprintf("%slaunches a goroutine after %d effects: %s", indent, worker.Prefix, describeEffects(worker.Effects)))
	}
	if len(cancellation) != 0 {
		parts := make([]string, 0, len(cancellation))
		for _, index := range cancellation {
			parts = append(parts, "parameter "+strconv.Itoa(index))
		}
		lines = append(lines, indent+"needs the context or cancel function it is handed to be exact: "+strings.Join(parts, ", "))
	}
	return lines
}

func describeEffects(effects []Effect) string {
	parts := make([]string, 0, len(effects))
	for _, effect := range effects {
		var target strings.Builder
		target.WriteString("parameter " + strconv.Itoa(effect.Parameter))
		for _, field := range effect.Fields {
			target.WriteString(" field " + strconv.Itoa(field))
		}
		operation := effect.Kind.String()
		if effect.Kind == Invoke && effect.Method != "" {
			operation = "invoke " + effect.Method
		}
		parts = append(parts, operation+" "+target.String())
	}
	return strings.Join(parts, ", then ")
}

func describeConditions(conditions []FactCondition) string {
	parts := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		subject := "parameter " + strconv.Itoa(condition.Parameter)
		if condition.Parameter < 0 {
			subject = "its own test " + strconv.Itoa(condition.Internal)
		}
		var part string
		switch {
		case condition.Constant.Kind == FactConstantAbsent && condition.Holds:
			part = subject + " holds"
		case condition.Constant.Kind == FactConstantAbsent:
			part = subject + " does not hold"
		case condition.Holds:
			part = subject + " == " + condition.Constant.String()
		default:
			part = subject + " != " + condition.Constant.String()
		}
		if condition.Implied {
			part += " (implied)"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " and ")
}

func describeReturned(returned []FactReturned) string {
	parts := make([]string, 0, len(returned))
	for _, result := range returned {
		value := "non-nil"
		if result.Constant.Kind != FactConstantAbsent {
			value = result.Constant.String()
		}
		parts = append(parts, fmt.Sprintf("result %d %s", result.Index, value))
	}
	return strings.Join(parts, ", ")
}

// String renders the constant as Go source would write it.
func (constant FactConstant) String() string {
	switch constant.Kind {
	case FactConstantNil:
		return "nil"
	case FactConstantString:
		return strconv.Quote(constant.Exact)
	case FactConstantBool, FactConstantInt:
		return constant.Exact
	case FactConstantAbsent:
	}
	return "?"
}

// String names the synchronization event.
func (kind Kind) String() string {
	names := [...]string{
		Send: "send on", Receive: "receive from", Close: "close", GroupAdd: "WaitGroup.Add on", GroupDone: "WaitGroup.Done on",
		GroupWait: "WaitGroup.Wait on", Lock: "Lock", Unlock: "Unlock", CondWait: "Cond.Wait on", Cancel: "cancel",
		ReadLock: "RLock", ReadUnlock: "RUnlock", Invoke: "invoke",
	}
	if int(kind) < len(names) {
		return names[kind]
	}
	return "effect " + strconv.Itoa(int(kind))
}
