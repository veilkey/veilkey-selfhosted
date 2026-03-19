package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"veilkey-vaultcenter/internal/api"
	"veilkey-vaultcenter/internal/commands"
	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-vaultcenter/internal/db"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runHKMInit()
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
		commands.RunSetupServer(dbPath, dataDir)
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
		log.Println("WARNING: VEILKEY_TRUSTED_IPS not set, sensitive endpoints are unrestricted")
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
	} else if pw := readDataDirPassword(dataDir); pw != "" {
		kek := crypto.DeriveKEK(pw, salt)
		if err := server.Unlock(kek); err != nil {
			log.Fatalf("Failed to unlock with data dir password file: %v", err)
		}
		log.Println("Server unlocked via data dir password file")
	} else if os.Getenv("VEILKEY_PASSWORD") != "" {
		log.Fatal("VEILKEY_PASSWORD env var is no longer supported (password exposed in process environment). Use VEILKEY_PASSWORD_FILE instead.")
	} else {
		log.Println("Server started in LOCKED mode. POST /api/unlock with password to unlock.")
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

func readDataDirPassword(dataDir string) string {
	path := filepath.Join(dataDir, "password")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// runHKMInit handles: veilkey-storage init --root
func runHKMInit() {
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
		fmt.Println("Usage: veilkey-storage init --root")
		fmt.Println("  --root      Initialize as root node")
		fmt.Println("  Password is read from stdin (pipe) or interactive TTY prompt.")
		os.Exit(1)
	}

	password := ""

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

	if password == "" {
		password = cmdutil.ReadPassword("Enter KEK password: ")
		stat, _ := os.Stdin.Stat()
		isPiped := (stat.Mode() & os.ModeCharDevice) == 0
		if !isPiped {
			password2 := cmdutil.ReadPassword("Confirm KEK password: ")
			if password != password2 {
				log.Fatal("Passwords do not match.")
			}
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

	pwCiphertext, pwNonce, pwErr := crypto.Encrypt(dek, []byte(password))
	tempRef := ""
	if pwErr == nil {
		pwRefID, refErr := cmdutil.GenerateHexRef(16)
		if refErr == nil {
			parts := db.RefParts{Family: db.RefFamilyVK, Scope: db.RefScopeTemp, ID: pwRefID}
			encoded := base64Encode(pwCiphertext) + ":" + base64Encode(pwNonce)
			expiresAt := time.Now().UTC().Add(1 * time.Hour)
			if saveErr := database.SaveRefWithExpiry(parts, encoded, 1, db.RefStatusTemp, expiresAt, "VAULTCENTER_PASSWORD"); saveErr == nil {
				tempRef = parts.Canonical()
			}
		}
	}

	fmt.Println("VeilKey HKM initialized (root node).")
	fmt.Printf("  Node ID: %s\n", nodeID)
	fmt.Printf("  Salt:    %s\n", saltFile)
	fmt.Printf("  DB:      %s\n", dbPath)
	fmt.Printf("  DEK v1:  created\n")
	if tempRef != "" {
		fmt.Println("")
		fmt.Printf("  Password ref: %s\n", tempRef)
		fmt.Println("  This ref expires in 1 hour. Retrieve your password before then:")
		fmt.Printf("    curl -s http://localhost:<port>/api/resolve/%s\n", tempRef)
	}
	fmt.Println("")
	fmt.Println("  WARNING: Your password is the only way to unlock this server.")
	fmt.Println("  Store it in a secure location (e.g. password manager) within 1 hour.")
	fmt.Println("  After 1 hour, the temporary password ref is permanently deleted.")
	fmt.Println("  If you lose your password, all encrypted data is unrecoverable.")
	fmt.Println("  VeilKey assumes no liability for data loss due to lost passwords.")
	fmt.Println("  Full responsibility for password custody lies with the operator.")
}

// detectExternalIP returns the first non-loopback IPv4 address
func detectExternalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
