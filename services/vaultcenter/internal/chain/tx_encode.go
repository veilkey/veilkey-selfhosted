package chain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// validTxTypes is the set of all recognised transaction types.
var validTxTypes = map[TxType]bool{
	TxSaveTokenRef:        true,
	TxUpdateTokenRef:      true,
	TxDeleteTokenRef:      true,
	TxUpsertAgent:         true,
	TxRegisterChild:       true,
	TxIncrementRefVersion: true,
	TxSaveBinding:         true,
	TxDeleteBinding:       true,
	TxSetConfig:           true,
	TxRecordAuditEvent:    true,
}

// EncodeTx creates a TxEnvelope, marshals the payload, and returns the full JSON bytes.
func EncodeTx(txType TxType, payload any) ([]byte, error) {
	if !validTxTypes[txType] {
		return nil, fmt.Errorf("chain: unknown tx type %q", txType)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("chain: marshal payload: %w", err)
	}

	env := TxEnvelope{
		Type:      txType,
		Nonce:     crypto.GenerateUUID(),
		Timestamp: time.Now().UTC(),
		Payload:   raw,
	}

	out, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("chain: marshal envelope: %w", err)
	}
	return out, nil
}

// DecodeTx unmarshals raw bytes into a TxEnvelope. Returns an error if the type is unknown.
func DecodeTx(raw []byte) (*TxEnvelope, error) {
	var env TxEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("chain: unmarshal envelope: %w", err)
	}
	if !validTxTypes[env.Type] {
		return nil, fmt.Errorf("chain: unknown tx type %q", env.Type)
	}
	return &env, nil
}

// DecodePayload unmarshals the envelope's Payload field into a concrete type.
func DecodePayload[T any](env *TxEnvelope) (*T, error) {
	var v T
	if err := json.Unmarshal(env.Payload, &v); err != nil {
		return nil, fmt.Errorf("chain: decode payload: %w", err)
	}
	return &v, nil
}
