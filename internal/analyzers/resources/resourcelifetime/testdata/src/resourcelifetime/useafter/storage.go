package useafter

import "os"

type fileOwner struct {
	file  *os.File
	count int
}

func replaceOwner(*fileOwner)
func retainFile(*os.File)
func inspectFile(file *os.File) bool { return file != nil }

func replacedField(path string, fresh *os.File) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	owner := fileOwner{file: file}
	_ = owner.file.Close()
	owner.file = fresh
	_, err = owner.file.WriteString("fresh")
	return err
}

func opaqueField(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	owner := fileOwner{file: file}
	_ = owner.file.Close()
	replaceOwner(&owner)
	_, err = owner.file.WriteString("unknown")
	return err
}

func mixedSlot(path string, other *os.File, choose bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	owner := fileOwner{file: file}
	if choose {
		owner.file = other
	}
	_, err = owner.file.WriteString("unknown")
	return err
}

func escapedFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	retainFile(file)
	_ = file.Close()
	_, err = file.WriteString("unknown")
	return err
}

func asyncFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	go retainFile(file)
	_ = file.Close()
	_, err = file.WriteString("unknown")
	return err
}

func overwrittenObject(path string, fresh *os.File) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	*file = *fresh
	_, err = file.WriteString("replacement")
	return err
}

func fieldAfterClose(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	owner := fileOwner{file: file}
	_ = owner.file.Close()
	owner.count++                           // An independent field does not replace the file.
	_, err = owner.file.WriteString("late") // want "resource from os.Create is used after Close"
	return err
}

func savedField(path string, fresh *os.File) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	owner := fileOwner{file: file}
	saved := owner.file
	_ = owner.file.Close()
	owner.file = fresh
	_, err = saved.WriteString("late") // want "resource from os.Create is used after Close"
	return err
}

func constantElement(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	files := [2]*os.File{nil, file}
	_ = files[1].Close()
	_, err = files[1:][0].WriteString("late") // want "resource from os.Create is used after Close"
	return err
}

func agreedField(path string, branch bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	var owner fileOwner
	if branch {
		owner.file = file
	} else {
		owner.file = file
	}
	_ = owner.file.Close()
	_, err = owner.file.WriteString("late") // want "resource from os.Create is used after Close"
	return err
}

func readOnlyBetween(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	_ = file.Close()
	_ = inspectFile(file)
	_, err = file.WriteString("late") // want "resource from os.Create is used after Close"
	return err
}

func doubleCloseNotChecked(path string) {
	file, err := os.Create(path)
	if err != nil {
		return
	}
	_ = file.Close()
	_ = file.Close()
}

func differentElement(path string, other *os.File) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	files := [2]*os.File{file, other}
	_ = files[0].Close()
	_, err = files[1].WriteString("other")
	return err
}

func branchLocalUse(path string, closeEarly bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if closeEarly {
		_ = file.Close()
		_, err = file.WriteString("late") // want "resource from os.Create is used after Close"
		return err
	}
	return file.Close()
}
