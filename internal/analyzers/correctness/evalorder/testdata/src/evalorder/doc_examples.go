package evalorder

//gohawk:example flagged
func refresh(value *int) error {
	*value = 42
	return nil
}

func load(value int) (int, error) {
	return value, refresh(&value) // want "later operand may mutate value after its earlier value was evaluated"
}

//gohawk:example end

//gohawk:example ok
func refreshSafely(value *int) error {
	*value = 42
	return nil
}

func loadInOrder(value int) (int, error) {
	err := refreshSafely(&value)
	return value, err
}

//gohawk:example end
