package apishape

//gohawk:example flagged Too many parameters
func CreateUser(name string, age int, active bool, score float64, role byte) error { // want "exported API has 5 parameters"
	return nil
}

//gohawk:example end

//gohawk:example flagged Adjacent optional parameters
func FindUser(firstName, lastName *string) error { // want "adjacent optional scalar parameters are easy to swap"
	return nil
}

//gohawk:example end

//gohawk:example ok
type CreateUserInput struct {
	Name, Email, City, Country, Role string
}

func CreateUserWithInput(input CreateUserInput) error {
	return nil
}

//gohawk:example end
