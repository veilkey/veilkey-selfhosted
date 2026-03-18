package commands

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

func readPasswordFromFileEnv() string {
	path := os.Getenv("VEILKEY_PASSWORD_FILE")
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read VEILKEY_PASSWORD_FILE (%s): %v", path, err)
	}
	pw := strings.TrimSpace(string(data))
	if pw == "" {
		log.Fatalf("VEILKEY_PASSWORD_FILE (%s) is empty", path)
	}
	return pw
}

func readPassword(prompt string) string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		var s string
		fmt.Fscan(os.Stdin, &s)
		return strings.TrimSpace(s)
	}

	tty, err := os.Open("/dev/tty")
	if err != nil {
		log.Fatalf("Failed to open TTY: %v", err)
	}
	defer tty.Close()

	fmt.Fprint(tty, prompt)
	data, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(tty)
	if err != nil {
		log.Fatalf("Failed to read password: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func parseDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("warning: invalid duration %s=%q, using default %s", key, v, defaultVal)
	}
	return defaultVal
}

func generateInitRef(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
