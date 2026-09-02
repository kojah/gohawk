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

func staleNestedCall(value int) {
	consume(value, wrap(replace(&value))) // want "later operand may mutate value after its earlier value was evaluated"
}

func staleImmediateClosure(value int) {
	consume(value, func() error {
		return replace(&value) // want "later operand may mutate value after its earlier value was evaluated"
	}())
}

func staleParenthesizedImmediateClosure(value int) {
	consume(value, (func() error {
		return replace(&value) // want "later operand may mutate value after its earlier value was evaluated"
	})())
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

type fieldState struct {
	result int
	other  int
}

func updateOther(state *fieldState) error {
	state.other = 42
	return nil
}

func disjointFieldUpdate(state fieldState) (int, error) {
	return state.result, updateOther(&state)
}

func updateResult(state *fieldState) error {
	state.result = 42
	return nil
}

func staleFieldUpdate(state fieldState) (int, error) {
	return state.result, updateResult(&state) // want "later operand may mutate state after its earlier value was evaluated"
}

func consume(int, error) {}

func wrap(err error) error { return err }

type installOptions struct {
	Name string
}

type sourceOperations struct {
	git func() error
	oci func() error
}

func updateInstallOptions(options *installOptions) error {
	options.Name = "resolved"
	return nil
}

func dispatchSource(string, sourceOperations) {}

func delayedSourceCallbacks(options installOptions) {
	dispatchSource(options.Name, sourceOperations{
		git: func() error {
			return updateInstallOptions(&options)
		},
		oci: func() error {
			return updateInstallOptions(&options)
		},
	})
}

func consumeCallback(func(), error) {}

func delayedEarlierRead(value int) {
	consumeCallback(func() {
		_ = value
	}, replace(&value))
}
