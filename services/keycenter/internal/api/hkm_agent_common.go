package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
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
		port = db.DefaultAgentPort
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

func (s *Server) decryptAgentDEK(encDEK, encNonce []byte) ([]byte, error) {
	if len(encDEK) == 0 {
		return nil, fmt.Errorf("agent has no DEK assigned")
	}
	s.kekMu.RLock()
	kek := s.kek
	s.kekMu.RUnlock()

	return crypto.Decrypt(kek, encDEK, encNonce)
}

func (s *Server) findAgent(hashOrLabel string) (*agentInfo, error) {
	agent, err := s.findAgentRecord(hashOrLabel)
	if err != nil {
		return nil, err
	}
	if err := validateAgentAvailability(agent); err != nil {
		return nil, err
	}
	return agentToInfo(agent), nil
}

func (s *Server) findAgentRecord(hashOrLabel string) (*db.Agent, error) {
	agent, err := s.db.GetAgentRecord(hashOrLabel)
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

func (s *Server) respondAgentLookupError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var stateErr *agentStateError
	if errors.As(err, &stateErr) {
		s.respondError(w, stateErr.statusCode, stateErr.message)
		return
	}
	s.respondError(w, http.StatusNotFound, err.Error())
}

func (s *Server) fetchAgentCiphertext(agentURL, ref string) (*cipherSecret, error) {
	resp, err := s.httpClient.Get(agentURL + "/api/cipher/" + ref)
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

func (s *Server) fetchAgentFieldCiphertext(agentURL, ref, fieldKey string) (*cipherSecretField, error) {
	resp, err := s.httpClient.Get(agentURL + "/api/cipher/" + ref + "/fields/" + fieldKey)
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
