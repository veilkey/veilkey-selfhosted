package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"veilkey-localvault/internal/crypto"
	"veilkey-localvault/internal/db"
)

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref  string `json:"ref"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ref == "" || req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "ref and name are required")
		return
	}
	if !strings.HasPrefix(req.Ref, "VK:TEMP:") {
		s.respondError(w, http.StatusBadRequest, "only VK:TEMP refs can be promoted")
		return
	}
	if !isValidResourceName(req.Name) {
		s.respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	// Resolve keycenter URL
	target := s.resolveKeycenterTarget()
	if target.URL == "" {
		s.respondError(w, http.StatusServiceUnavailable, "keycenter URL not configured")
		return
	}

	// Pull plaintext from keycenter
	resolveURL := target.URL + "/api/resolve/" + url.PathEscape(req.Ref)
	resp, err := s.httpClient.Get(resolveURL)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "failed to reach keycenter: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.respondError(w, http.StatusBadGateway, fmt.Sprintf("keycenter resolve failed (%d): %s", resp.StatusCode, string(body)))
		return
	}

	var resolveResp struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resolveResp); err != nil {
		s.respondError(w, http.StatusBadGateway, "failed to parse keycenter response")
		return
	}
	if resolveResp.Value == "" {
		s.respondError(w, http.StatusBadGateway, "keycenter returned empty value")
		return
	}

	// Encrypt with localvault DEK
	nodeInfo, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}

	s.kekMu.RLock()
	kek := s.kek
	s.kekMu.RUnlock()

	dek, err := crypto.Decrypt(kek, nodeInfo.DEK, nodeInfo.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt DEK")
		return
	}

	ciphertext, nonce, err := crypto.Encrypt(dek, []byte(resolveResp.Value))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	// Save to localvault secrets table
	refID, err := generateSecretRef(8)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate ref")
		return
	}

	secret := &db.Secret{
		ID:         crypto.GenerateUUID(),
		Name:       strings.ToUpper(req.Name),
		Ref:        refID,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		Version:    nodeInfo.Version,
		Scope:      "LOCAL",
		Status:     "active",
	}
	if err := s.db.SaveSecret(secret); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save secret: "+err.Error())
		return
	}

	parsed := ParsedRef{Family: RefFamilyVK, Scope: RefScope("LOCAL"), ID: refID}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"ref":    parsed.CanonicalString(),
		"token":  parsed.CanonicalString(),
		"name":   secret.Name,
		"status": "active",
		"action": "promoted",
	})
}
