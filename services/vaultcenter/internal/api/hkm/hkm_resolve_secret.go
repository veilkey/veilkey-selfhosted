package hkm

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
)

// handleResolveSecret resolves a scoped VK ref to its plaintext value
// If not found locally, cascades to children (federated resolve)
// X-VeilKey-Cascade header prevents infinite loops between parent/child
func (h *Handler) handleResolveSecret(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	if strings.Contains(ref, "/") || strings.Contains(ref, "..") || strings.ContainsAny(ref, "\x00\n\r") || len(ref) > 64 {
		respondError(w, http.StatusBadRequest, "invalid ref format")
		return
	}

	isCascade := r.Header.Get("X-VeilKey-Cascade") == "true"

	if tracked, err := h.deps.DB().GetRef(ref); err == nil && tracked != nil {
		if h.resolveTrackedRef(w, ref, tracked) {
			return
		}
	}

	// Try direct vault-runtime-hash resolve only for raw compact tokens.
	// Canonical scoped refs like VK:{SCOPE}:{ref} must stay on tracked/local resolution.
	if !strings.Contains(ref, ":") && len(ref) > 8 {
		agentHash := ref[:8]
		secretRef := ref[8:]
		agent, agentErr := h.deps.DB().GetAgentByHash(agentHash)
		if agentErr == nil && len(agent.DEK) > 0 {
			agentDEK, dekErr := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
			if dekErr == nil {
				ai := agentToInfo(agent)
				cipher, cipherErr := h.fetchAgentCiphertext(ai.URL(), secretRef)
				if cipherErr == nil {
					plaintext, decErr := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
					if decErr == nil {
						respondJSON(w, http.StatusOK, map[string]interface{}{
							"ref":   ref,
							"name":  cipher.Name,
							"value": string(plaintext),
						})
						return
					}
				}
			}
		}
	}

	if isCascade {
		respondError(w, http.StatusNotFound, "ref not found locally: "+ref)
		return
	}

	// Federated resolve: try children concurrently
	children, _ := h.deps.DB().ListChildren()
	if len(children) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), h.deps.CascadeResolveTimeout())
		defer cancel()

		type resolveResult struct {
			body []byte
		}
		resultCh := make(chan resolveResult, 1)
		sem := make(chan struct{}, 10)
		var wg sync.WaitGroup

		for i := range children {
			child := &children[i]
			if child.URL == "" {
				continue
			}
			wg.Add(1)
			go func(childURL string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}
				req, err := http.NewRequestWithContext(ctx, "GET", childURL+"/api/resolve/"+url.PathEscape(ref), nil)
				if err != nil {
					log.Printf("resolve: failed to create request for %s: %v", childURL, err)
					return
				}
				req.Header.Set("X-VeilKey-Cascade", "true")
				resp, err := h.deps.HTTPClient().Do(req)
				if err != nil || resp.StatusCode != http.StatusOK {
					if resp != nil {
						resp.Body.Close()
					}
					return
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					log.Printf("resolve: failed to read response from %s: %v", childURL, err)
					return
				}
				select {
				case resultCh <- resolveResult{body: body}:
					cancel()
				default:
				}
			}(child.URL)
		}

		go func() {
			wg.Wait()
			close(resultCh)
		}()

		if result, ok := <-resultCh; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(result.body)
			return
		}
	}

	// Parent resolve
	if info, err := h.deps.DB().GetNodeInfo(); err == nil && info.ParentURL != "" {
		client := &http.Client{Timeout: h.deps.ParentForwardTimeout()}
		req, err := http.NewRequest("GET", info.ParentURL+"/api/resolve/"+url.PathEscape(ref), nil)
		if err != nil {
			log.Printf("resolve: failed to create parent request: %v", err)
			respondError(w, http.StatusNotFound, "ref not found: "+ref)
			return
		}
		req.Header.Set("X-VeilKey-Cascade", "true")
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("resolve: failed to read parent response: %v", err)
				respondError(w, http.StatusNotFound, "ref not found: "+ref)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(body)
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	respondError(w, http.StatusNotFound, "ref not found: "+ref)
}

func (h *Handler) resolveTrackedRef(w http.ResponseWriter, ref string, tracked *db.TokenRef) bool {
	agentHash := tracked.AgentHash
	secretRef := tracked.RefID

	if agentHash != "" {
		agent, err := h.deps.DB().GetAgentByHash(agentHash)
		if err != nil || len(agent.DEK) == 0 {
			return false
		}
		agentDEK, err := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
		if err != nil {
			return false
		}
		ai := agentToInfo(agent)
		cipher, err := h.fetchAgentCiphertext(ai.URL(), secretRef)
		if err != nil {
			return false
		}
		plaintext, err := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
		if err != nil {
			return false
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"ref":                ref,
			"name":               cipher.Name,
			"value":              string(plaintext),
			"vault":              agent.Label,
			"vault_runtime_hash": agentHash,
			"agent_hash":         agentHash,
		})
		now := time.Now().UTC()
		_ = h.deps.DB().MarkSecretCatalogRevealed(ref, now)
		h.deps.SaveAuditEvent(
			"secret",
			ref,
			"resolve",
			"api",
			agentHash,
			"",
			"resolve",
			nil,
			map[string]any{
				"ref":                ref,
				"vault_runtime_hash": agentHash,
				"resolved_at":        now.Format(time.RFC3339),
			},
		)
		return true
	}

	// Temp encrypt refs: ciphertext stored directly in token_refs
	if tracked.RefScope == refScopeTemp && tracked.Ciphertext != "" && tracked.AgentHash == "" {
		if tracked.ExpiresAt != nil && time.Now().UTC().After(*tracked.ExpiresAt) {
			respondError(w, http.StatusGone, "temp ref expired: "+ref)
			return true
		}
		if resolved, err := h.resolveTempRef(tracked); err == nil {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"ref":   ref,
				"value": resolved,
			})
			return true
		}
	}

	if secret, err := h.resolveHostTrackedSecret(tracked); err == nil && secret != nil {
		now := time.Now().UTC()
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"ref":                ref,
			"name":               secret.Name,
			"value":              secret.Value,
			"vault":              "host",
			"vault_runtime_hash": "host",
			"agent_hash":         "",
		})
		_ = h.deps.DB().MarkSecretCatalogRevealed(ref, now)
		h.deps.SaveAuditEvent(
			"secret",
			ref,
			"resolve",
			"api",
			"",
			"",
			"resolve",
			nil,
			map[string]any{
				"ref":                ref,
				"vault_runtime_hash": "host",
				"resolved_at":        now.Format(time.RFC3339),
			},
		)
		return true
	}

	return false
}

type resolvedHostSecret = ResolvedHostSecret

// ResolvedHostSecret holds the plaintext name and value of a locally-stored secret.
type ResolvedHostSecret struct {
	Name  string
	Value string
}

// ResolveHostTrackedSecret is the exported wrapper used by the api package
// for exact-lookup resolution of non-agent tracked refs.
func (h *Handler) ResolveHostTrackedSecret(tracked *db.TokenRef) (*ResolvedHostSecret, error) {
	return h.resolveHostTrackedSecret(tracked)
}

func (h *Handler) resolveHostTrackedSecret(tracked *db.TokenRef) (*resolvedHostSecret, error) {
	if tracked == nil || tracked.AgentHash != "" {
		return nil, nil
	}

	var (
		secret *db.Secret
		err    error
	)
	if tracked.SecretName != "" {
		secret, err = h.deps.DB().GetSecretByName(tracked.SecretName)
	}
	if (err != nil || secret == nil) && tracked.RefID != "" {
		secret, err = h.deps.DB().GetSecretByRef(tracked.RefID)
	}
	if err != nil || secret == nil {
		return nil, err
	}

	info, err := h.deps.DB().GetNodeInfo()
	if err != nil {
		return nil, err
	}
	localDEK, err := crypto.DecryptDEK(h.deps.GetKEK(), info.DEK, info.DEKNonce)
	if err != nil {
		return nil, err
	}
	plaintext, err := crypto.Decrypt(localDEK, secret.Ciphertext, secret.Nonce)
	if err != nil {
		return nil, err
	}

	return &resolvedHostSecret{
		Name:  secret.Name,
		Value: string(plaintext),
	}, nil
}
