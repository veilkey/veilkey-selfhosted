package commands

import "testing"

func TestCurrentKeyDerivationVersionNotEmpty(t *testing.T) {
	if CurrentKeyDerivationVersion == "" {
		t.Error("CurrentKeyDerivationVersion must not be empty")
	}
}

func TestProductVersionReturnsString(t *testing.T) {
	v := productVersion()
	if v == "" {
		t.Error("productVersion() must return a non-empty string")
	}
}
