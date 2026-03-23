package commands

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"veilkey-vaultcenter/internal/api"
	"veilkey-vaultcenter/internal/db"

	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
)

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
		RunSetupServer(dbPath, dataDir)
		return
	}

	salt, err := os.ReadFile(saltFile)
	if err != nil {
		log.Fatalf("Failed to read salt file: %v", err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

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

	server := api.NewServer(database, nil, trustedIPs)
	server.SetSalt(salt)

	if database.HasNodeInfo() {
		info, err := database.GetNodeInfo()
		if err != nil {
			log.Fatalf("Failed to load node info: %v", err)
		}
		server.SetIdentity(&api.NodeIdentity{
			NodeID:    info.NodeID,
			ParentURL: info.ParentURL,
			Version:   info.Version,
			IsHKM:     true,
		})
		log.Printf("HKM mode: node=%s version=%d", info.NodeID, info.Version)
	} else {
		log.Fatal("node info not found. Legacy centralized mode is no longer supported; initialize HKM root with 'init --root'.")
	}

	if pw := cmdutil.ReadPasswordFromFileEnv(); pw != "" {
		kek := crypto.DeriveKEK(pw, salt)
		if err := server.Unlock(kek); err != nil {
			log.Fatalf("Failed to unlock with VEILKEY_PASSWORD_FILE: %v", err)
		}
		log.Println("Server unlocked via VEILKEY_PASSWORD_FILE")
	} else if os.Getenv("VEILKEY_PASSWORD") != "" {
		log.Fatal("VEILKEY_PASSWORD env var is no longer supported (password exposed in process environment). Use VEILKEY_PASSWORD_FILE instead.")
	} else {
		log.Println("Server started in LOCKED mode. POST /api/unlock with password to unlock.")
	}

	// CometBFT chain node (optional — set VEILKEY_CHAIN_HOME to enable)
	if chainHome := os.Getenv("VEILKEY_CHAIN_HOME"); chainHome != "" {
		adapter := &db.ChainStoreAdapter{DB: database}
		cometNode, chainErr := chain.StartNode(adapter, adapter, chainHome)
		if chainErr != nil {
			log.Fatalf("Failed to start chain node: %v", chainErr)
		}
		defer func() { _ = chain.StopNode(cometNode) }()
		server.SetChainClient(chain.NewClient(cometNode))
		server.SetChainHome(chainHome)
		server.SetChainNodeID(string(cometNode.NodeInfo().ID()))
		log.Printf("CometBFT chain node started (home=%s, node=%s)", chainHome, cometNode.NodeInfo().ID())
	} else {
		log.Println("Chain disabled (VEILKEY_CHAIN_HOME not set, using DB direct mode)")
	}

	gcStop := make(chan struct{})
	defer close(gcStop)
	go api.StartTempRefGC(database, cmdutil.ParseDurationEnv("VEILKEY_GC_INTERVAL", 5*time.Minute), gcStop)
	log.Println("Temp ref GC started")

	handler := server.SetupRoutes()
	tlsCert := os.Getenv("VEILKEY_TLS_CERT")
	tlsKey := os.Getenv("VEILKEY_TLS_KEY")
	if tlsCert != "" && tlsKey != "" {
		log.Printf("veilkey server starting on %s (TLS)", addr)
		if err := http.ListenAndServeTLS(addr, tlsCert, tlsKey, handler); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	} else {
		log.Printf("veilkey server starting on %s", addr)
		log.Println("WARNING: TLS not configured (set VEILKEY_TLS_CERT and VEILKEY_TLS_KEY to enable)")
		if err := http.ListenAndServe(addr, handler); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}
}
