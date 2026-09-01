package wirepolicy

type Payload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

var payload = Payload{"Ada", 36} // want "persisted or wire struct literal must use field keys"

type helpRow struct {
	command     string
	description string
}

var help = helpRow{"serve", "start the server"}

type Row struct {
	Name   string
	Status string
}

var uiRow = Row{"worker", "ready"}

type TaggedStorageRow struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

var taggedEvent = TaggedStorageRow{"42", "created"} // want "persisted or wire struct literal must use field keys"

type RegressionEventEnvelope struct {
	ID   string // want "serialized field ID requires an explicit json or toml tag"
	Kind string // want "serialized field Kind requires an explicit json or toml tag"
}
