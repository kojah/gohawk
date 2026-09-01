package scheme

type Builder struct {
	registered []any
}

func (b *Builder) AddToScheme(any) error { return nil }
