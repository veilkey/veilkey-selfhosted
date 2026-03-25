package commands

import (
	"strconv"
	"testing"
)

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

func TestKeyDerivationVersionIsNumeric(t *testing.T) {
	_, err := strconv.Atoi(CurrentKeyDerivationVersion)
	if err != nil {
		t.Errorf("CurrentKeyDerivationVersion must be a valid number, got %q: %v",
			CurrentKeyDerivationVersion, err)
	}
}

func TestKeyDerivationVersionNotEmpty(t *testing.T) {
	if len(CurrentKeyDerivationVersion) == 0 {
		t.Error("CurrentKeyDerivationVersion must not be empty")
	}
}

func TestProductVersionNotEmpty(t *testing.T) {
	v := productVersion()
	if len(v) == 0 {
		t.Error("productVersion() must return a non-empty string")
	}
	if v == "" {
		t.Error("productVersion() returned empty string")
	}
}
