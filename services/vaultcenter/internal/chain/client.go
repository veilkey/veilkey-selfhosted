package chain

import (
	"context"
	"fmt"

	rpclocal "github.com/cometbft/cometbft/rpc/client/local"
	nm "github.com/cometbft/cometbft/node"
)

// Client wraps the CometBFT local RPC client for TX submission.
type Client struct {
	local *rpclocal.Local
}

// NewClient creates a Client from a running CometBFT node.
func NewClient(node *nm.Node) *Client {
	return &Client{
		local: rpclocal.New(node),
	}
}

// BroadcastTxCommit submits a TX and waits for it to be included in a block.
// Returns the FinalizeBlock result code + log.
func (c *Client) BroadcastTxCommit(ctx context.Context, txType TxType, payload any) (code uint32, log string, err error) {
	txBytes, err := EncodeTx(txType, payload)
	if err != nil {
		return 0, "", fmt.Errorf("chain: encode tx: %w", err)
	}

	result, err := c.local.BroadcastTxCommit(ctx, txBytes)
	if err != nil {
		return 0, "", fmt.Errorf("chain: broadcast: %w", err)
	}

	// Check CheckTx result first
	if result.CheckTx.Code != 0 {
		return result.CheckTx.Code, result.CheckTx.Log, nil
	}

	// Then FinalizeBlock result
	return result.TxResult.Code, result.TxResult.Log, nil
}

// BroadcastTxSync submits a TX and waits for CheckTx only (not block inclusion).
func (c *Client) BroadcastTxSync(ctx context.Context, txType TxType, payload any) error {
	txBytes, err := EncodeTx(txType, payload)
	if err != nil {
		return fmt.Errorf("chain: encode tx: %w", err)
	}

	result, err := c.local.BroadcastTxSync(ctx, txBytes)
	if err != nil {
		return fmt.Errorf("chain: broadcast sync: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("chain: check tx failed: %s", result.Log)
	}
	return nil
}
