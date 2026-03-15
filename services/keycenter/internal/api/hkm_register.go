package api

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

// handleRegister registers a new child node and issues a DEK
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultNodeUUID string `json:"vault_node_uuid"`
		NodeID        string `json:"node_id"`
		Label         string `json:"label"`
		URL           string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	nodeID := req.VaultNodeUUID
	if nodeID == "" {
		nodeID = req.NodeID
	}
	if nodeID == "" {
		s.respondError(w, http.StatusBadRequest, "vault_node_uuid or node_id is required")
		return
	}
	if req.URL != "" {
		parsedURL, err := url.Parse(strings.TrimSpace(req.URL))
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			s.respondError(w, http.StatusBadRequest, "url must be a valid http(s) URL")
			return
		}
		req.URL = strings.TrimRight(parsedURL.String(), "/")
	}

	// Check if child already exists
	if existing, err := s.db.GetChild(nodeID); err == nil && existing != nil {
		s.respondError(w, http.StatusConflict, "child already registered: "+nodeID)
		return
	}

	// Generate DEK for child
	childDEK, err := crypto.GenerateKey()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate child DEK")
		return
	}

	// Encrypt child's DEK with parent's DEK (for parent's records)
	parentDEK, err := s.getLocalDEK()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get parent DEK")
		return
	}
	encryptedChildDEK, childNonce, err := crypto.Encrypt(parentDEK, childDEK)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to encrypt child DEK")
		return
	}

	child := &db.Child{
		NodeID:       nodeID,
		Label:        req.Label,
		URL:          req.URL,
		EncryptedDEK: encryptedChildDEK,
		Nonce:        childNonce,
		Version:      1,
	}
	if err := s.db.RegisterChild(child); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to register child: "+err.Error())
		return
	}

	log.Printf("Registered child node: %s (%s)", nodeID, req.Label)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"dek":     childDEK,
		"version": 1,
	})
}
