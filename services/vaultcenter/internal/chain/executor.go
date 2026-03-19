package chain

import (
	"fmt"
	"time"

	"veilkey-vaultcenter/internal/db"
)

// Execute applies a decoded TxEnvelope to the database.
// Returns (resultCode uint32, resultLog string).
// Code 0 = success, 1 = unknown type, 2 = decode error, 3 = db error.
func Execute(d *db.DB, env *TxEnvelope) (uint32, string) {
	switch env.Type {

	// ── TokenRef operations ─────────────────────────────────────────────

	case TxSaveTokenRef:
		p, err := DecodePayload[SaveTokenRefPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode SaveTokenRef: %v", err)
		}
		parts := db.RefParts{Family: p.RefFamily, Scope: db.RefScope(p.RefScope), ID: p.RefID}
		var expiresAt time.Time
		if p.ExpiresAt != nil {
			expiresAt = *p.ExpiresAt
		} else {
			expiresAt = time.Now().UTC().Add(4 * time.Hour)
		}
		if err := d.SaveRefWithExpiryAndHash(
			parts, p.Ciphertext, p.Version,
			db.RefStatus(p.Status), expiresAt,
			p.SecretName, p.PlaintextHash,
		); err != nil {
			return 3, fmt.Sprintf("db SaveTokenRef: %v", err)
		}
		return 0, "ok"

	case TxUpdateTokenRef:
		p, err := DecodePayload[UpdateTokenRefPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode UpdateTokenRef: %v", err)
		}
		if err := d.UpdateRefWithName(
			p.RefCanonical, p.Ciphertext, p.Version,
			db.RefStatus(p.Status), "",
		); err != nil {
			return 3, fmt.Sprintf("db UpdateTokenRef: %v", err)
		}
		return 0, "ok"

	case TxDeleteTokenRef:
		p, err := DecodePayload[DeleteTokenRefPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode DeleteTokenRef: %v", err)
		}
		if err := d.DeleteRef(p.RefCanonical); err != nil {
			return 3, fmt.Sprintf("db DeleteTokenRef: %v", err)
		}
		return 0, "ok"

	case TxIncrementRefVersion:
		p, err := DecodePayload[IncrementRefVersionPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode IncrementRefVersion: %v", err)
		}
		if err := d.UpdateRefWithName(p.RefCanonical, "", p.NewVersion, "", ""); err != nil {
			return 3, fmt.Sprintf("db IncrementRefVersion: %v", err)
		}
		return 0, "ok"

	// ── Agent operations ────────────────────────────────────────────────

	case TxUpsertAgent:
		p, err := DecodePayload[UpsertAgentPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode UpsertAgent: %v", err)
		}
		if err := d.UpsertAgent(
			p.NodeID, p.Label, p.VaultHash, p.VaultName,
			p.IP, p.Port, p.SecretsCount, p.ConfigsCount,
			p.Version, p.KeyVersion,
		); err != nil {
			return 3, fmt.Sprintf("db UpsertAgent: %v", err)
		}
		return 0, "ok"

	case TxRegisterChild:
		p, err := DecodePayload[RegisterChildPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode RegisterChild: %v", err)
		}
		child := &db.Child{
			NodeID:       p.NodeID,
			Label:        p.Label,
			URL:          p.URL,
			EncryptedDEK: p.EncryptedDEK,
			Nonce:        p.Nonce,
			Version:      p.Version,
		}
		if err := d.RegisterChild(child); err != nil {
			return 3, fmt.Sprintf("db RegisterChild: %v", err)
		}
		return 0, "ok"

	// ── Config operations ───────────────────────────────────────────────

	case TxSetConfig:
		p, err := DecodePayload[SetConfigPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode SetConfig: %v", err)
		}
		if err := d.SaveConfig(p.Key, p.Value); err != nil {
			return 3, fmt.Sprintf("db SetConfig: %v", err)
		}
		return 0, "ok"

	// ── Binding operations ──────────────────────────────────────────────

	case TxSaveBinding:
		_, err := DecodePayload[SaveBindingPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode SaveBinding: %v", err)
		}
		// TODO: implement d.SaveBinding() when binding DB methods are refactored
		return 0, "ok"

	case TxDeleteBinding:
		_, err := DecodePayload[DeleteBindingPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode DeleteBinding: %v", err)
		}
		// TODO: implement d.DeleteBinding() when binding DB methods are refactored
		return 0, "ok"

	// ── Audit operations ────────────────────────────────────────────────

	case TxRecordAuditEvent:
		_, err := DecodePayload[RecordAuditEventPayload](env)
		if err != nil {
			return 2, fmt.Sprintf("decode RecordAuditEvent: %v", err)
		}
		// TODO: implement d.SaveAuditEvent() when audit DB methods are refactored
		return 0, "ok"

	default:
		return 1, fmt.Sprintf("unknown tx type: %s", env.Type)
	}
}
