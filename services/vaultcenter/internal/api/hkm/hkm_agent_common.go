package hkm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"veilkey-vaultcenter/internal/db"

	"github.com/veilkey/veilkey-go-package/agentapi"
	"github.com/veilkey/veilkey-go-package/crypto"
)

type agentStateError struct {
	statusCode int
	message    string
}

func (e *agentStateError) Error() string {
	return e.message
}

type agentInfo struct {
	NodeID         string
	Label          string
	AgentHash      string
	VaultHash      string
	VaultName      string
	IP             string
	Port           int
	KeyVersion     int
	DEK            []byte
	DEKNonce       []byte
	RebindRequired bool
	RebindReason   string
	RetryStage     int
	NextRetryAt    string
	Blocked        bool
	BlockReason    string
}

func (a *agentInfo) URL() string {
	port := a.Port
	if port == 0 {
		port = agentapi.DefaultPort
	}
	return fmt.Sprintf("%s://%s:%d", AgentScheme(), a.IP, port)
}

type cipherSecret struct {
	Name       string
	Ciphertext []byte
	Nonce      []byte
}

type cipherSecretField struct {
	Name       string
	FieldKey   string
	FieldType  string
	Ciphertext []byte
	Nonce      []byte
}

func agentToInfo(agent *db.Agent) *agentInfo {
	nextRetryAt := ""
	if agent.NextRetryAt != nil {
		nextRetryAt = agent.NextRetryAt.UTC().Format(time.RFC3339)
	}
	return &agentInfo{
		NodeID:         agent.NodeID,
		Label:          agent.Label,
		AgentHash:      agent.AgentHash,
		VaultHash:      agent.VaultHash,
		VaultName:      agent.VaultName,
		IP:             agent.IP,
		Port:           agent.Port,
		KeyVersion:     agent.KeyVersion,
		DEK:            agent.DEK,
		DEKNonce:       agent.DEKNonce,
		RebindRequired: agent.RebindRequired,
		RebindReason:   agent.RebindReason,
		RetryStage:     agent.RetryStage,
		NextRetryAt:    nextRetryAt,
		Blocked:        agent.BlockedAt != nil,
		BlockReason:    agent.BlockReason,
	}
}

func generateAgentHash() (string, error) {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generateAgentSecret generates a 32-byte random secret and returns its hex representation.
func generateAgentSecret() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// decryptAgentSecret decrypts an agent's stored encrypted secret using the KEK.
// Returns empty string if the agent has no encrypted secret stored.
func (h *Handler) decryptAgentSecret(encSecret, encNonce []byte) string {
	if len(encSecret) == 0 {
		return ""
	}
	kek := h.deps.GetKEK()
	plaintext, err := crypto.Decrypt(kek, encSecret, encNonce)
	if err != nil {
		return ""
	}
	return string(plaintext)
}

// setAgentAuthHeader adds an Authorization Bearer header to the request if the agent has a secret.
func (h *Handler) setAgentAuthHeader(req *http.Request, agent *agentInfo) {
	dbAgent, err := h.deps.DB().GetAgentByHash(agent.AgentHash)
	if err != nil {
		return
	}
	secret := h.decryptAgentSecret(dbAgent.AgentSecretEnc, dbAgent.AgentSecretNonce)
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
}

// agentSecretForHash retrieves and decrypts the agent secret for the given agent hash.
func (h *Handler) agentSecretForHash(agentHash string) string {
	dbAgent, err := h.deps.DB().GetAgentByHash(agentHash)
	if err != nil {
		return ""
	}
	return h.decryptAgentSecret(dbAgent.AgentSecretEnc, dbAgent.AgentSecretNonce)
}

func (h *Handler) decryptAgentDEK(encDEK, encNonce []byte) ([]byte, error) {
	if len(encDEK) == 0 {
		return nil, fmt.Errorf("agent has no DEK assigned")
	}
	// kek lock handled by deps
	kek := h.deps.GetKEK()
	// kek unlock handled by deps

	return crypto.Decrypt(kek, encDEK, encNonce)
}

func (h *Handler) findAgent(hashOrLabel string) (*agentInfo, error) {
	agent, err := h.findAgentRecord(hashOrLabel)
	if err != nil {
		return nil, err
	}
	if err := validateAgentAvailability(agent); err != nil {
		return nil, err
	}
	return agentToInfo(agent), nil
}

func (h *Handler) findAgentRecord(hashOrLabel string) (*db.Agent, error) {
	agent, err := h.deps.DB().GetAgentRecord(hashOrLabel)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s", hashOrLabel)
	}
	return agent, nil
}

func validateAgentAvailability(agent *db.Agent) error {
	if agent.BlockedAt != nil {
		return &agentStateError{
			statusCode: http.StatusLocked,
			message:    "agent is blocked and requires human-approved rebind",
		}
	}
	if agent.RebindRequired {
		return &agentStateError{
			statusCode: http.StatusConflict,
			message:    "agent requires human-approved rebind",
		}
	}
	return nil
}

func (h *Handler) respondAgentLookupError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var stateErr *agentStateError
	if errors.As(err, &stateErr) {
		respondError(w, stateErr.statusCode, stateErr.message)
		return
	}
	respondError(w, http.StatusNotFound, "not found")
}

func (h *Handler) fetchAgentCiphertext(agent *agentInfo, ref string) (*cipherSecret, error) {
	req, err := http.NewRequest(http.MethodGet, joinPath(agent.URL(), agentPathCipher, ref), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	h.setAgentAuthHeader(req, agent)

	resp, err := h.deps.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned %d", resp.StatusCode)
	}

	var data struct {
		Name       string `json:"name"`
		Ciphertext []byte `json:"ciphertext"`
		Nonce      []byte `json:"nonce"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid agent response: %w", err)
	}

	return &cipherSecret{
		Name:       data.Name,
		Ciphertext: data.Ciphertext,
		Nonce:      data.Nonce,
	}, nil
}

func (h *Handler) fetchAgentFieldCiphertext(agent *agentInfo, ref, fieldKey string) (*cipherSecretField, error) {
	req, err := http.NewRequest(http.MethodGet, joinPath(agent.URL(), agentPathCipher, ref, "fields", fieldKey), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	h.setAgentAuthHeader(req, agent)

	resp, err := h.deps.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned %d", resp.StatusCode)
	}

	var data struct {
		Name       string `json:"name"`
		Field      string `json:"field"`
		Type       string `json:"type"`
		Ciphertext []byte `json:"ciphertext"`
		Nonce      []byte `json:"nonce"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid agent response: %w", err)
	}

	return &cipherSecretField{
		Name:       data.Name,
		FieldKey:   data.Field,
		FieldType:  data.Type,
		Ciphertext: data.Ciphertext,
		Nonce:      data.Nonce,
	}, nil
}
