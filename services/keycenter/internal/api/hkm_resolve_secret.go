package api

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

// handleResolveSecret resolves a scoped VK ref to its plaintext value
// If not found locally, cascades to children (federated resolve)
// X-VeilKey-Cascade header prevents infinite loops between parent/child
func (s *Server) handleResolveSecret(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		s.respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	if strings.Contains(ref, "/") || strings.Contains(ref, "..") || strings.ContainsAny(ref, "\x00\n\r") || len(ref) > 64 {
		s.respondError(w, http.StatusBadRequest, "invalid ref format")
		return
	}

	isCascade := r.Header.Get("X-VeilKey-Cascade") == "true"

	if tracked, err := s.db.GetRef(ref); err == nil && tracked != nil {
		if s.resolveTrackedRef(w, ref, tracked) {
			return
		}
	}

	// Try direct vault-runtime-hash resolve only for raw compact tokens.
		// Canonical scoped refs like VK:{SCOPE}:{ref} must stay on tracked/local resolution.
	if !strings.Contains(ref, ":") && len(ref) > 8 {
		agentHash := ref[:8]
		secretRef := ref[8:]
		agent, agentErr := s.db.GetAgentByHash(agentHash)
		if agentErr == nil && len(agent.DEK) > 0 {
			agentDEK, dekErr := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
			if dekErr == nil {
				ai := agentToInfo(agent)
				cipher, cipherErr := s.fetchAgentCiphertext(ai.URL(), secretRef)
				if cipherErr == nil {
					plaintext, decErr := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
					if decErr == nil {
						s.respondJSON(w, http.StatusOK, map[string]interface{}{
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
		s.respondError(w, http.StatusNotFound, "ref not found locally: "+ref)
		return
	}

	// Federated resolve: try children concurrently
	children, _ := s.db.ListChildren()
	if len(children) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), s.timeouts.CascadeResolve)
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
			go func(url string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}
				req, err := http.NewRequestWithContext(ctx, "GET", url+"/api/resolve/"+ref, nil)
				if err != nil {
					log.Printf("resolve: failed to create request for %s: %v", url, err)
					return
				}
				req.Header.Set("X-VeilKey-Cascade", "true")
				resp, err := s.httpClient.Do(req)
				if err != nil || resp.StatusCode != http.StatusOK {
					if resp != nil {
						resp.Body.Close()
					}
					return
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					log.Printf("resolve: failed to read response from %s: %v", url, err)
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
	if info, err := s.db.GetNodeInfo(); err == nil && info.ParentURL != "" {
		client := &http.Client{Timeout: s.timeouts.ParentForward}
		req, err := http.NewRequest("GET", info.ParentURL+"/api/resolve/"+ref, nil)
		if err != nil {
			log.Printf("resolve: failed to create parent request: %v", err)
			s.respondError(w, http.StatusNotFound, "ref not found: "+ref)
			return
		}
		req.Header.Set("X-VeilKey-Cascade", "true")
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("resolve: failed to read parent response: %v", err)
				s.respondError(w, http.StatusNotFound, "ref not found: "+ref)
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

	s.respondError(w, http.StatusNotFound, "ref not found: "+ref)
}

func (s *Server) resolveTrackedRef(w http.ResponseWriter, ref string, tracked *db.TokenRef) bool {
	agentHash := tracked.AgentHash
	secretRef := tracked.RefID

	if agentHash != "" {
		agent, err := s.db.GetAgentByHash(agentHash)
		if err != nil || len(agent.DEK) == 0 {
			return false
		}
		agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
		if err != nil {
			return false
		}
		ai := agentToInfo(agent)
		cipher, err := s.fetchAgentCiphertext(ai.URL(), secretRef)
		if err != nil {
			return false
		}
		plaintext, err := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
		if err != nil {
			return false
		}
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"ref":                ref,
			"name":               cipher.Name,
			"value":              string(plaintext),
			"vault":              agent.Label,
			"vault_runtime_hash": agentHash,
			"agent_hash":         agentHash,
		})
		now := time.Now().UTC()
		_ = s.db.MarkSecretCatalogRevealed(ref, now)
		s.saveAuditEvent(
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
	if tracked.RefScope == "TEMP" && tracked.Ciphertext != "" && tracked.AgentHash == "" {
		if tracked.ExpiresAt != nil && time.Now().UTC().After(*tracked.ExpiresAt) {
			s.respondError(w, http.StatusGone, "temp ref expired: "+ref)
			return true
		}
		if resolved, err := s.resolveTempRef(tracked); err == nil {
			s.respondJSON(w, http.StatusOK, map[string]interface{}{
				"ref":   ref,
				"value": resolved,
			})
			return true
		}
	}

	if secret, err := s.resolveHostTrackedSecret(tracked); err == nil && secret != nil {
		now := time.Now().UTC()
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"ref":                ref,
			"name":               secret.Name,
			"value":              secret.Value,
			"vault":              "host",
			"vault_runtime_hash": "host",
			"agent_hash":         "",
		})
		_ = s.db.MarkSecretCatalogRevealed(ref, now)
		s.saveAuditEvent(
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

type resolvedHostSecret struct {
	Name  string
	Value string
}

func (s *Server) resolveHostTrackedSecret(tracked *db.TokenRef) (*resolvedHostSecret, error) {
	if tracked == nil || tracked.AgentHash != "" {
		return nil, nil
	}

	var (
		secret *db.Secret
		err    error
	)
	if tracked.SecretName != "" {
		secret, err = s.db.GetSecretByName(tracked.SecretName)
	}
	if (err != nil || secret == nil) && tracked.RefID != "" {
		secret, err = s.db.GetSecretByRef(tracked.RefID)
	}
	if err != nil || secret == nil {
		return nil, err
	}

	info, err := s.db.GetNodeInfo()
	if err != nil {
		return nil, err
	}
	localDEK, err := crypto.DecryptDEK(s.kek, info.DEK, info.DEKNonce)
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
