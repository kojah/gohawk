package wirepolicy

type Payload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

var payload = Payload{"Ada", 36} // want "persisted or wire struct literal must use field keys"

type RegressionEventRow struct {
	ID   string // want "serialized field ID requires an explicit json or toml tag"
	Kind string // want "serialized field Kind requires an explicit json or toml tag"
}
