package wirepolicyfix

type Payload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

var payload = Payload{"Ada", 36} // want "persisted or wire struct literal must use field keys"
