package chain

import (
	"encoding/json"
	"time"

	"github.com/veilkey/veilkey-go-package/refs"
)

type TxType string

const (
	TxSaveTokenRef        TxType = "SaveTokenRef"
	TxUpdateTokenRef      TxType = "UpdateTokenRef"
	TxDeleteTokenRef      TxType = "DeleteTokenRef"
	TxUpsertAgent         TxType = "UpsertAgent"
	TxRegisterChild       TxType = "RegisterChild"
	TxIncrementRefVersion TxType = "IncrementRefVersion"
	TxSaveBinding         TxType = "SaveBinding"
	TxDeleteBinding       TxType = "DeleteBinding"
	TxSetConfig           TxType = "SetConfig"
	TxRecordAuditEvent    TxType = "RecordAuditEvent"
)

type TxEnvelope struct {
	Type      TxType          `json:"type"`
	Nonce     string          `json:"nonce"`
	Timestamp time.Time       `json:"timestamp"`
	ActorType string          `json:"actor_type,omitempty"` // "agent", "operator", "system", "api"
	ActorID   string          `json:"actor_id,omitempty"`   // remote IP or agent hash
	Source    string          `json:"source,omitempty"`     // route path or handler name
	Payload   json.RawMessage `json:"payload"`
}

// SaveTokenRefPayload carries the data for a SaveTokenRef transaction.
type SaveTokenRefPayload struct {
	RefFamily     string         `json:"ref_family"`
	RefScope      refs.RefScope  `json:"ref_scope"`
	RefID         string         `json:"ref_id"`
	SecretName    string         `json:"secret_name"`
	AgentHash     string         `json:"agent_hash"`
	PlaintextHash string         `json:"plaintext_hash,omitempty"`
	Ciphertext    string         `json:"ciphertext"`
	Version       int            `json:"version"`
	Status        refs.RefStatus `json:"status"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
}

// UpdateTokenRefPayload carries the data for an UpdateTokenRef transaction.
type UpdateTokenRefPayload struct {
	RefCanonical  string         `json:"ref_canonical"`
	PlaintextHash string         `json:"plaintext_hash,omitempty"`
	Ciphertext    string         `json:"ciphertext,omitempty"`
	Version       int            `json:"version"`
	Status        refs.RefStatus `json:"status,omitempty"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
}

// DeleteTokenRefPayload carries the data for a DeleteTokenRef transaction.
type DeleteTokenRefPayload struct {
	RefCanonical string `json:"ref_canonical"`
}

// UpsertAgentPayload carries the data for an UpsertAgent transaction.
type UpsertAgentPayload struct {
	NodeID           string `json:"node_id"`
	Label            string `json:"label"`
	AgentHash        string `json:"agent_hash"`
	VaultHash        string `json:"vault_hash"`
	VaultName        string `json:"vault_name"`
	AgentRole        string `json:"agent_role"`
	HostEnabled      bool   `json:"host_enabled"`
	LocalEnabled     bool   `json:"local_enabled"`
	ManagedPaths     string `json:"managed_paths,omitempty"`
	KeyVersion       int    `json:"key_version"`
	RotationRequired bool   `json:"rotation_required"`
	RotationReason   string `json:"rotation_reason,omitempty"`
	RebindRequired   bool   `json:"rebind_required"`
	RebindReason     string `json:"rebind_reason,omitempty"`
	IP               string `json:"ip,omitempty"`
	Port             int    `json:"port,omitempty"`
	DEK              []byte `json:"dek,omitempty"`
	DEKNonce         []byte `json:"dek_nonce,omitempty"`
	SecretsCount     int    `json:"secrets_count"`
	ConfigsCount     int    `json:"configs_count"`
	Version          int    `json:"version"`
}

// RegisterChildPayload carries the data for a RegisterChild transaction.
type RegisterChildPayload struct {
	NodeID       string `json:"node_id"`
	Label        string `json:"label"`
	URL          string `json:"url"`
	EncryptedDEK []byte `json:"encrypted_dek"`
	Nonce        []byte `json:"nonce"`
	Version      int    `json:"version"`
}

// IncrementRefVersionPayload carries the data for an IncrementRefVersion transaction.
type IncrementRefVersionPayload struct {
	RefCanonical string `json:"ref_canonical"`
	NewVersion   int    `json:"new_version"`
}

// SaveBindingPayload carries the data for a SaveBinding transaction.
type SaveBindingPayload struct {
	BindingID    string `json:"binding_id"`
	BindingType  string `json:"binding_type"`
	TargetName   string `json:"target_name"`
	VaultHash    string `json:"vault_hash"`
	SecretName   string `json:"secret_name"`
	FieldKey     string `json:"field_key"`
	RefCanonical string `json:"ref_canonical"`
	Required     bool   `json:"required"`
}

// DeleteBindingPayload carries the data for a DeleteBinding transaction.
type DeleteBindingPayload struct {
	BindingID string `json:"binding_id"`
}

// SetConfigPayload carries the data for a SetConfig transaction.
type SetConfigPayload struct {
	Key    string         `json:"key"`
	Value  string         `json:"value"`
	Scope  refs.RefScope  `json:"scope"`
	Status refs.RefStatus `json:"status"`
}

// RecordAuditEventPayload carries the data for a RecordAuditEvent transaction.
type RecordAuditEventPayload struct {
	EventID             string `json:"event_id"`
	EntityType          string `json:"entity_type"`
	EntityID            string `json:"entity_id"`
	Action              string `json:"action"`
	ActorType           string `json:"actor_type"`
	ActorID             string `json:"actor_id"`
	Reason              string `json:"reason,omitempty"`
	Source              string `json:"source,omitempty"`
	ApprovalChallengeID string `json:"approval_challenge_id,omitempty"`
	BeforeJSON          string `json:"before_json,omitempty"`
	AfterJSON           string `json:"after_json,omitempty"`
}
