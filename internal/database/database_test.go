package database

import "testing"

func TestValidSSLMode(t *testing.T) {
	valid := []string{
		"disable", "allow", "prefer",
		"require", "verify-ca", "verify-full",
	}

	for _, mode := range valid {
		if !validSSLMode(mode) {
			t.Errorf("expected %q to be valid", mode)
		}
	}
}

func TestInvalidSSLMode(t *testing.T) {
	invalid := []string{"", "invalid", "VERIFY-FULL", "true"}

	for _, mode := range invalid {
		if validSSLMode(mode) {
			t.Errorf("expected %q to be invalid", mode)
		}
	}
}
