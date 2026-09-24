package syntax

import (
	"go/importer"
	"testing"
)

func TestSymbolMatchesPackageVariable(t *testing.T) {
	osPackage, err := importer.Default().Import("os")
	if err != nil {
		t.Fatal(err)
	}
	args := osPackage.Scope().Lookup("Args")
	if !PackageVariable("os", "Args").MatchesObject(args) {
		t.Error("os.Args did not match its package variable identity")
	}
	if PackageVariable("os", "Args").MatchesObject(osPackage.Scope().Lookup("Getenv")) {
		t.Error("os.Getenv matched a package variable identity")
	}
}
