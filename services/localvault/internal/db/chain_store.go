package db

import (
	"time"

	"github.com/veilkey/veilkey-go-package/refs"
	chain "github.com/veilkey/veilkey-chain"
)

// ChainStoreAdapter wraps *DB to implement chain.Store and chain.ChainMeta.
// localvault is a full node — it replicates blocks but only applies
// config and audit operations. Ref/agent/child operations are no-ops
// since localvault does not manage those entities.
type ChainStoreAdapter struct {
	DB *DB
}

var (
	_ chain.Store     = (*ChainStoreAdapter)(nil)
	_ chain.ChainMeta = (*ChainStoreAdapter)(nil)
)

// SaveRefWithExpiryAndHash is a no-op on localvault (vaultcenter manages refs).
func (a *ChainStoreAdapter) SaveRefWithExpiryAndHash(_ chain.RefParts, _ string, _ int, _ refs.RefStatus, _ time.Time, _, _ string) error {
	return nil
}

// UpdateRefWithName is a no-op on localvault.
func (a *ChainStoreAdapter) UpdateRefWithName(_, _ string, _ int, _ refs.RefStatus, _ string) error {
	return nil
}

// DeleteRef is a no-op on localvault.
func (a *ChainStoreAdapter) DeleteRef(_ string) error {
	return nil
}

// UpsertAgent is a no-op on localvault.
func (a *ChainStoreAdapter) UpsertAgent(_, _, _, _, _ string, _, _, _, _, _ int) error {
	return nil
}

// RegisterChild is a no-op on localvault (identity-only record, no DEK).
func (a *ChainStoreAdapter) RegisterChild(_ *chain.ChildRecord) error {
	return nil
}

// SaveBinding is a no-op on localvault (vaultcenter manages bindings).
func (a *ChainStoreAdapter) SaveBinding(_ *chain.BindingRecord) error { return nil }

// DeleteBinding is a no-op on localvault.
func (a *ChainStoreAdapter) DeleteBinding(_ string) error { return nil }

// SaveConfig applies config changes from chain blocks.
func (a *ChainStoreAdapter) SaveConfig(key, value string) error {
	return a.DB.SaveConfig(key, value)
}

// SaveAuditEvent replicates audit events from chain blocks.
func (a *ChainStoreAdapter) SaveAuditEvent(event *chain.AuditRecord) error {
	// localvault doesn't have a full AuditEvent table — store as config for now.
	// Future: add audit table to localvault if needed.
	return nil
}

// GetConfigValue reads a config value for chain state recovery.
func (a *ChainStoreAdapter) GetConfigValue(key string) (string, error) {
	cfg, err := a.DB.GetConfig(key)
	if err != nil {
		return "", err
	}
	return cfg.Value, nil
}
