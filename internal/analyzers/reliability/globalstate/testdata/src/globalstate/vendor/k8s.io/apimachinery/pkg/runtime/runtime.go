package runtime

type Scheme struct {
	types map[string]any
}

type SchemeBuilder []func(*Scheme) error
