package commands

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"veilkey-vaultcenter/internal/api/admin"
	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
)

var setupMu sync.Mutex

func RunSetupServer(dbPath, dataDir string) {
	addr := os.Getenv("VEILKEY_ADDR")
	if addr == "" {
		log.Fatal("VEILKEY_ADDR is required")
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()

	// Serve setup.html at root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if body, ok := admin.EmbeddedUISetupFile(); ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(body)
			return
		}
		http.Error(w, "setup ui not available (run build-admin-ui.sh)", http.StatusServiceUnavailable)
	})

	// Serve static assets (shared with admin/install builds)
	if assets, ok := admin.EmbeddedUIAssets(); ok {
		mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	}
	if body, ok := admin.EmbeddedUIStaticFile("favicon.svg"); ok {
		mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Write(body)
		})
	}

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"setup"}`))
	})

	saltFile := dataDir + "/salt"
	mux.HandleFunc("POST /api/setup/init", func(w http.ResponseWriter, r *http.Request) {
		handleSetupInit(w, r, database, saltFile)
	})

	log.Printf("veilkey-vaultcenter setup mode on %s (waiting for first-run initialization)", addr)

	tlsCert := os.Getenv("VEILKEY_TLS_CERT")
	tlsKey := os.Getenv("VEILKEY_TLS_KEY")
	if tlsCert != "" && tlsKey != "" {
		if err := http.ListenAndServeTLS(addr, tlsCert, tlsKey, mux); err != nil {
			log.Fatalf("Setup server failed: %v", err)
		}
	} else {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Setup server failed: %v", err)
		}
	}
}

func handleSetupInit(w http.ResponseWriter, r *http.Request, database *db.DB, saltFile string) {
	setupMu.Lock()
	defer setupMu.Unlock()

	// Already initialized (race condition guard)
	if _, err := os.Stat(saltFile); err == nil {
		http.Error(w, "already initialized", http.StatusConflict)
		return
	}

	var req struct {
		Password      string `json:"password"`
		AdminPassword string `json:"admin_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	req.AdminPassword = strings.TrimSpace(req.AdminPassword)
	if len(req.Password) < 8 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "password must be at least 8 characters"})
		return
	}
	if len(req.AdminPassword) < 8 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin_password must be at least 8 characters"})
		return
	}

	salt, err := crypto.GenerateSalt()
	if err != nil {
		log.Printf("setup: failed to generate salt: %v", err)
		http.Error(w, "failed to generate salt", http.StatusInternalServerError)
		return
	}
	kek := crypto.DeriveKEK(req.Password, salt)

	dek, err := crypto.GenerateKey()
	if err != nil {
		log.Printf("setup: failed to generate DEK: %v", err)
		http.Error(w, "failed to generate DEK", http.StatusInternalServerError)
		return
	}

	encDEK, encNonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		log.Printf("setup: failed to encrypt DEK: %v", err)
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
		log.Printf("setup: failed to save node info: %v", err)
		http.Error(w, "failed to save node info", http.StatusInternalServerError)
		return
	}

	// Store password as VK:TEMP ref (1-hour window)
	tempRef := ""
	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	if pwCipher, pwNonce, pwErr := crypto.Encrypt(dek, []byte(req.Password)); pwErr == nil {
		if refID, refErr := generateInitRef(16); refErr == nil {
			parts := db.RefParts{Family: "VK", Scope: "TEMP", ID: refID}
			encoded := base64.StdEncoding.EncodeToString(pwCipher) + ":" + base64.StdEncoding.EncodeToString(pwNonce)
			if saveErr := database.SaveRefWithExpiry(parts, encoded, 1, "temp", expiresAt, "VAULTCENTER_PASSWORD"); saveErr == nil {
				tempRef = parts.Canonical()
			}
		}
	}

	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		log.Printf("setup: failed to write salt: %v", err)
		http.Error(w, "failed to write salt", http.StatusInternalServerError)
		return
	}

	if err := database.SetAdminPassword(req.AdminPassword); err != nil {
		log.Printf("setup: failed to set admin password: %v", err)
		http.Error(w, "failed to set admin password", http.StatusInternalServerError)
		return
	}

	// Store admin password as VK:TEMP ref (1-hour window)
	adminTempRef := ""
	if pwCipher, pwNonce, pwErr := crypto.Encrypt(dek, []byte(req.AdminPassword)); pwErr == nil {
		if refID, refErr := generateInitRef(16); refErr == nil {
			parts := db.RefParts{Family: "VK", Scope: "TEMP", ID: refID}
			encoded := base64.StdEncoding.EncodeToString(pwCipher) + ":" + base64.StdEncoding.EncodeToString(pwNonce)
			if saveErr := database.SaveRefWithExpiry(parts, encoded, 1, "temp", expiresAt, "ADMIN_PASSWORD"); saveErr == nil {
				adminTempRef = parts.Canonical()
			}
		}
	}

	log.Printf("setup: initialization complete, node_id=%s, temp_ref=%s, admin_temp_ref=%s", nodeID, tempRef, adminTempRef)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"node_id":        nodeID,
		"temp_ref":       tempRef,
		"admin_temp_ref": adminTempRef,
		"expires_at":     expiresAt.Format(time.RFC3339),
	})

	go func() {
		time.Sleep(500 * time.Millisecond)
		log.Println("setup: exiting for restart in normal mode")
		os.Exit(1)
	}()
}
