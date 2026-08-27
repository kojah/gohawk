package docdeterminism

import "slices"

type User struct{}

//gohawk:example flagged
func names(users map[string]User) []string {
	var result []string
	for name := range users { // want "map iteration reaches ordered output without sorting"
		result = append(result, name)
	}
	return result
}

//gohawk:example end

//gohawk:example ok
func sortedNames(users map[string]User) []string {
	var result []string
	for name := range users {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}

//gohawk:example end
