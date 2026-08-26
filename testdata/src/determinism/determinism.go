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

func commutative(input map[string]int) int {
	total := 0
	for _, value := range input {
		total += value
	}
	return total
}
