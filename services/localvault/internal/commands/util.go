package commands

import (
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/term"

	"veilkey-localvault/internal/db"
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

func ensureVaultIdentity(database *db.DB, nodeID string) (string, string, error) {
	vaultHash := ""
	if cfg, err := database.GetConfig("VAULT_HASH"); err == nil {
		vaultHash = strings.TrimSpace(cfg.Value)
	}
	if vaultHash == "" {
		vaultHash = defaultVaultHash(nodeID)
		if err := database.SaveConfig("VAULT_HASH", vaultHash); err != nil {
			return "", "", err
		}
	}

	vaultName := ""
	if cfg, err := database.GetConfig("VAULT_NAME"); err == nil {
		vaultName = strings.TrimSpace(cfg.Value)
	}
	if vaultName == "" {
		vaultName = strings.TrimSpace(os.Getenv("VEILKEY_VAULT_NAME"))
		if vaultName == "" {
			vaultName, _ = os.Hostname()
		}
		if vaultName == "" {
			vaultName = "localvault"
		}
		if err := database.SaveConfig("VAULT_NAME", vaultName); err != nil {
			return "", "", err
		}
	}

	return vaultHash, vaultName, nil
}

func defaultVaultHash(nodeID string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(nodeID)), "-", "")
	if len(normalized) >= 8 {
		return normalized[:8]
	}
	if normalized != "" {
		return normalized
	}
	return "unknown"
}
