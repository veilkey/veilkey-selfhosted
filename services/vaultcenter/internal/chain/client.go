package chain

import (
	"context"
	"fmt"

	nm "github.com/cometbft/cometbft/node"
	rpclocal "github.com/cometbft/cometbft/rpc/client/local"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
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

// BroadcastTxCommitRaw submits pre-encoded TX bytes and waits for block inclusion.
// Returns the raw result for the caller to inspect codes.
func (c *Client) BroadcastTxCommitRaw(ctx context.Context, txBytes []byte) (*ctypes.ResultBroadcastTxCommit, error) {
	result, err := c.local.BroadcastTxCommit(ctx, txBytes)
	if err != nil {
		return nil, fmt.Errorf("chain: broadcast commit: %w", err)
	}
	return result, nil
}

// BroadcastTxSyncRaw submits pre-encoded TX bytes and waits for CheckTx only.
func (c *Client) BroadcastTxSyncRaw(ctx context.Context, txBytes []byte) error {
	result, err := c.local.BroadcastTxSync(ctx, txBytes)
	if err != nil {
		return fmt.Errorf("chain: broadcast sync: %w", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("chain: check tx failed: %s", result.Log)
	}
	return nil
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
