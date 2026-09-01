package determinism

import "slices"

func unstable(input map[string]string) []string {
	result := make([]string, 0, len(input))
	for key := range input { // want "map iteration reaches ordered output without sorting"
		result = append(result, key)
	}
	return result
}

func stable(input map[string]string) []string {
	result := make([]string, 0, len(input))
	for key := range input {
		result = append(result, key)
	}
	slices.Sort(result)
	return result
}

func sortValues(values []string) {
	slices.Sort(values)
}

func stableThroughHelper(input map[string]string) []string {
	result := make([]string, 0, len(input))
	for key := range input {
		result = append(result, key)
	}
	sortValues(result)
	return result
}

func conditionallySortValues(values []string, enabled bool) {
	if enabled {
		slices.Sort(values)
	}
}

func unstableThroughConditionalHelper(input map[string]string, sortResult bool) []string {
	result := make([]string, 0, len(input))
	for key := range input { // want "map iteration reaches ordered output without sorting"
		result = append(result, key)
	}
	conditionallySortValues(result, sortResult)
	return result
}

func firstValue(input map[string]string) string {
	for _, value := range input { // want "map iteration reaches ordered output without sorting"
		return value
	}
	return ""
}

func soleValue(input map[string]string) string {
	if len(input) != 1 {
		return ""
	}
	for _, value := range input {
		return value
	}
	return ""
}

func branchDoesNotProveSingleton(input map[string]string) string {
	if len(input) != 1 {
		goto selectValue
	}
selectValue:
	for _, value := range input { // want "map iteration reaches ordered output without sorting"
		return value
	}
	return ""
}

func membershipOnly(input map[string]string, needle string) bool {
	values := make([]string, 0, len(input))
	for _, value := range input {
		values = append(values, value)
	}
	return slices.Contains(values, needle)
}

func unrelatedMembershipWithOrderedResult(input map[string]string, needle string) string {
	found := false
	for key := range input {
		found = found || key == needle
	}
	if found {
		return "present"
	}
	return "absent"
}

func conditionallySorted(input map[string]string, early bool) []string {
	var result []string
	for key := range input { // want "map iteration reaches ordered output without sorting"
		result = append(result, key)
	}
	if early {
		return result
	}
	slices.Sort(result)
	return result
}

func commutative(input map[string]int) int {
	total := 0
	for _, value := range input {
		total += value
	}
	return total
}
