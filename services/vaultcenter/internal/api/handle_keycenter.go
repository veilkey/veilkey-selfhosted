package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"strings"
	"time"
	"veilkey-vaultcenter/internal/httputil"

	"veilkey-vaultcenter/internal/db"

	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/refs"
)

type tempRefItem struct {
	RefCanonical string     `json:"ref_canonical"`
	SecretName   string     `json:"secret_name"`
	AgentHash    string     `json:"agent_hash"`
	Status       string     `json:"status"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (s *Server) handleKeycenterTempRefs(w http.ResponseWriter, r *http.Request) {
	refs, err := s.db.ListActiveTempRefs()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list temp refs")
		return
	}
	items := make([]tempRefItem, 0, len(refs))
	for _, ref := range refs {
		items = append(items, tempRefItem{
			RefCanonical: ref.RefCanonical,
			SecretName:   ref.SecretName,
			AgentHash:    ref.AgentHash,
			Status:       string(ref.Status),
			ExpiresAt:    ref.ExpiresAt,
			CreatedAt:    ref.CreatedAt,
		})
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"refs": items})
}

func (s *Server) handleKeycenterCreateTempRef(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Value = strings.TrimSpace(req.Value)
	if req.Value == "" {
		s.respondError(w, http.StatusBadRequest, "value is required")
		return
	}

	// Dedup: check for existing active temp ref with same plaintext
	hash := sha256.Sum256([]byte(req.Value))
	plaintextHash := hex.EncodeToString(hash[:])
	if existing, err := s.db.FindActiveTempRefByHash(plaintextHash); err == nil && existing != nil {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"ref":          existing.RefCanonical,
			"name":         existing.SecretName,
			"expires_at":   existing.ExpiresAt,
			"deduplicated": true,
		})
		return
	}

	dek, err := s.GetLocalDEK()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get encryption key")
		return
	}
	ciphertext, nonce, err := crypto.Encrypt(dek, []byte(req.Value))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	refID, err := crypto.GenerateHexRef(16)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate ref")
		return
	}

	parts := db.RefParts{Family: db.RefFamilyVK, Scope: db.RefScopeTemp, ID: refID}
	encoded := crypto.EncodeCiphertext(ciphertext, nonce)
	expiresAt := time.Now().UTC().Add(tempRefTTL())

	nodeInfo, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}

	if _, err := s.SubmitTx(r.Context(), chain.TxSaveTokenRef, chain.SaveTokenRefPayload{
		RefFamily:     parts.Family,
		RefScope:      refs.RefScope(parts.Scope),
		RefID:         parts.ID,
		SecretName:    req.Name,
		PlaintextHash: plaintextHash,
		Ciphertext:    encoded,
		Version:       nodeInfo.Version,
		Status:        refs.RefStatus(db.RefStatusTemp),
		ExpiresAt:     &expiresAt,
	}); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save temp ref")
		return
	}

	canonical := parts.Canonical()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"ref":        canonical,
		"name":       req.Name,
		"expires_at": expiresAt,
	})
}

func (s *Server) handleKeycenterPromoteToVault(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref       string `json:"ref"`
		Name      string `json:"name"`
		VaultHash string `json:"vault_hash"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ref == "" || req.Name == "" || req.VaultHash == "" {
		s.respondError(w, http.StatusBadRequest, "ref, name, and vault_hash are required")
		return
	}

	// 1. Resolve temp-ref to plaintext (VC internal)
	tracked, err := s.db.GetRef(req.Ref)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "ref not found")
		return
	}
	plaintext, err := s.resolveTempRef(tracked)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to resolve ref")
		return
	}

	// 2. Find agent and decrypt agentDEK
	agent, err := s.FindAgentRecord(req.VaultHash)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "vault not found")
		return
	}
	agentDEK, err := s.DecryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt agent key")
		return
	}

	// 3. Encrypt with agentDEK (LV cannot decrypt — VC only)
	ciphertext, nonce, err := crypto.Encrypt(agentDEK, []byte(plaintext))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	// 4. Send pre-encrypted ciphertext to LV /api/cipher (not /api/promote)
	agentURL := s.AgentURL(agent.IP, agent.Port)
	cipherBody, err := json.Marshal(map[string]any{
		"name":       req.Name,
		"ciphertext": ciphertext,
		"nonce":      nonce,
		"scope":      "LOCAL",
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to encode request")
		return
	}
	resp, err := s.httpClient.Post(
		strings.TrimRight(agentURL, "/")+"/api/cipher",
		httputil.ContentTypeJSON,
		bytes.NewReader(cipherBody),
	)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "failed to reach vault")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "failed to read vault response")
		return
	}
	if resp.StatusCode != http.StatusOK {
		s.respondError(w, resp.StatusCode, "promote failed: "+string(body))
		return
	}

	var cipherResp map[string]any
	if err := json.Unmarshal(body, &cipherResp); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to parse vault response")
		return
	}

	// 5. Register tracked ref so /api/resolve/{ref} works
	if refID, ok := cipherResp["ref"].(string); ok && refID != "" {
		_, txErr := s.SubmitTx(r.Context(), chain.TxSaveTokenRef, chain.SaveTokenRefPayload{
			RefFamily:  "VK",
			RefScope:   "LOCAL",
			RefID:      refID,
			SecretName: req.Name,
			AgentHash:  agent.AgentHash,
			Version:    agent.KeyVersion,
			Status:     "active",
		})
		if txErr != nil {
			log.Printf("promote: failed to register tracked ref: %v", txErr)
		}
		// Chain executor doesn't pass AgentHash through Store interface,
		// so update it directly after TX commits.
		canonical := db.RefParts{Family: db.RefFamilyVK, Scope: db.RefScopeLocal, ID: refID}.Canonical()
		if err := s.db.UpdateRefAgentHash(canonical, agent.AgentHash); err != nil {
			log.Printf("promote: failed to set agent hash: %v", err)
		}
	}

	s.respondJSON(w, http.StatusOK, cipherResp)
}

func (s *Server) handleKeycenterRevealRef(w http.ResponseWriter, r *http.Request) {
	canonical := r.PathValue("ref")
	if canonical == "" {
		s.respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	tracked, err := s.db.GetRef(canonical)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "ref not found")
		return
	}
	if tracked.RefScope != db.RefScopeTemp {
		s.respondError(w, http.StatusForbidden, "only TEMP refs can be revealed here")
		return
	}
	plaintext, err := s.resolveTempRef(tracked)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt ref")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"value": plaintext})
}
