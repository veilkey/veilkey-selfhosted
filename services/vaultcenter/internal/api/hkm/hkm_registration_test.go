package hkm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"veilkey-vaultcenter/internal/db"

	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/crypto"
)

type registrationTestDeps struct {
	Deps
	database *db.DB
	kek      []byte
	trusted  bool
	upserts  int
}

func (d *registrationTestDeps) DB() *db.DB                    { return d.database }
func (d *registrationTestDeps) GetKEK() []byte                { return d.kek }
func (d *registrationTestDeps) IsTrustedIPString(string) bool { return d.trusted }
func (d *registrationTestDeps) SubmitTx(_ context.Context, txType chain.TxType, payload any) (string, error) {
	if txType != chain.TxUpsertAgent {
		return "", fmt.Errorf("unexpected transaction type: %v", txType)
	}
	p, ok := payload.(chain.UpsertAgentPayload)
	if !ok {
		return "", fmt.Errorf("unexpected upsert payload: %T", payload)
	}
	d.upserts++
	return "", d.database.UpsertAgent(p.NodeID, p.Label, p.VaultHash, p.VaultName, p.IP,
		p.Port, p.SecretsCount, p.ConfigsCount, p.Version, p.KeyVersion, p.Salt)
}

func newRegistrationHandler(t *testing.T, trusted bool) (*Handler, *registrationTestDeps) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "registration.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	kek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	deps := &registrationTestDeps{database: database, kek: kek, trusted: trusted}
	return NewHandler(deps), deps
}

func registrationHeartbeat(t *testing.T, h *Handler, nodeID, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"node_id": nodeID, "label": "registration-test", "vault_hash": "test-vault",
		"vault_name": "registration-test", "key_version": 1, "registration_token": token,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/agents/heartbeat", bytes.NewReader(body))
	r.RemoteAddr = "192.0.2.10:40000"
	w := httptest.NewRecorder()
	h.handleAgentHeartbeat(w, r)
	return w
}

func TestHeartbeatNewAgentRequiresTokenRegardlessOfTrustedIP(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(fmt.Sprintf("trusted=%t", trusted), func(t *testing.T) {
			h, deps := newRegistrationHandler(t, trusted)
			nodeID := crypto.GenerateUUID()
			w := registrationHeartbeat(t, h, nodeID, "")
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 without registration token, got %d", w.Code)
			}
			if deps.upserts != 0 {
				t.Fatal("registration must be rejected before any agent transaction")
			}
			if _, err := deps.database.GetAgentByNodeID(nodeID); err == nil {
				t.Fatal("tokenless request must not create an agent")
			}
		})
	}
}

func TestHeartbeatRegistrationValidTokenConsumedAndExistingAgentContinues(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(fmt.Sprintf("trusted=%t", trusted), func(t *testing.T) {
			h, deps := newRegistrationHandler(t, trusted)
			tokenID, nodeID := crypto.GenerateUUID(), crypto.GenerateUUID()
			if err := deps.database.SaveRegistrationToken(&db.RegistrationToken{
				TokenID: tokenID, Status: "active", ExpiresAt: time.Now().UTC().Add(time.Hour),
			}); err != nil {
				t.Fatal(err)
			}
			w := registrationHeartbeat(t, h, nodeID, tokenID)
			if w.Code != http.StatusOK {
				t.Fatalf("expected valid registration to succeed, got %d", w.Code)
			}
			var response struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Status != "registered" {
				t.Fatal("expected registered response")
			}
			token, err := deps.database.GetRegistrationToken(tokenID)
			if err != nil || token.Status != "used" || token.UsedByNode != nodeID {
				t.Fatal("valid registration must consume its token for the registered node")
			}
			agent, err := deps.database.GetAgentByNodeID(nodeID)
			if err != nil {
				t.Fatal(err)
			}
			if agent.AgentHash == "" || agent.AgentSecretHash == "" {
				t.Fatal("registration must assign agent identity and authentication")
			}
			w = registrationHeartbeat(t, h, nodeID, "")
			if w.Code != http.StatusOK {
				t.Fatalf("existing registration must work without a registration token, got %d", w.Code)
			}
			updated, err := deps.database.GetAgentByNodeID(nodeID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.AgentHash != agent.AgentHash || updated.AgentSecretHash != agent.AgentSecretHash || !bytes.Equal(updated.DEK, agent.DEK) {
				t.Fatal("existing registration credentials must be preserved")
			}
			w = registrationHeartbeat(t, h, crypto.GenerateUUID(), tokenID)
			if w.Code != http.StatusForbidden {
				t.Fatalf("used registration token must not register another agent, got %d", w.Code)
			}
		})
	}
}

func TestHeartbeatNewAgentRejectsInvalidRegistrationToken(t *testing.T) {
	for _, state := range []string{"unknown", "expired", "revoked"} {
		t.Run(state, func(t *testing.T) {
			h, deps := newRegistrationHandler(t, true)
			tokenID := crypto.GenerateUUID()
			if state != "unknown" {
				token := &db.RegistrationToken{TokenID: tokenID, Status: "active", ExpiresAt: time.Now().UTC().Add(-time.Hour)}
				if state == "revoked" {
					token.Status, token.ExpiresAt = "revoked", time.Now().UTC().Add(time.Hour)
				}
				if err := deps.database.SaveRegistrationToken(token); err != nil {
					t.Fatal(err)
				}
			}
			w := registrationHeartbeat(t, h, crypto.GenerateUUID(), tokenID)
			if w.Code != http.StatusForbidden || deps.upserts != 0 {
				t.Fatalf("invalid registration must be rejected before upsert, got status %d and %d upserts", w.Code, deps.upserts)
			}
		})
	}
}
