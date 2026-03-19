package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"veilkey-vaultcenter/internal/api/hkm"
	"github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-vaultcenter/internal/db"
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
			"ref":        existing.RefCanonical,
			"name":       existing.SecretName,
			"expires_at": existing.ExpiresAt,
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

	refID, err := hkm.GenerateSecretRef(16)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate ref")
		return
	}

	parts := db.RefParts{Family: db.RefFamilyVK, Scope: db.RefScopeTemp, ID: refID}
	encoded := base64.StdEncoding.EncodeToString(ciphertext) + ":" + base64.StdEncoding.EncodeToString(nonce)
	expiresAt := time.Now().UTC().Add(1 * time.Hour)

	nodeInfo, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}

	if err := s.db.SaveRefWithExpiryAndHash(parts, encoded, nodeInfo.Version, db.RefStatusTemp, expiresAt, req.Name, plaintextHash); err != nil {
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

	agentURL, err := s.FindAgentURL(req.VaultHash)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "vault not found: "+err.Error())
		return
	}

	// Call LV's /api/promote endpoint
	promoteBody, err := json.Marshal(map[string]string{
		"ref":  req.Ref,
		"name": req.Name,
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to encode promote request")
		return
	}
	resp, err := s.httpClient.Post(
		strings.TrimRight(agentURL, "/")+"/api/promote",
		"application/json",
		bytes.NewReader(promoteBody),
	)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "failed to reach vault: "+err.Error())
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

	var promoteResp map[string]any
	if err := json.Unmarshal(body, &promoteResp); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to parse vault response")
		return
	}
	s.respondJSON(w, http.StatusOK, promoteResp)
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
