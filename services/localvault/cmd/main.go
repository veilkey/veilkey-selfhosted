package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"veilkey-localvault/internal/api"
	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/httputil"
	"veilkey-localvault/internal/db"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runInit()
			return
		case "cron":
			runCron()
			return
		case "rebind":
			runRebind()
			return
		}
	}

	runServer()
}

func runServer() {
	dbPath := os.Getenv("VEILKEY_DB_PATH")
	if dbPath == "" {
		log.Fatal("VEILKEY_DB_PATH is required")
	}
	dataDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	saltFile := filepath.Join(dataDir, "salt")

	if _, err := os.Stat(saltFile); os.IsNotExist(err) {
		// Setup mode: salt doesn't exist, serve wizard only
		runSetupServer(dbPath, dataDir)
		return
	}

	// Normal mode: salt exists, load full server
	server, addr, listenPort := mustLoadServer()

	// Heartbeat to vaultcenter using the same effective target as tracked-ref sync.
	hubURL := server.LogResolvedVaultcenterURL("startup")
	hostname, _ := os.Hostname()
	server.StartHeartbeat(hubURL, hostname, listenPort, 5*time.Minute)

	handler := server.SetupRoutes()
	tlsCert := os.Getenv("VEILKEY_TLS_CERT")
	tlsKey := os.Getenv("VEILKEY_TLS_KEY")
	if tlsCert != "" && tlsKey != "" {
		log.Printf("veilkey-localvault starting on %s (TLS)", addr)
		if err := http.ListenAndServeTLS(addr, tlsCert, tlsKey, handler); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	} else {
		log.Printf("veilkey-localvault starting on %s", addr)
		log.Println("WARNING: TLS not configured (set VEILKEY_TLS_CERT and VEILKEY_TLS_KEY to enable)")
		if err := http.ListenAndServe(addr, handler); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}
}

// initMu serialises concurrent POST /api/install/init requests.
var initMu sync.Mutex

func runSetupServer(dbPath, dataDir string) {
	addr := os.Getenv("VEILKEY_ADDR")
	if addr == "" {
		log.Fatal("VEILKEY_ADDR is required")
	}

	// Create minimal DB for config storage during setup.
	// Note: database is intentionally not closed here — ListenAndServe blocks
	// until process exit, at which point the OS reclaims all handles.
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a minimal server (no KEK, locked state)
	server := api.NewServer(database, nil, []string{})

	mux := http.NewServeMux()

	// Wizard UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		api.RenderInstallWizard(w)
	})
	mux.Handle("/assets/", http.FileServer(http.FS(api.InstallUIAssets())))

	// Health (always available)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"setup"}`))
	})

	// Install APIs
	mux.HandleFunc("GET /api/install/status", server.HandleInstallStatus)
	mux.HandleFunc("PATCH /api/install/vaultcenter-url", server.HandlePatchVaultcenterURL)
	mux.HandleFunc("POST /api/install/init", func(w http.ResponseWriter, r *http.Request) {
		handleInstallInit(w, r, database, dataDir, server)
	})

	log.Printf("veilkey-localvault setup mode on %s (waiting for initialization)", addr)

	tlsCert := os.Getenv("VEILKEY_TLS_CERT")
	tlsKey := os.Getenv("VEILKEY_TLS_KEY")
	if tlsCert != "" && tlsKey != "" {
		if err := http.ListenAndServeTLS(addr, tlsCert, tlsKey, api.LogMiddleware(mux)); err != nil {
			log.Fatalf("Setup server failed: %v", err)
		}
	} else {
		if err := http.ListenAndServe(addr, api.LogMiddleware(mux)); err != nil {
			log.Fatalf("Setup server failed: %v", err)
		}
	}
}

func handleInstallInit(w http.ResponseWriter, r *http.Request, database *db.DB, dataDir string, server *api.Server) {
	var req struct {
		Password       string `json:"password"`
		VaultcenterURL string `json:"vaultcenter_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	req.VaultcenterURL = strings.TrimSpace(req.VaultcenterURL)

	if len(req.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	initMu.Lock()
	defer initMu.Unlock()

	saltFile := filepath.Join(dataDir, "salt")
	if _, err := os.Stat(saltFile); err == nil {
		http.Error(w, "already initialized", http.StatusConflict)
		return
	}

	// Generate salt
	salt, err := crypto.GenerateSalt()
	if err != nil {
		log.Printf("install: failed to generate salt: %v", err)
		http.Error(w, "failed to generate salt", http.StatusInternalServerError)
		return
	}

	// Derive KEK from password + salt
	kek := crypto.DeriveKEK(req.Password, salt)

	// Generate DEK
	dek, err := crypto.GenerateKey()
	if err != nil {
		log.Printf("install: failed to generate DEK: %v", err)
		http.Error(w, "failed to generate DEK", http.StatusInternalServerError)
		return
	}

	// Encrypt DEK with KEK
	encDEK, encNonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		log.Printf("install: failed to encrypt DEK: %v", err)
		http.Error(w, "failed to encrypt DEK", http.StatusInternalServerError)
		return
	}

	// Generate node ID
	nodeID := crypto.GenerateUUID()

	// Save node info to DB
	info := &db.NodeInfo{
		NodeID:   nodeID,
		DEK:      encDEK,
		DEKNonce: encNonce,
		Version:  1,
	}
	if err := database.SaveNodeInfo(info); err != nil {
		log.Printf("install: failed to save node info: %v", err)
		http.Error(w, "failed to save node info", http.StatusInternalServerError)
		return
	}

	// Save vaultcenter URL to DB config if provided
	if req.VaultcenterURL != "" {
		normalized := strings.TrimRight(req.VaultcenterURL, "/")
		if err := database.SaveConfig("VEILKEY_VAULTCENTER_URL", normalized); err != nil {
			log.Printf("install: failed to save vaultcenter URL: %v", err)
			http.Error(w, "failed to save vaultcenter URL", http.StatusInternalServerError)
			return
		}
		log.Printf("install: vaultcenter URL saved: %s", normalized)
	}

	// Save password file BEFORE salt — salt existence signals "init complete"
	passwordFile := filepath.Join(dataDir, "password")
	if err := os.WriteFile(passwordFile, []byte(req.Password), 0600); err != nil {
		log.Printf("install: failed to write password file: %v", err)
		// non-fatal: server can still be unlocked manually
	} else {
		log.Printf("install: password file saved to %s", passwordFile)
	}

	// Write salt file (this is the trigger that switches from setup to normal mode)
	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		log.Printf("install: failed to write salt file: %v", err)
		http.Error(w, "failed to write salt file", http.StatusInternalServerError)
		return
	}

	log.Printf("install: initialization complete, node_id=%s", nodeID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "initialized",
		"node_id": nodeID,
		"message": "Initialization complete. The server will restart in normal mode.",
	})

	// Exit so systemd (or supervisor) restarts the process in normal mode.
	// Exit 0 = clean shutdown; Restart=always or Restart=on-success will pick it up.
	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Println("install: exiting for restart in normal mode")
		os.Exit(0)
	}()
}

func mustLoadServer() (*api.Server, string, int) {
	dbPath := os.Getenv("VEILKEY_DB_PATH")
	if dbPath == "" {
		log.Fatal("VEILKEY_DB_PATH is required")
	}
	dataDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	saltFile := filepath.Join(dataDir, "salt")

	salt, err := os.ReadFile(saltFile)
	if err != nil {
		log.Fatal("Salt file not found. Run with 'init --root' first.")
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	addr := os.Getenv("VEILKEY_ADDR")
	if addr == "" {
		log.Fatal("VEILKEY_ADDR is required")
	}

	var trustedIPs []string
	if v := os.Getenv("VEILKEY_TRUSTED_IPS"); v != "" {
		trustedIPs = strings.Split(v, ",")
		log.Printf("Trusted IPs: %v", trustedIPs)
	} else {
		log.Println("WARNING: VEILKEY_TRUSTED_IPS not set, sensitive endpoints are unrestricted")
	}

	info, err := database.GetNodeInfo()
	if err != nil {
		log.Fatal("Node info not found. Run 'init --root' first.")
	}

	vaultHash, vaultName, err := ensureVaultIdentity(database, info.NodeID)
	if err != nil {
		log.Fatalf("Failed to ensure vault identity: %v", err)
	}

	server := api.NewServer(database, nil, trustedIPs)
	server.SetSalt(salt)
	server.SetIdentity(&api.NodeIdentity{
		NodeID:    info.NodeID,
		Version:   info.Version,
		VaultHash: vaultHash,
		VaultName: vaultName,
	})
	log.Printf("VeilKey agent: node=%s version=%d vault=%s:%s", info.NodeID, info.Version, vaultName, vaultHash)

	if pw := readPasswordFromFileEnv(); pw != "" {
		kek := crypto.DeriveKEK(pw, salt)
		if _, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce); err != nil {
			log.Fatalf("Failed to unlock: invalid password")
		}
		server.Unlock(kek)
		log.Println("Server unlocked via VEILKEY_PASSWORD_FILE")
	} else if pw := readDataDirPassword(dataDir); pw != "" {
		kek := crypto.DeriveKEK(pw, salt)
		if _, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce); err != nil {
			log.Fatalf("Failed to unlock with data dir password file: invalid password")
		}
		server.Unlock(kek)
		log.Println("Server unlocked via data dir password file")
	} else if os.Getenv("VEILKEY_PASSWORD") != "" {
		log.Fatal("VEILKEY_PASSWORD env var is no longer supported (password exposed in process environment). Use VEILKEY_PASSWORD_FILE instead.")
	} else {
		log.Println("Server started in LOCKED mode. POST /api/unlock with password to unlock.")
	}

	listenPort := 0
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		p, err := strconv.Atoi(addr[idx+1:])
		if err != nil {
			log.Fatalf("VEILKEY_ADDR has invalid port: %s", addr)
		}
		listenPort = p
	} else {
		log.Fatalf("VEILKEY_ADDR has no port: %s", addr)
	}
	return server, addr, listenPort
}

func runCron() {
	action := "tick"
	if len(os.Args) > 2 {
		action = os.Args[2]
	}
	switch action {
	case "tick":
		server, _, listenPort := mustLoadServer()
		defer server.Close()
		if deleted, err := server.CleanupExpiredTestFunctions(time.Now()); err != nil {
			log.Fatalf("cron tick cleanup failed: %v", err)
		} else if deleted > 0 {
			log.Printf("cron tick deleted %d expired TEST functions", deleted)
		}
		hubURL := server.LogResolvedVaultcenterURL("cron")
		if hubURL == "" {
			log.Fatal("VEILKEY_VAULTCENTER_URL is required for cron tick")
		}
		globalEndpoint := httputil.JoinPath(hubURL, "/api/functions/global")
		if upserted, removed, err := server.SyncGlobalFunctions(globalEndpoint); err != nil {
			log.Fatalf("cron tick global function sync failed: %v", err)
		} else if upserted > 0 || removed > 0 {
			log.Printf("cron tick synced global functions: upserted=%d removed=%d", upserted, removed)
		}
		hostname, _ := os.Hostname()
		endpoint := httputil.JoinPath(hubURL, "/api/agents/heartbeat")
		if err := server.SendHeartbeatOnce(endpoint, hostname, listenPort); err != nil {
			if errors.Is(err, api.ErrRotationRequired) {
				if retryErr := server.SendHeartbeatOnce(endpoint, hostname, listenPort); retryErr != nil {
					log.Fatalf("cron tick failed after rotation update: %v", retryErr)
				}
				fmt.Println("rotation applied and heartbeat sent")
				return
			}
			log.Fatalf("cron tick failed: %v", err)
		}
		fmt.Println("heartbeat sent")
	default:
		fmt.Println("Usage: veilkey-localvault cron tick")
		os.Exit(1)
	}
}

func runRebind() {
	keyVersion := 0
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--key-version":
			if i+1 < len(os.Args) {
				parsed, err := strconv.Atoi(os.Args[i+1])
				if err != nil || parsed <= 0 {
					log.Fatal("rebind requires a positive --key-version")
				}
				keyVersion = parsed
				i++
			}
		}
	}
	if keyVersion <= 0 {
		fmt.Println("Usage: veilkey-localvault rebind --key-version <n>")
		os.Exit(1)
	}
	dbPath := os.Getenv("VEILKEY_DB_PATH")
	if dbPath == "" {
		log.Fatal("VEILKEY_DB_PATH is required")
	}
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	if err := database.UpdateNodeVersion(keyVersion); err != nil {
		log.Fatalf("Failed to update node version: %v", err)
	}
	fmt.Printf("rebind prepared with key_version=%d\n", keyVersion)
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

func runInit() {
	isRoot := false
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--root":
			isRoot = true
		case "--password":
			log.Fatal("--password flag is no longer supported (password exposed in ps/proc). Provide password via stdin or interactive prompt.")
		}
	}

	if !isRoot {
		fmt.Println("Usage: veilkey-localvault init --root")
		fmt.Println("  --root      Initialize as HKM node")
		fmt.Println("  Password is read from stdin (pipe) or interactive TTY prompt.")
		os.Exit(1)
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

	password := readPassword("Enter KEK password: ")
	stat, _ := os.Stdin.Stat()
	isPiped := (stat.Mode() & os.ModeCharDevice) == 0
	if !isPiped {
		password2 := readPassword("Confirm KEK password: ")
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

func readPasswordFromFileEnv() string {
	return cmdutil.ReadPasswordFromFileEnv()
}

func readPassword(prompt string) string {
	return cmdutil.ReadPassword(prompt)
}

func readDataDirPassword(dataDir string) string {
	path := filepath.Join(dataDir, "password")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
