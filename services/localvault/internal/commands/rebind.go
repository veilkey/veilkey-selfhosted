package commands

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"veilkey-localvault/internal/db"
)

func RunRebind() {
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
