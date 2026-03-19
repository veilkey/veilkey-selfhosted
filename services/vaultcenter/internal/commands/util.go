package commands

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/veilkey/veilkey-go-package/cmdutil"
)

func readPasswordFromFileEnv() string {
	return cmdutil.ReadPasswordFromFileEnv()
}

func readDataDirPassword(dataDir string) string {
	path := filepath.Join(dataDir, "password")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readPassword(prompt string) string {
	return cmdutil.ReadPassword(prompt)
}

func parseDurationEnv(key string, defaultVal time.Duration) time.Duration {
	return cmdutil.ParseDurationEnv(key, defaultVal)
}

func generateInitRef(length int) (string, error) {
	return cmdutil.GenerateHexRef(length)
}
