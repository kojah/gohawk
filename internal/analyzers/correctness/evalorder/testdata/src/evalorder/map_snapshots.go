package evalorder

import "encoding/json"

// A freshly initialized map's entries are shared by an earlier map snapshot.
// Unknown JSON may still reset its header with null; that coverage loss is
// intentional, rather than treating every decode as a proven header change.
func decodeSharedMap(data []byte) (map[string]int, error) {
	value := map[string]int{}
	return value, json.Unmarshal(data, &value)
}

func decodeAllocatedMap(data []byte) (map[string]int, error) {
	value := make(map[string]int)
	return value, json.Unmarshal(data, &value)
}

func decodeMapEntry(data []byte) (int, error) {
	value := map[string]int{}
	return value["key"], json.Unmarshal(data, &value) // want "later operand may mutate value after its earlier value was evaluated"
}

func decodeNilMap(data []byte) (map[string]int, error) {
	var value map[string]int
	return value, json.Unmarshal(data, &value) // want "later operand may mutate value after its earlier value was evaluated"
}

func decodeReplacedMap(data []byte) (map[string]int, error) {
	value := map[string]int{}
	value = nil
	return value, json.Unmarshal(data, &value) // want "later operand may mutate value after its earlier value was evaluated"
}

func replaceMap(value *map[string]int) error {
	*value = map[string]int{"key": 1}
	return nil
}

func replaceMapSnapshot() (map[string]int, error) {
	value := map[string]int{}
	return value, replaceMap(&value) // want "later operand may mutate value after its earlier value was evaluated"
}

type decodingMap map[string]int

func (value *decodingMap) UnmarshalJSON([]byte) error {
	*value = decodingMap{"key": 1}
	return nil
}

func decodeCustomMap(data []byte) (decodingMap, error) {
	value := decodingMap{}
	return value, json.Unmarshal(data, &value) // want "later operand may mutate value after its earlier value was evaluated"
}

func decodeEscapedMap(data []byte) (map[string]int, error) {
	value := map[string]int{}
	_ = replaceMap(&value)
	return value, json.Unmarshal(data, &value) // want "later operand may mutate value after its earlier value was evaluated"
}
