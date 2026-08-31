package testpolicy

import "testing"

//gohawk:example flagged
func requireUser(t *testing.T, user *User) { // want "test helper accepting t must call t.Helper"
	if user == nil {
		t.Fatal("expected a user")
	}
}

//gohawk:example end

//gohawk:example ok
func requireUserSafely(t *testing.T, user *User) {
	t.Helper()
	if user == nil {
		t.Fatal("expected a user")
	}
}

//gohawk:example end
