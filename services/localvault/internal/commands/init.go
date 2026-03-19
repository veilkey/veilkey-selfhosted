package commands

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
)

func RunInit() {
	isRoot := false
	tokenStr := ""
	centerURL := ""
	for i := 2; i < len(os.Args); i++ {
		switch {
		case os.Args[i] == "--root":
			isRoot = true
		case os.Args[i] == "--token" && i+1 < len(os.Args):
			i++
			tokenStr = os.Args[i]
		case strings.HasPrefix(os.Args[i], "--token="):
			tokenStr = strings.TrimPrefix(os.Args[i], "--token=")
		case os.Args[i] == "--center" && i+1 < len(os.Args):
			i++
			centerURL = os.Args[i]
		case strings.HasPrefix(os.Args[i], "--center="):
			centerURL = strings.TrimPrefix(os.Args[i], "--center=")
		case os.Args[i] == "--password":
			log.Fatal("--password flag is no longer supported (password exposed in ps/proc). Provide password via stdin or interactive prompt.")
		}
	}

	if !isRoot {
		fmt.Println("Usage: veilkey-localvault init --root [--token vk_reg_...] [--center https://vc.example.com]")
		fmt.Println("  --root      Initialize as HKM node")
		fmt.Println("  --token     Registration token from VaultCenter")
		fmt.Println("  --center    VaultCenter URL (alternative to token)")
		fmt.Println("  Password is read from stdin (pipe) or interactive TTY prompt.")
		os.Exit(1)
	}

	// Parse registration token if provided
	var tokenID, tokenLabel string
	if tokenStr != "" {
		var tokenURL string
		var err error
		tokenID, tokenURL, tokenLabel, err = decodeRegistrationToken(tokenStr)
		if err != nil {
			log.Fatalf("Invalid registration token: %v", err)
		}
		if tokenURL != "" {
			centerURL = tokenURL
		}
		if centerURL == "" {
			log.Fatal("VaultCenter URL is required. Provide via --center or include in token.")
		}
		fmt.Printf("  Token: %s (label: %s)\n", tokenID[:8]+"...", tokenLabel)
		fmt.Printf("  VaultCenter: %s\n", centerURL)

		if err := validateTokenRemote(centerURL, tokenID); err != nil {
			log.Fatalf("Token validation failed: %v", err)
		}
		fmt.Println("  Token validated successfully.")
	}

	dbPath := os.Getenv("VEILKEY_DB_PATH")
	if dbPath == "" {
		log.Fatal("VEILKEY_DB_PATH is required")
	}
	dataDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	saltFile := filepath.Join(dataDir, "salt")

	if _, err := os.Stat(saltFile); err == nil {
		log.Fatal("Already initialized. Salt file exists: " + saltFile)
	}

	password := cmdutil.ReadPassword("Enter KEK password: ")
	stat, _ := os.Stdin.Stat()
	isPiped := (stat.Mode() & os.ModeCharDevice) == 0
	if !isPiped {
		password2 := cmdutil.ReadPassword("Confirm KEK password: ")
		if password != password2 {
			log.Fatal("Passwords do not match.")
		}
	}
	if len(password) < 8 {
		log.Fatal("Password must be at least 8 characters.")
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		log.Fatalf("Failed to generate salt: %v", err)
	}
	kek := crypto.DeriveKEK(password, salt)

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	nodeID := crypto.GenerateUUID()
	dek, err := crypto.GenerateKey()
	if err != nil {
		log.Fatalf("Failed to generate DEK: %v", err)
	}

	encDEK, encNonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		log.Fatalf("Failed to encrypt DEK: %v", err)
	}

	info := &db.NodeInfo{
		NodeID:   nodeID,
		DEK:      encDEK,
		DEKNonce: encNonce,
		Version:  1,
	}
	if err := database.SaveNodeInfo(info); err != nil {
		log.Fatalf("Failed to save node info: %v", err)
	}

	// Save VC URL and registration token to DB config
	if centerURL != "" {
		normalized := strings.TrimRight(centerURL, "/")
		if err := database.SaveConfig(db.ConfigKeyVaultcenterURL, normalized); err != nil {
			log.Printf("Warning: failed to save vaultcenter URL: %v", err)
		} else {
			fmt.Printf("  VaultCenter URL saved: %s\n", normalized)
		}
	}
	if tokenID != "" {
		if err := database.SaveConfig("VEILKEY_REGISTRATION_TOKEN", tokenID); err != nil {
			log.Printf("Warning: failed to save registration token: %v", err)
		}
	}

	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		log.Fatalf("Failed to save salt: %v", err)
	}

	fmt.Println("VeilKey agent initialized.")
	fmt.Printf("  Node ID: %s\n", nodeID)
	fmt.Printf("  Salt:    %s\n", saltFile)
	fmt.Printf("  DB:      %s\n", dbPath)
	fmt.Println("")
	fmt.Println("  IMPORTANT: Remember your password. Lost password = unrecoverable data.")
}

func decodeRegistrationToken(token string) (tokenID, vcURL, label string, err error) {
	const prefix = "vk_reg_"
	if !strings.HasPrefix(token, prefix) {
		return "", "", "", fmt.Errorf("token must start with %s", prefix)
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, prefix))
	if err != nil {
		return "", "", "", fmt.Errorf("invalid base64 encoding: %w", err)
	}
	var payload struct {
		TokenID string `json:"t"`
		URL     string `json:"u"`
		Label   string `json:"l"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", "", "", fmt.Errorf("invalid token payload: %w", err)
	}
	if payload.TokenID == "" {
		return "", "", "", fmt.Errorf("token has no ID")
	}
	return payload.TokenID, payload.URL, payload.Label, nil
}

func validateTokenRemote(vcURL, tokenID string) error {
	url := strings.TrimRight(vcURL, "/") + "/api/registration-tokens/" + tokenID + "/validate"
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("cannot reach VaultCenter: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token rejected by VaultCenter (HTTP %d)", resp.StatusCode)
	}
	return nil
}
