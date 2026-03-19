package commands

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// fetchChainGenesis fetches genesis.json and persistent_peers from vaultcenter
// and writes them to the chain home directory.
func fetchChainGenesis(vaultcenterURL, chainHome string) {
	url := strings.TrimRight(vaultcenterURL, "/") + "/api/chain/info"
	log.Printf("Chain: fetching genesis from %s", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Chain: failed to fetch genesis: %v (will generate local genesis)", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Chain: vaultcenter returned %d for chain info (chain may not be enabled on vaultcenter)", resp.StatusCode)
		return
	}

	var data struct {
		Genesis         json.RawMessage `json:"genesis"`
		PersistentPeers string          `json:"persistent_peers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("Chain: failed to decode chain info: %v", err)
		return
	}

	if len(data.Genesis) == 0 {
		log.Println("Chain: empty genesis from vaultcenter")
		return
	}

	// Write genesis.json
	configDir := filepath.Join(chainHome, "config")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Printf("Chain: failed to create config dir: %v", err)
		return
	}
	genesisFile := filepath.Join(configDir, "genesis.json")
	if err := os.WriteFile(genesisFile, data.Genesis, 0600); err != nil {
		log.Printf("Chain: failed to write genesis: %v", err)
		return
	}
	log.Printf("Chain: genesis.json written to %s", genesisFile)

	// Write persistent_peers to a config snippet for CometBFT
	if data.PersistentPeers != "" {
		peersFile := filepath.Join(configDir, "persistent_peers.txt")
		_ = os.WriteFile(peersFile, []byte(data.PersistentPeers), 0600)
		log.Printf("Chain: persistent_peers=%s", data.PersistentPeers)
	}
}
