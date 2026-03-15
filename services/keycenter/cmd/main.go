package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"veilkey-keycenter/internal/api"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
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
	dbPath := getEnvDefault("VEILKEY_DB_PATH", "/opt/veilkey/data/veilkey.db")
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
	defer database.Close()

	addr := getEnvDefault("VEILKEY_ADDR", ":10180")

	var trustedIPs []string
	if v := os.Getenv("VEILKEY_TRUSTED_IPS"); v != "" {
		trustedIPs = strings.Split(v, ",")
		log.Printf("Trusted IPs: %v", trustedIPs)
	} else {
		log.Println("WARNING: VEILKEY_TRUSTED_IPS not set, sensitive endpoints are unrestricted")
	}

	server := api.NewServer(database, nil, trustedIPs)
	server.SetSalt(salt)

	// Auto-detect HKM mode
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

	// Auto-unlock if VEILKEY_PASSWORD_FILE is set
	if pw := readPasswordFromFileEnv(); pw != "" {
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

	handler := server.SetupRoutes()
	log.Printf("veilkey server starting on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
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

	dbPath := getEnvDefault("VEILKEY_DB_PATH", "/opt/veilkey/data/veilkey.db")
	dataDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}
	saltFile := filepath.Join(dataDir, "salt")

	// Check if already initialized
	if _, err := os.Stat(saltFile); err == nil {
		log.Fatal("Already initialized. Salt file exists: " + saltFile)
	}

	// Get password
	if password == "" {
		password = readPassword("Enter KEK password: ")
		stat, _ := os.Stdin.Stat()
		isPiped := (stat.Mode() & os.ModeCharDevice) == 0
		if !isPiped {
			password2 := readPassword("Confirm KEK password: ")
			if password != password2 {
				log.Fatal("Passwords do not match.")
			}
		}
	}
	if len(password) < 8 {
		log.Fatal("Password must be at least 8 characters.")
	}

	// Generate salt + derive KEK
	salt, err := crypto.GenerateSalt()
	if err != nil {
		log.Fatalf("Failed to generate salt: %v", err)
	}
	kek := crypto.DeriveKEK(password, salt)

	// Initialize database
	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Generate UUID + DEK
	nodeID := crypto.GenerateUUID()
	dek, err := crypto.GenerateKey()
	if err != nil {
		log.Fatalf("Failed to generate DEK: %v", err)
	}

	// Encrypt DEK with KEK
	encDEK, encNonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		log.Fatalf("Failed to encrypt DEK: %v", err)
	}

	// Save node info
	info := &db.NodeInfo{
		NodeID:   nodeID,
		DEK:      encDEK,
		DEKNonce: encNonce,
		Version:  1,
	}
	if err := database.SaveNodeInfo(info); err != nil {
		log.Fatalf("Failed to save node info: %v", err)
	}

	// Save salt
	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		log.Fatalf("Failed to save salt: %v", err)
	}

	fmt.Println("VeilKey HKM initialized (root node).")
	fmt.Printf("  Node ID: %s\n", nodeID)
	fmt.Printf("  Salt:    %s\n", saltFile)
	fmt.Printf("  DB:      %s\n", dbPath)
	fmt.Printf("  DEK v1:  created\n")
	fmt.Println("")
	fmt.Println("  IMPORTANT: Remember your password. Lost password = unrecoverable data.")
}

// readPasswordFromFileEnv reads the password from the file path specified in VEILKEY_PASSWORD_FILE.
// Returns empty string if the env var is not set.
func readPasswordFromFileEnv() string {
	path := os.Getenv("VEILKEY_PASSWORD_FILE")
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read VEILKEY_PASSWORD_FILE (%s): %v", path, err)
	}
	pw := strings.TrimRight(string(data), "\n\r")
	if pw == "" {
		log.Fatalf("VEILKEY_PASSWORD_FILE (%s) is empty", path)
	}
	return pw
}

func readPassword(prompt string) string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text())
		}
		return ""
	}

	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Fprint(os.Stderr, prompt)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text())
		}
		return ""
	}
	defer tty.Close()

	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(tty)
	if scanner.Scan() {
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func getEnvDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// parseDurationEnv reads a duration from env var (e.g. "30s", "5m"), falls back to default
func parseDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("warning: invalid duration %s=%q, using default %s", key, v, defaultVal)
	}
	return defaultVal
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
