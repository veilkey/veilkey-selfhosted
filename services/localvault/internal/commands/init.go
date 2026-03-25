package commands

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"veilkey-localvault/internal/api"
	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/agentapi"
	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/tlsutil"
)

func RunInit() {
	isRoot := false
	forceInit := false
	tokenStr := ""
	centerURL := ""
	for i := 2; i < len(os.Args); i++ {
		switch {
		case os.Args[i] == "--root":
			isRoot = true
		case os.Args[i] == "--force":
			forceInit = true
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
			log.Fatal("Passwords are now auto-generated. The --password flag is no longer supported.")
		}
	}

	if !isRoot {
		fmt.Println("Usage: veilkey-localvault init --root [--force] [--token vk_reg_...] [--center https://vc.example.com]")
		fmt.Println("  --root      Initialize as HKM node")
		fmt.Println("  --force     Force re-initialization (WARNING: destroys existing data)")
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

	// Refuse to init if DB already exists (prevents accidental data loss)
	if err := checkInitDBExists(dbPath, forceInit); err != nil {
		log.Fatal(err)
	}
	if forceInit {
		_ = os.Remove(saltFile)
	}

	if _, err := os.Stat(saltFile); err == nil {
		if !forceInit {
			log.Fatal("Already initialized. Salt file exists: " + saltFile)
		}
		log.Printf("WARNING: --force specified, overwriting existing salt file at %s", saltFile)
		_ = os.Remove(saltFile)
	}

	// Auto-generate password for VC-managed unlock (no user prompt needed).
	// The password is stored on VaultCenter (encrypted with VC KEK) and fetched on startup.
	passwordBytes := make([]byte, 32)
	if _, err := cryptorand.Read(passwordBytes); err != nil {
		log.Fatalf("Failed to generate random password: %v", err)
	}
	password := hex.EncodeToString(passwordBytes)

	salt, err := crypto.GenerateSalt()
	if err != nil {
		log.Fatalf("Failed to generate salt: %v", err)
	}
	kek := crypto.DeriveKEK(password, salt)

	// Derive DB encryption key from KEK (not from salt)
	_ = os.Setenv("VEILKEY_DB_KEY", api.DeriveDBKeyFromKEK(kek))

	// Remove any existing DB (from setup mode or --force)
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-shm")
	_ = os.Remove(dbPath + "-wal")

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() { _ = database.Close() }()

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

	// Store vault_key file for bootstrap (auto-unlock before first VC registration).
	// This file is deleted after agent_secret is received from VC.
	vaultKeyFile := filepath.Join(dataDir, "vault_key")
	if err := os.WriteFile(vaultKeyFile, []byte(password), 0600); err != nil {
		log.Fatalf("Failed to save vault_key: %v", err)
	}

	fmt.Println("VeilKey agent initialized (VC-managed unlock).")
	fmt.Printf("  Node ID: %s\n", nodeID)
	fmt.Printf("  Salt:    %s\n", saltFile)
	fmt.Printf("  DB:      %s\n", dbPath)
	fmt.Println("")
	fmt.Println("  Password auto-generated. VC will manage unlock on startup.")
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
	url := strings.TrimRight(vcURL, "/") + agentapi.PathRegistrationTokenValidate + tokenID + "/validate"
	client := tlsutil.InitHTTPClientFromEnv()
	client.Timeout = cmdutil.ParseDurationEnv("VEILKEY_HTTP_TIMEOUT", 10*time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("cannot reach VaultCenter: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token rejected by VaultCenter (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// checkInitDBExists checks if the database file already exists.
// If force is false and the DB exists, it returns an error.
// If force is true and the DB exists, it removes the DB files and returns nil.
func checkInitDBExists(dbPath string, force bool) error {
	if _, err := os.Stat(dbPath); err != nil {
		return nil // DB does not exist, safe to proceed
	}
	if !force {
		return fmt.Errorf("ABORT: Database already exists at %s\n"+
			"  This would destroy all existing secrets.\n"+
			"  To force re-init, delete the file first:\n"+
			"    rm %s %s-shm %s-wal\n"+
			"  Or use --force flag.", dbPath, dbPath, dbPath, dbPath)
	}
	log.Printf("WARNING: --force specified, overwriting existing database at %s", dbPath)
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-shm")
	_ = os.Remove(dbPath + "-wal")
	return nil
}
