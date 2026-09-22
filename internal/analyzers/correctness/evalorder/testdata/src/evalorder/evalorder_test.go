package evalorder

func staleTestOnly(value int) (int, error) {
	return value, replace(&value) // want "later operand may mutate value after its earlier value was evaluated"
}

func orderedTestOnly(value int) (int, error) {
	err := replace(&value)
	return value, err
}
