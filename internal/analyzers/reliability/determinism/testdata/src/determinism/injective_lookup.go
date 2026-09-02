package determinism

import "fmt"

type lookupCode int

const (
	lookupFirst lookupCode = iota + 1
	lookupSecond
	lookupThird
)

func directInjectiveLookup(target lookupCode) string {
	for key, value := range map[string]lookupCode{"first": lookupFirst, "second": lookupSecond} {
		if value == target {
			return key
		}
	}
	return ""
}

func directInjectiveLookupReversed(target lookupCode) string {
	for key, value := range map[string]lookupCode{"first": lookupFirst, "second": lookupSecond} {
		if target == value {
			return key
		}
	}
	return ""
}

func literalLookupMap() map[string]lookupCode {
	return map[string]lookupCode{"first": lookupFirst, "second": lookupSecond}
}

func helperInjectiveLookup(target lookupCode) (string, error) {
	for key, value := range literalLookupMap() {
		if value == target {
			return key, nil
		}
	}
	return "", nil
}

func assignedLookupMap() map[string]lookupCode {
	mapping := make(map[string]lookupCode)
	mapping["first"] = lookupFirst
	mapping["second"] = lookupSecond
	mapping["third"] = lookupThird
	return mapping
}

func helperAccumulatorInjectiveLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping {
		if value == target {
			result = key
		}
	}
	return result
}

func localLiteralInjectiveLookup(target lookupCode) string {
	result := ""
	mapping := map[string]lookupCode{"first": lookupFirst, "second": lookupSecond}
	for key, value := range mapping {
		if value == target {
			result = key
		}
	}
	return result
}

func dynamicMapValueLookup(target, dynamic lookupCode) string {
	for key, value := range map[string]lookupCode{"first": lookupFirst, "dynamic": dynamic} { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func dynamicMapKeyLookup(target lookupCode, dynamic string) string {
	for key, value := range map[string]lookupCode{dynamic: lookupFirst, "second": lookupSecond} { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func duplicateMapValueLookup(target lookupCode) string {
	for key, value := range map[string]lookupCode{"first": lookupFirst, "duplicate": lookupFirst} { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func parameterMapValueLookup(mapping map[string]lookupCode, target lookupCode) string {
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

var globalLookupMap = map[string]lookupCode{"first": lookupFirst, "second": lookupSecond}

func globalMapValueLookup(target lookupCode) string {
	for key, value := range globalLookupMap { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func sharedLookupMap() map[string]lookupCode {
	return globalLookupMap
}

func sharedHelperMapValueLookup(target lookupCode) string {
	for key, value := range sharedLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func parameterizedLookupMap(value lookupCode) map[string]lookupCode {
	return map[string]lookupCode{"first": value, "second": lookupSecond}
}

func parameterizedHelperMapValueLookup(target lookupCode) string {
	for key, value := range parameterizedLookupMap(lookupFirst) { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func ExportedLookupMap() map[string]lookupCode {
	return map[string]lookupCode{"first": lookupFirst, "second": lookupSecond}
}

func exportedHelperMapValueLookup(target lookupCode) string {
	for key, value := range ExportedLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func variadicLookupMap(...lookupCode) map[string]lookupCode {
	return map[string]lookupCode{"first": lookupFirst, "second": lookupSecond}
}

func variadicHelperMapValueLookup(target lookupCode) string {
	for key, value := range variadicLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func mutatedSourceMapValueLookup(target lookupCode) string {
	mapping := assignedLookupMap()
	mapping["another"] = lookupThird
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		if value == target {
			return key
		}
	}
	return ""
}

func consumeLookupMap(map[string]lookupCode) {}

func escapedMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		consumeLookupMap(mapping)
		if value == target {
			result = key
		}
	}
	return result
}

func calledInsideMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		fmt.Print("")
		if value == target {
			result = key
		}
	}
	return result
}

func compoundMapValueLookup(target lookupCode, enabled bool) string {
	for key, value := range literalLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value == target && enabled {
			return key
		}
	}
	return ""
}

func multipleAssignmentMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		if value == target {
			result = key
			result += key
		}
	}
	return result
}

func appendedMapValueLookup(target lookupCode) []string {
	var result []string
	for key, value := range literalLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value == target {
			result = append(result, key)
		}
	}
	return result
}

func loggedMapValueLookup(target lookupCode) string {
	for key, value := range literalLookupMap() { // want "map iteration reaches ordered output without sorting"
		fmt.Print(key)
		if value == target {
			return key
		}
	}
	return ""
}

func unconditionallyOverwrittenMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		if value == target {
			result = key
		}
		result = key
	}
	return result
}

func extraRangeVariableUseMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		fmt.Print(value)
		if value == target {
			result = key
		}
	}
	return result
}

func nonEqualityMapValueLookup(target lookupCode) string {
	for key, value := range literalLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value != target {
			return key
		}
	}
	return ""
}

func derivedTargetMapValueLookup(target lookupCode) string {
	for key, value := range literalLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value == target+0 {
			return key
		}
	}
	return ""
}

func breakMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		if value == target {
			result = key
		}
		break
	}
	return result
}

func continueMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		if value == target {
			result = key
		}
		continue
	}
	return result
}

func deferredMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		defer func() {}()
		if value == target {
			result = key
		}
	}
	return result
}

func goroutineMapValueLookup(target lookupCode) string {
	result := ""
	mapping := assignedLookupMap()
	for key, value := range mapping { // want "map iteration reaches ordered output without sorting"
		go func() {}()
		if value == target {
			result = key
		}
	}
	return result
}

var globalLookupTarget = lookupFirst

func globalTargetMapValueLookup() string {
	for key, value := range literalLookupMap() { // want "map iteration reaches ordered output without sorting"
		if value == globalLookupTarget {
			return key
		}
	}
	return ""
}

func targetAccumulatorMapValueLookup(target string) string {
	for key, value := range map[string]string{"first": "one", "second": "two"} { // want "map iteration reaches ordered output without sorting"
		if value == target {
			target = key
		}
	}
	return target
}
