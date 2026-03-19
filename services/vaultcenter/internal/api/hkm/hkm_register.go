package hkm

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"veilkey-vaultcenter/internal/httputil"

	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/crypto"
)

// handleRegister registers a new child node and issues a DEK
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultNodeUUID string `json:"vault_node_uuid"`
		NodeID        string `json:"node_id"`
		Label         string `json:"label"`
		URL           string `json:"url"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	nodeID := req.VaultNodeUUID
	if nodeID == "" {
		nodeID = req.NodeID
	}
	if nodeID == "" {
		respondError(w, http.StatusBadRequest, "vault_node_uuid or node_id is required")
		return
	}
	if req.URL != "" {
		parsedURL, err := url.Parse(strings.TrimSpace(req.URL))
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			respondError(w, http.StatusBadRequest, "url must be a valid http(s) URL")
			return
		}
		req.URL = strings.TrimRight(parsedURL.String(), "/")
	}

	// Check if child already exists
	if existing, err := h.deps.DB().GetChild(nodeID); err == nil && existing != nil {
		respondError(w, http.StatusConflict, "child already registered: "+nodeID)
		return
	}

	// Generate DEK for child
	childDEK, err := crypto.GenerateKey()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate child DEK")
		return
	}

	// Encrypt child's DEK with parent's DEK (for parent's records)
	parentDEK, err := h.getLocalDEK()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get parent DEK")
		return
	}
	encryptedChildDEK, childNonce, err := crypto.Encrypt(parentDEK, childDEK)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to encrypt child DEK")
		return
	}

	// Record child registration on chain (identity only — no key material)
	if _, err := h.deps.SubmitTx(r.Context(), chain.TxRegisterChild, chain.RegisterChildPayload{
		NodeID:  nodeID,
		Label:   req.Label,
		URL:     req.URL,
		Version: 1,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to register child: "+err.Error())
		return
	}
	// DEK delivery via direct DB write (never on chain)
	if err := h.deps.DB().UpdateChildDEK(nodeID, encryptedChildDEK, childNonce, 1); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save child DEK: "+err.Error())
		return
	}

	log.Printf("Registered child node: %s (%s)", nodeID, req.Label)

	resp := map[string]interface{}{
		"dek":     childDEK,
		"version": 1,
	}

	// Include chain genesis + peers if chain is enabled
	if genesis, peers := h.deps.ChainInfo(); genesis != nil {
		resp["chain_genesis"] = genesis
		if peers != "" {
			resp["chain_persistent_peers"] = peers
		}
	}

	respondJSON(w, http.StatusOK, resp)
}
