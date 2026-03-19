package db

import (
	"time"

	"github.com/veilkey/veilkey-go-package/refs"
	chain "github.com/veilkey/veilkey-chain"
)

// ChainStoreAdapter wraps *DB to implement chain.Store and chain.ChainMeta.
type ChainStoreAdapter struct {
	DB *DB
}

var (
	_ chain.Store        = (*ChainStoreAdapter)(nil)
	_ chain.ChainMeta = (*ChainStoreAdapter)(nil)
)

func (a *ChainStoreAdapter) SaveRefWithExpiryAndHash(parts chain.RefParts, ciphertext string, version int, status refs.RefStatus, expiresAt time.Time, secretName, plaintextHash string) error {
	dbParts := RefParts{Family: parts.Family, Scope: RefScope(parts.Scope), ID: parts.ID}
	return a.DB.SaveRefWithExpiryAndHash(dbParts, ciphertext, version, RefStatus(status), expiresAt, secretName, plaintextHash)
}

func (a *ChainStoreAdapter) UpdateRefWithName(canonical, ciphertext string, version int, status refs.RefStatus, name string) error {
	return a.DB.UpdateRefWithName(canonical, ciphertext, version, RefStatus(status), name)
}

func (a *ChainStoreAdapter) DeleteRef(canonical string) error {
	return a.DB.DeleteRef(canonical)
}

func (a *ChainStoreAdapter) UpsertAgent(nodeID, label, vaultHash, vaultName, ip string, port, secretsCount, configsCount, version, keyVersion int) error {
	return a.DB.UpsertAgent(nodeID, label, vaultHash, vaultName, ip, port, secretsCount, configsCount, version, keyVersion)
}

func (a *ChainStoreAdapter) RegisterChild(child *chain.ChildRecord) error {
	// Chain TX only records node identity — DEK delivery is out-of-band via REST.
	return a.DB.RegisterChild(&Child{
		NodeID:  child.NodeID,
		Label:   child.Label,
		URL:     child.URL,
		Version: child.Version,
	})
}

func (a *ChainStoreAdapter) SaveBinding(binding *chain.BindingRecord) error {
	return a.DB.SaveBinding(&Binding{
		BindingID:    binding.BindingID,
		BindingType:  binding.BindingType,
		TargetName:   binding.TargetName,
		VaultHash:    binding.VaultHash,
		SecretName:   binding.SecretName,
		FieldKey:     binding.FieldKey,
		RefCanonical: binding.RefCanonical,
		Required:     binding.Required,
	})
}

func (a *ChainStoreAdapter) DeleteBinding(bindingID string) error {
	return a.DB.DeleteBinding(bindingID)
}

func (a *ChainStoreAdapter) DeleteAgent(nodeID string) error {
	return a.DB.DeleteAgentByNodeID(nodeID)
}

func (a *ChainStoreAdapter) DeleteChild(nodeID string) error {
	return a.DB.DeleteChild(nodeID)
}

func (a *ChainStoreAdapter) UpdateChildURL(nodeID, url string) error {
	return a.DB.UpdateChildURL(nodeID, url)
}

func (a *ChainStoreAdapter) SaveGlobalFunction(fn *chain.GlobalFunctionRecord) error {
	return a.DB.SaveGlobalFunction(&GlobalFunction{
		Name:     fn.Name,
		Command:  fn.Body,
		Category: fn.Language,
	})
}

func (a *ChainStoreAdapter) DeleteGlobalFunction(name string) error {
	return a.DB.DeleteGlobalFunction(name)
}

func (a *ChainStoreAdapter) DeleteBindingsByTarget(bindingType, targetName string) error {
	return a.DB.DeleteBindingsByTarget(bindingType, targetName)
}

func (a *ChainStoreAdapter) SaveConfig(key, value string) error {
	return a.DB.SaveConfig(key, value)
}

func (a *ChainStoreAdapter) SaveAuditEvent(event *chain.AuditRecord) error {
	return a.DB.SaveAuditEvent(&AuditEvent{
		EventID:             event.EventID,
		EntityType:          event.EntityType,
		EntityID:            event.EntityID,
		Action:              event.Action,
		ActorType:           event.ActorType,
		ActorID:             event.ActorID,
		Reason:              event.Reason,
		Source:              event.Source,
		ApprovalChallengeID: event.ApprovalChallengeID,
		BeforeJSON:          event.BeforeJSON,
		AfterJSON:           event.AfterJSON,
	})
}

func (a *ChainStoreAdapter) GetConfigValue(key string) (string, error) {
	cfg, err := a.DB.GetConfig(key)
	if err != nil {
		return "", err
	}
	return cfg.Value, nil
}
