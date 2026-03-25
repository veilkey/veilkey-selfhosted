package commands

import (
	"os"
	"path/filepath"
	"strings"
)

// CurrentKeyDerivationVersion is incremented when DeriveDBKeyFromKEK logic changes.
// If this changes, existing DBs cannot be opened with the new binary without migration.
const CurrentKeyDerivationVersion = "1"

// productVersion returns the current product version string.
// It checks VEILKEY_PRODUCT_VERSION env, then VERSION file candidates.
func productVersion() string {
	if value := strings.TrimSpace(os.Getenv("VEILKEY_PRODUCT_VERSION")); value != "" {
		return value
	}
	if file := strings.TrimSpace(os.Getenv("VEILKEY_PRODUCT_VERSION_FILE")); file != "" {
		if data, err := os.ReadFile(file); err == nil {
			if value := strings.TrimSpace(string(data)); value != "" {
				return value
			}
		}
	}
	candidates := []string{
		"VERSION",
		filepath.Join("..", "VERSION"),
		filepath.Join("..", "..", "VERSION"),
		filepath.Join("..", "..", "..", "VERSION"),
	}
	for _, candidate := range candidates {
		if data, err := os.ReadFile(candidate); err == nil {
			if value := strings.TrimSpace(string(data)); value != "" {
				return value
			}
		}
	}
	return "unknown"
}
