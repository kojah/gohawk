package inlineerror

func readConfig() error { return nil }

//gohawk:example flagged
func mismatched(previous error) error {
	if err := readConfig(); previous != nil { // want "condition checks previous instead of newly declared err"
		return err
	}
	return nil
}

//gohawk:example end

//gohawk:example ok
func matched(previous error) error {
	if err := readConfig(); err != nil {
		return err
	}
	return previous
}

//gohawk:example end
