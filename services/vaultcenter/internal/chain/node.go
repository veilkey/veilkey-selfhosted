package chain

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	cfg "github.com/cometbft/cometbft/config"
	cmtflags "github.com/cometbft/cometbft/libs/cli/flags"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	nm "github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"

	"veilkey-vaultcenter/internal/db"
)

// StartNode creates and starts a CometBFT node embedded in-process.
// chainHome is the directory for CometBFT config/data (e.g. $dataDir/chain).
func StartNode(database *db.DB, chainHome string) (*nm.Node, error) {
	if err := ensureChainHome(chainHome); err != nil {
		return nil, fmt.Errorf("chain: ensure home: %w", err)
	}

	config := defaultCometConfig(chainHome)

	pv := privval.LoadFilePV(
		config.PrivValidatorKeyFile(),
		config.PrivValidatorStateFile(),
	)

	nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
	if err != nil {
		return nil, fmt.Errorf("chain: load node key: %w", err)
	}

	logger := cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout))
	logger, _ = cmtflags.ParseLogLevel(config.LogLevel, logger, cfg.DefaultLogLevel)

	app := NewApplication(database)

	node, err := nm.NewNode(
		config,
		pv,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		nm.DefaultGenesisDocProviderFunc(config),
		cfg.DefaultDBProvider,
		nm.DefaultMetricsProvider(config.Instrumentation),
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("chain: create node: %w", err)
	}

	if err := node.Start(); err != nil {
		return nil, fmt.Errorf("chain: start node: %w", err)
	}

	return node, nil
}

// StopNode gracefully stops a running CometBFT node.
func StopNode(node *nm.Node) error {
	if node == nil {
		return nil
	}
	if err := node.Stop(); err != nil {
		return err
	}
	node.Wait()
	return nil
}

func defaultCometConfig(chainHome string) *cfg.Config {
	config := cfg.DefaultConfig()
	config.SetRoot(chainHome)

	// Single validator: fast block times
	config.Consensus.TimeoutCommit = cfg.DefaultConsensusConfig().TimeoutCommit
	config.Consensus.CreateEmptyBlocks = false

	// RPC on localhost only
	config.RPC.ListenAddress = "tcp://127.0.0.1:26657"

	// P2P: allow external connections for future multi-node
	config.P2P.ListenAddress = "tcp://0.0.0.0:26656"

	// Minimal logging
	config.LogLevel = "error"

	return config
}

func ensureChainHome(chainHome string) error {
	configDir := filepath.Join(chainHome, "config")
	dataDir := filepath.Join(chainHome, "data")

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}

	// Generate validator key if not exists
	pvKeyFile := filepath.Join(configDir, "priv_validator_key.json")
	pvStateFile := filepath.Join(dataDir, "priv_validator_state.json")
	if _, err := os.Stat(pvKeyFile); os.IsNotExist(err) {
		pv := privval.GenFilePV(pvKeyFile, pvStateFile)
		pv.Save()
	}

	// Generate node key if not exists
	nodeKeyFile := filepath.Join(configDir, "node_key.json")
	if _, err := os.Stat(nodeKeyFile); os.IsNotExist(err) {
		if _, err := p2p.LoadOrGenNodeKey(nodeKeyFile); err != nil {
			return fmt.Errorf("generate node key: %w", err)
		}
	}

	// Generate genesis if not exists
	genesisFile := filepath.Join(configDir, "genesis.json")
	if _, err := os.Stat(genesisFile); os.IsNotExist(err) {
		if err := generateGenesis(pvKeyFile, genesisFile); err != nil {
			return fmt.Errorf("generate genesis: %w", err)
		}
	}

	return nil
}

func generateGenesis(pvKeyFile, genesisFile string) error {
	pv := privval.LoadFilePVEmptyState(pvKeyFile, "")

	genDoc := `{
  "genesis_time": "2026-01-01T00:00:00.000000000Z",
  "chain_id": "veilkey-chain-1",
  "initial_height": "1",
  "consensus_params": {
    "block": { "max_bytes": "22020096", "max_gas": "-1" },
    "evidence": { "max_age_num_blocks": "100000", "max_age_duration": "172800000000000", "max_bytes": "1048576" },
    "validator": { "pub_key_types": ["ed25519"] },
    "version": { "app": "0" },
    "abci": { "vote_extensions_enable_height": "0" }
  },
  "validators": [
    {
      "address": "%s",
      "pub_key": { "type": "tendermint/PubKeyEd25519", "value": "%s" },
      "power": "10",
      "name": "vaultcenter"
    }
  ],
  "app_hash": ""
}`

	pubKey := pv.Key.PubKey
	addr := pubKey.Address()

	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKey.Bytes())
	addrHex := fmt.Sprintf("%X", addr)

	content := fmt.Sprintf(genDoc, addrHex, pubKeyB64)
	return os.WriteFile(genesisFile, []byte(content), 0600)
}
