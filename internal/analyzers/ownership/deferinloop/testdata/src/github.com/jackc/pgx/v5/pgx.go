package pgx

type Rows interface {
	Close()
	Err() error
	Next() bool
}

type Conn struct{}

func (*Conn) Query() (Rows, error) { return nil, nil }
