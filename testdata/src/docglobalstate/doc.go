package docglobalstate

type User struct{}

//gohawk:example flagged
var users = map[string]User{} // want "mutable package state users"
//gohawk:example end

//gohawk:example ok
type Store struct {
	users map[string]User
}

func NewStore() *Store {
	return &Store{users: make(map[string]User)}
}

//gohawk:example end
