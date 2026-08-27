package docdeferinloop

import "os"

//gohawk:example flagged
func readAll(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
	return nil
}

//gohawk:example end

//gohawk:example ok
func readAllSafely(names []string) error {
	for _, name := range names {
		if err := readOne(name); err != nil {
			return err
		}
	}
	return nil
}

func readOne(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

//gohawk:example end
