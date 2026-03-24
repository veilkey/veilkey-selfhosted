package commands

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"veilkey-vaultcenter/internal/api"
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

	// CometBFT chain node deferred — DB required for chain store adapter.
	// Chain will not start until after unlock when DB is available.
	if chainHome := os.Getenv("VEILKEY_CHAIN_HOME"); chainHome != "" {
		log.Printf("Chain home=%s (will start after unlock when DB is available)", chainHome)
	} else {
		log.Println("Chain disabled (VEILKEY_CHAIN_HOME not set, using DB direct mode)")
	}

	// Temp ref GC and plugins require DB — they start after unlock via post-unlock hook.
	// For now, log that they are deferred.
	log.Println("Temp ref GC and plugins deferred until unlock")

	handler, err := server.SetupRoutes()
	if err != nil {
		log.Fatalf("Failed to setup routes: %v", err)
	}
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
