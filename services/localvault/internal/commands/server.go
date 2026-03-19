package commands

import (
	"encoding/json"
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

	chain "github.com/veilkey/veilkey-chain"
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

	// CometBFT chain full node (optional — set VEILKEY_CHAIN_HOME to enable)
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

		adapter := &db.ChainStoreAdapter{DB: server.DB()}
		cometNode, chainErr := chain.StartNode(adapter, adapter, chainHome)
		if chainErr != nil {
			log.Printf("Failed to start chain node: %v (continuing without chain)", chainErr)
		} else {
			defer chain.StopNode(cometNode)
			log.Printf("CometBFT full node started (home=%s)", chainHome)
		}
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
		handleInstallInit(w, r, database, dataDir)
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

func handleInstallInit(w http.ResponseWriter, r *http.Request, database *db.DB, dataDir string) {
	initMu.Lock()
	defer initMu.Unlock()

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

	saltFile := filepath.Join(dataDir, "salt")
	if _, err := os.Stat(saltFile); err == nil {
		http.Error(w, "already initialized", http.StatusConflict)
		return
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		log.Printf("install: failed to generate salt: %v", err)
		http.Error(w, "failed to generate salt", http.StatusInternalServerError)
		return
	}

	kek := crypto.DeriveKEK(req.Password, salt)

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
		http.Error(w, "failed to write salt file", http.StatusInternalServerError)
		return
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
		log.Fatal("VEILKEY_TRUSTED_IPS is required (comma-separated CIDRs)")
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

	if pw := cmdutil.ReadPasswordFromFileEnv(); pw != "" {
		kek := crypto.DeriveKEK(pw, salt)
		if _, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce); err != nil {
			log.Fatalf("Failed to unlock: invalid password")
		}
		server.Unlock(kek)
		log.Println("Server unlocked via VEILKEY_PASSWORD_FILE")
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
