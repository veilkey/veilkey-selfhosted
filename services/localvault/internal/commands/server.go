package commands

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/veilkey/veilkey-go-package/httputil"

	"veilkey-localvault/internal/api"
	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
)

var initMu sync.Mutex

func RunServer() {
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
		runSetupServer(dbPath, dataDir)
		return
	}

	server, addr, listenPort := mustLoadServer()
	hubURL := server.LogResolvedVaultcenterURL("startup")

	// Auto-unlock: try vault_key file first (bootstrap), then VC-managed unlock
	autoUnlock(server, hubURL)

	// CometBFT chain full node deferred — DB required for chain store adapter.
	// Chain will not start until after unlock when DB is available.
	if chainHome := os.Getenv("VEILKEY_CHAIN_HOME"); chainHome != "" {
		// Fetch genesis from vaultcenter if not already present
		genesisFile := filepath.Join(chainHome, "config", "genesis.json")
		if _, err := os.Stat(genesisFile); os.IsNotExist(err) {
			if hubURL != "" {
				fetchChainGenesis(hubURL, chainHome)
			} else {
				log.Println("Chain: no vaultcenter URL, skipping genesis fetch")
			}
		}
		log.Printf("Chain home=%s (will start after unlock when DB is available)", chainHome)
	} else {
		log.Println("Chain disabled (VEILKEY_CHAIN_HOME not set)")
	}
	hostname, _ := os.Hostname()
	server.StartHeartbeat(hubURL, hostname, listenPort, cmdutil.ParseDurationEnv("VEILKEY_HEARTBEAT_INTERVAL", 5*time.Minute))

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

func runSetupServer(dbPath, dataDir string) {
	addr := os.Getenv("VEILKEY_ADDR")
	if addr == "" {
		log.Fatal("VEILKEY_ADDR is required")
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	server := api.NewServer(database, nil, []string{})

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		api.RenderInstallWizard(w)
	})
	mux.Handle("/assets/", http.FileServer(http.FS(api.InstallUIAssets())))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", httputil.ContentTypeJSON)
		w.Write([]byte(`{"status":"setup"}`))
	})

	mux.HandleFunc("GET /api/install/status", server.HandleInstallStatus)
	mux.HandleFunc("PATCH /api/install/vaultcenter-url", server.HandlePatchVaultcenterURL)
	mux.HandleFunc("POST /api/install/init", func(w http.ResponseWriter, r *http.Request) {
		handleInstallInit(w, r, database, dbPath, dataDir)
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

func handleInstallInit(w http.ResponseWriter, r *http.Request, database *db.DB, dbPath, dataDir string) {
	initMu.Lock()
	defer initMu.Unlock()

	var req struct {
		VaultcenterURL string `json:"vaultcenter_url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.VaultcenterURL = strings.TrimSpace(req.VaultcenterURL)

	saltFile := filepath.Join(dataDir, "salt")
	if _, err := os.Stat(saltFile); err == nil {
		http.Error(w, "already initialized", http.StatusConflict)
		return
	}

	// Auto-generate password for VC-managed unlock
	passwordBytes := make([]byte, 32)
	if _, err := cryptorand.Read(passwordBytes); err != nil {
		log.Printf("install: failed to generate random password: %v", err)
		http.Error(w, "failed to generate random password", http.StatusInternalServerError)
		return
	}
	password := fmt.Sprintf("%x", passwordBytes)

	salt, err := crypto.GenerateSalt()
	if err != nil {
		log.Printf("install: failed to generate salt: %v", err)
		http.Error(w, "failed to generate salt", http.StatusInternalServerError)
		return
	}

	kek := crypto.DeriveKEK(password, salt)

	// Delete the unencrypted setup DB and create a new encrypted one
	_ = database.Close()
	_ = os.Remove(dbPath)
	_ = os.Setenv("VEILKEY_DB_KEY", api.DeriveDBKeyFromKEK(kek))
	database, dbErr := db.New(dbPath)
	if dbErr != nil {
		log.Printf("install: failed to create encrypted database: %v", dbErr)
		http.Error(w, "failed to initialize encrypted database", http.StatusInternalServerError)
		return
	}

	dek, err := crypto.GenerateKey()
	if err != nil {
		log.Printf("install: failed to generate DEK: %v", err)
		http.Error(w, "failed to generate DEK", http.StatusInternalServerError)
		return
	}

	encDEK, encNonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		log.Printf("install: failed to encrypt DEK: %v", err)
		http.Error(w, "failed to encrypt DEK", http.StatusInternalServerError)
		return
	}

	nodeID := crypto.GenerateUUID()

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

	if req.VaultcenterURL != "" {
		normalized := strings.TrimRight(req.VaultcenterURL, "/")
		if err := database.SaveConfig(db.ConfigKeyVaultcenterURL, normalized); err != nil {
			log.Printf("install: failed to save vaultcenter URL: %v", err)
			http.Error(w, "failed to save vaultcenter URL", http.StatusInternalServerError)
			return
		}
		log.Printf("install: vaultcenter URL saved: %s", normalized)
	}

	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		log.Printf("install: failed to write salt file: %v", err)
		// Clean up encrypted DB to avoid unrecoverable state (DB encrypted but no salt)
		_ = database.Close()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-shm")
		_ = os.Remove(dbPath + "-wal")
		http.Error(w, "failed to write salt file", http.StatusInternalServerError)
		return
	}

	// Store vault_key for bootstrap auto-unlock (deleted after VC registration)
	vaultKeyFile := filepath.Join(dataDir, "vault_key")
	if err := os.WriteFile(vaultKeyFile, []byte(password), 0600); err != nil {
		log.Printf("install: failed to write vault_key file: %v", err)
		// Non-fatal: manual unlock is still possible via password
	}

	log.Printf("install: initialization complete, node_id=%s", nodeID)

	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "initialized",
		"node_id": nodeID,
		"message": "Initialization complete. The server will restart in normal mode.",
	})

	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Println("install: exiting for restart in normal mode (exit 1 for systemd)")
		os.Exit(1)
	}()
}

// autoUnlock tries to unlock the server automatically.
// Priority: 1) vault_key file (bootstrap after init), 2) VC-managed unlock via agent_secret.
func autoUnlock(server *api.Server, vcURL string) {
	if !server.IsLocked() {
		return
	}
	dbPath := os.Getenv("VEILKEY_DB_PATH")
	dataDir := filepath.Dir(dbPath)

	// 1. Try vault_key file (exists between init and first VC registration)
	vaultKeyFile := filepath.Join(dataDir, "vault_key")
	if vkData, err := os.ReadFile(vaultKeyFile); err == nil {
		password := strings.TrimSpace(string(vkData))
		if password != "" {
			salt := server.Salt()
			kek := crypto.DeriveKEK(password, salt)
			if err := server.Unlock(kek); err == nil {
				log.Println("Auto-unlock: succeeded via vault_key file (bootstrap mode)")
				server.SetVaultUnlockKey(password)
				server.LoadIdentity()
				return
			}
			log.Printf("Auto-unlock: vault_key file invalid: %v", err)
		}
	}

	// 2. Try VC-managed unlock (agent_secret file → fetch unlock key from VC)
	if vcURL != "" && server.ReadAgentSecretFile() != "" {
		if err := server.AutoUnlockFromVC(vcURL); err == nil {
			log.Println("Auto-unlock: succeeded via VaultCenter")
			server.LoadIdentity()
			_ = os.Remove(vaultKeyFile)
			return
		}
		log.Printf("Auto-unlock: VC not reachable, starting background retry")
		// Retry in background — VC might not be up yet
		go retryAutoUnlock(server, vcURL, vaultKeyFile)
		return
	}

	log.Println("Auto-unlock: no method available. Waiting for POST /api/unlock.")
}

// retryAutoUnlock retries VC-managed unlock in the background with exponential backoff.
func retryAutoUnlock(server *api.Server, vcURL, vaultKeyFile string) {
	delays := []time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}
	for i, delay := range delays {
		time.Sleep(delay)
		if !server.IsLocked() {
			return // already unlocked (manual or other)
		}
		if err := server.AutoUnlockFromVC(vcURL); err == nil {
			log.Println("Auto-unlock: succeeded via VaultCenter (background retry)")
			server.LoadIdentity()
			_ = os.Remove(vaultKeyFile)
			return
		} else {
			log.Printf("Auto-unlock: retry %d/%d failed: %v", i+1, len(delays), err)
		}
	}
	log.Println("Auto-unlock: all retries exhausted. POST /api/unlock to unlock manually.")
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
		log.Printf("WARNING: salt file not found (%s), will attempt recovery from VaultCenter", saltFile)
		salt = nil
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
		log.Fatal("VEILKEY_TRUSTED_IPS is required (comma-separated CIDRs)")
	}

	// DB is NOT opened here — it opens during Unlock() when KEK is available.
	// DB_KEY = SHA256(KEK), so password is required to open the encrypted DB.
	server := api.NewServer(nil, nil, trustedIPs)
	server.SetDBPath(dbPath, salt)

	log.Println("Server started in LOCKED mode. POST /api/unlock with password to unlock.")

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
