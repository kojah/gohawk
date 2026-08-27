package evalorder

func refresh(value *int) error {
	*value = 42
	return nil
}

//gohawk:example flagged
func load(value int) (int, error) {
	return value, refresh(&value) // want "later operand may mutate value after its earlier value was evaluated"
}

//gohawk:example end

//gohawk:example ok
func loadInOrder(value int) (int, error) {
	err := refresh(&value)
	return value, err
}

//gohawk:example end
