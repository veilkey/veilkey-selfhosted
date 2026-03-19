package hkm

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"github.com/veilkey/veilkey-go-package/crypto"
)

// handleFederatedRotate triggers key rotation across all children from root
// 1. Generate new DEK for each child
// 2. Push new DEK to child via POST /api/rekey
// 3. Update parent's record of child's encrypted DEK
func (h *Handler) handleFederatedRotate(w http.ResponseWriter, r *http.Request) {
	children, err := h.deps.DB().ListChildren()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	parentDEK, err := h.getLocalDEK()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get parent DEK")
		return
	}

	client := &http.Client{Timeout: h.deps.DeployTimeout()}

	type rotateResult struct {
		NodeID         string `json:"node_id"`
		Label          string `json:"label"`
		Status         string `json:"status"`
		OldVersion     int    `json:"old_version"`
		NewVersion     int    `json:"new_version"`
		SecretsUpdated int    `json:"secrets_updated,omitempty"`
		Error          string `json:"error,omitempty"`
	}
	var results []rotateResult
	successCount := 0
	failCount := 0

	for i := range children {
		child := &children[i]
		if child.URL == "" {
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "skipped", Error: "no URL registered",
			})
			failCount++
			continue
		}

		newVersion := child.Version + 1

		newChildDEK, err := crypto.GenerateKey()
		if err != nil {
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "error", Error: "failed to generate DEK",
			})
			failCount++
			continue
		}

		payload, err := json.Marshal(map[string]interface{}{
			"dek":     newChildDEK,
			"version": newVersion,
		})
		if err != nil {
			log.Printf("federation: failed to marshal rekey payload for %s: %v", child.NodeID, err)
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "error", Error: "marshal failed",
			})
			failCount++
			continue
		}
		resp, err := client.Post(child.URL+agentPathRekey, "application/json", bytes.NewReader(payload))
		if err != nil {
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "error", Error: "rekey request failed: " + err.Error(),
			})
			failCount++
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("federation: failed to read rekey response from %s: %v", child.NodeID, err)
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "error", Error: "read response failed",
			})
			failCount++
			continue
		}

		if resp.StatusCode != http.StatusOK {
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "error", Error: "rekey failed: " + string(body),
			})
			failCount++
			continue
		}

		var rekeyResp struct {
			SecretsUpdated int `json:"secrets_updated"`
		}
		if err := json.Unmarshal(body, &rekeyResp); err != nil {
			log.Printf("federation: failed to parse rekey response from %s: %v", child.NodeID, err)
		}

		encChildDEK, childNonce, err := crypto.Encrypt(parentDEK, newChildDEK)
		if err != nil {
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "partial", OldVersion: child.Version, NewVersion: newVersion,
				SecretsUpdated: rekeyResp.SecretsUpdated,
				Error:          "child rekeyed but parent record update failed",
			})
			failCount++
			continue
		}
		if err := h.deps.DB().UpdateChildDEK(child.NodeID, encChildDEK, childNonce, newVersion); err != nil {
			results = append(results, rotateResult{
				NodeID: child.NodeID, Label: child.Label,
				Status: "partial", OldVersion: child.Version, NewVersion: newVersion,
				SecretsUpdated: rekeyResp.SecretsUpdated,
				Error:          "child rekeyed but parent record update failed: " + err.Error(),
			})
			failCount++
			continue
		}

		results = append(results, rotateResult{
			NodeID: child.NodeID, Label: child.Label,
			Status: "ok", OldVersion: child.Version, NewVersion: newVersion,
			SecretsUpdated: rekeyResp.SecretsUpdated,
		})
		successCount++
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"success": successCount,
		"failed":  failCount,
		"total":   len(children),
	})
}
