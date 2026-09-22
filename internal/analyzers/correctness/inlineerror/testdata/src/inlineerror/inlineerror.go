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

func priorErrorPrecedence() (err error) {
	err = readConfig()
	if closeErr := readConfig(); err == nil {
		return closeErr
	}
	return err
}

func priorErrorPrecedenceNilFirst() (err error) {
	err = readConfig()
	if closeErr := readConfig(); nil == err {
		return closeErr
	}
	return err
}

func precedenceReturnsOther(prior, other error) error {
	if fresh := readConfig(); prior == nil { // want "condition checks prior instead of newly declared fresh"
		return fresh
	}
	return other
}

func precedenceWrongOperator(prior error) error {
	if fresh := readConfig(); prior != nil { // want "condition checks prior instead of newly declared fresh"
		return fresh
	}
	return prior
}

func precedenceInterveningStatement(prior error) error {
	if fresh := readConfig(); prior == nil { // want "condition checks prior instead of newly declared fresh"
		return fresh
	}
	prior = readConfig()
	return prior
}

func wrap(err error) error { return err }

func precedenceTransformsFresh(prior error) error {
	if fresh := readConfig(); prior == nil { // want "condition checks prior instead of newly declared fresh"
		return wrap(fresh)
	}
	return prior
}

func precedenceCompoundCondition(prior error, enabled bool) error {
	if fresh := readConfig(); prior == nil && enabled { // want "condition checks prior instead of newly declared fresh"
		return fresh
	}
	return prior
}

func precedenceWithElse(prior error) error {
	if fresh := readConfig(); prior == nil { // want "condition checks prior instead of newly declared fresh"
		return fresh
	} else {
		return prior
	}
}
