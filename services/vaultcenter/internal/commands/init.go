package commands

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"veilkey-vaultcenter/internal/db"

	"github.com/veilkey/veilkey-go-package/cmdutil"
	"github.com/veilkey/veilkey-go-package/crypto"
)

func RunInit() {
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
		fmt.Println("Usage: veilkey-vaultcenter init --root")
		fmt.Println("  --root      Initialize as root node")
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

	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		log.Fatalf("Failed to save salt: %v", err)
	}

	pwCiphertext, pwNonce, pwErr := crypto.Encrypt(dek, []byte(password))
	tempRef := ""
	if pwErr == nil {
		pwRefID, refErr := cmdutil.GenerateHexRef(16)
		if refErr == nil {
			parts := db.RefParts{Family: db.RefFamilyVK, Scope: db.RefScopeTemp, ID: pwRefID}
			encoded := crypto.EncodeCiphertext(pwCiphertext, pwNonce)
			expiresAt := time.Now().UTC().Add(1 * time.Hour)
			if saveErr := database.SaveRefWithExpiry(parts, encoded, 1, db.RefStatusTemp, expiresAt, db.ConfigKeyVaultcenterPassword); saveErr == nil {
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
