package wirepolicyfix

type Payload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

var payload = Payload{"Ada", 36} // want "persisted or wire struct literal must use field keys"

var payloadWithComments = Payload{ // want "persisted or wire struct literal must use field keys"
	"Grace", // preserve the name comment
	42,      // preserve the age comment
}
