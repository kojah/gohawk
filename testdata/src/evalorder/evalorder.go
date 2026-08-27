package evalorder

import "encoding/json"

func replace(target *int) error {
	*target = 42
	return nil
}

func staleReturn(value int) (int, error) {
	return value, replace(&value) // want "later operand may mutate value after its earlier value was evaluated"
}

func staleCall(value int) {
	consume(value, replace(&value)) // want "later operand may mutate value after its earlier value was evaluated"
}

func staleDecode(value map[string]int, data []byte) (map[string]int, error) {
	return value, json.Unmarshal(data, &value) // want "later operand may mutate value after its earlier value was evaluated"
}

func ordered(value int) (int, error) {
	err := replace(&value)
	return value, err
}

func readOnly(target *int) error {
	_ = *target
	return nil
}

func readOnlyLater(value int) (int, error) {
	return value, readOnly(&value)
}

func consume(int, error) {}
