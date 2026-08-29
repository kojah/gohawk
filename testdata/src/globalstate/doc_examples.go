package globalstate

//gohawk:example flagged
type User struct {
	Name string
}

var users = map[string]User{} // want "mutable package state users"

func rememberUser(id string, user User) {
	users[id] = user
}

//gohawk:example end

//gohawk:example ok
type StoredUser struct {
	Name string
}

type Store struct {
	users map[string]StoredUser
}

func NewStore() *Store {
	return &Store{users: make(map[string]StoredUser)}
}

func (store *Store) Remember(id string, user StoredUser) {
	store.users[id] = user
}

//gohawk:example end
