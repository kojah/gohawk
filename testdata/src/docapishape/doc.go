package docapishape

//gohawk:example flagged
func CreateUser(name, email, city, country, role string) error { // want "exported API has 5 parameters" "5 adjacent parameters share type string"
	return nil
}

//gohawk:example end

//gohawk:example ok
type CreateUserInput struct {
	Name, Email, City, Country, Role string
}

func CreateUserWithInput(input CreateUserInput) error { return nil }

//gohawk:example end
