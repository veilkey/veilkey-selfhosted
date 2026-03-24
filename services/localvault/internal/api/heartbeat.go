package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/httputil"
)

// ErrRotationRequired is returned by SendHeartbeatOnce when the hub requires a key rotation.
var ErrRotationRequired = errors.New("rotation_required")

func (s *Server) StartHeartbeat(hubURL, label string, port int, interval time.Duration) {
	if hubURL == "" {
		log.Println("VEILKEY_VAULTCENTER_URL not set, heartbeat disabled")
		return
	}

	go func() {
		endpoint := hubURL + "/api/agents/heartbeat"
		log.Printf("Heartbeat started: %s every %s", endpoint, interval)

		// 시작 직후 첫 heartbeat
		if err := s.SendHeartbeatOnce(endpoint, label, port); err != nil {
			log.Printf("Heartbeat failed: %v", err)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.SendHeartbeatOnce(endpoint, label, port); err != nil {
				log.Printf("Heartbeat failed: %v", err)
			}
		}
	}()
}

func (s *Server) SendHeartbeatOnce(endpoint, label string, port int) error {
	// Skip heartbeat if server is locked (DB not available yet)
	if s.IsLocked() || s.db == nil {
		return fmt.Errorf("server is locked, skipping heartbeat")
	}

	secretsCount := 0
	version := 0
	nodeID := ""

	if info, err := s.db.GetNodeInfo(); err == nil {
		nodeID = info.NodeID
		version = info.Version
		if s.identity != nil {
			s.identity.NodeID = info.NodeID
			s.identity.Version = info.Version
		}
	} else if s.identity != nil {
		nodeID = s.identity.NodeID
		version = s.identity.Version
	}
	configsCount := 0
	if !s.IsLocked() {
		if count, err := s.db.CountSecrets(); err == nil {
			secretsCount = count
		}
	}
	if count, err := s.db.CountConfigs(); err == nil {
		configsCount = count
	}

	payload := map[string]interface{}{
		"vault_node_uuid": nodeID,
		"node_id":         nodeID,
		"vault_hash":      s.identity.VaultHash,
		"vault_name":      s.identity.VaultName,
		"vault_id":        formatVaultID(s.identity.VaultName, s.identity.VaultHash),
		"managed_paths":   resolveManagedPaths(),
		"key_version":     version,
		"label":           label,
		"port":            port,
		"secrets_count":   secretsCount,
		"configs_count":   configsCount,
		"version":         version,
	}
	// Include registration token for first-time registration
	if regToken, err := s.db.GetConfig("VEILKEY_REGISTRATION_TOKEN"); err == nil && regToken != nil && regToken.Value != "" {
		payload["registration_token"] = regToken.Value
	}
	// Include vault_unlock_key for VC-managed unlock (sent once, cleared after VC stores it)
	if vuk := s.VaultUnlockKey(); vuk != "" {
		payload["vault_unlock_key"] = vuk
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("heartbeat marshal failed: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("heartbeat: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", httputil.ContentTypeJSON)
	if auth := s.agentAuthHeader(); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("heartbeat: failed to read response body: %w", readErr)
	}

	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusConflict {
			var payload struct {
				Status             string `json:"status"`
				ExpectedKeyVersion int    `json:"expected_key_version"`
			}
			if json.Unmarshal(respBody, &payload) == nil && payload.Status == "rotation_required" && payload.ExpectedKeyVersion > 0 {
				if err := s.db.UpdateNodeVersion(payload.ExpectedKeyVersion); err != nil {
					return fmt.Errorf("heartbeat rotation update failed: %w", err)
				}
				if s.identity != nil {
					s.identity.Version = payload.ExpectedKeyVersion
				}
				return ErrRotationRequired
			}
		}
		return fmt.Errorf("heartbeat rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// On successful registration, consume the one-time registration token
	var hbResp struct {
		Status      string `json:"status"`
		AgentSecret string `json:"agent_secret"`
	}
	if json.Unmarshal(respBody, &hbResp) == nil {
		// Store agent_secret if provided (first-time registration or upgrade)
		if hbResp.AgentSecret != "" && !s.IsLocked() {
			kek := s.GetKEK()
			encrypted, nonce, encErr := crypto.Encrypt(kek, []byte(hbResp.AgentSecret))
			if encErr != nil {
				log.Printf("heartbeat: failed to encrypt agent_secret: %v", encErr)
			} else if err := s.db.UpdateAgentSecret(encrypted, nonce); err != nil {
				log.Printf("heartbeat: failed to store agent_secret: %v", err)
			} else {
				log.Println("heartbeat: agent_secret stored successfully (DB)")
				s.invalidateAgentAuthCache()
			}
			// Also store to file for auto-unlock on next restart
			if err := s.WriteAgentSecretFile(hbResp.AgentSecret); err != nil {
				log.Printf("heartbeat: failed to write agent_secret file: %v", err)
			} else {
				log.Println("heartbeat: agent_secret stored to file")
				// Delete vault_key bootstrap file — no longer needed
				vaultKeyFile := filepath.Join(s.dataDir, "vault_key")
				if err := os.Remove(vaultKeyFile); err == nil {
					log.Println("heartbeat: vault_key bootstrap file deleted")
				}
			}
		}
		// Clear vault_unlock_key from memory after successful registration
		// (VC has stored it, no need to resend)
		if hbResp.Status == "registered" || hbResp.Status == "ok" {
			if s.VaultUnlockKey() != "" {
				s.ClearVaultUnlockKey()
				log.Println("heartbeat: vault_unlock_key cleared (VC has stored it)")
			}
		}
		if hbResp.Status == "registered" {
			if err := s.db.DeleteConfig("VEILKEY_REGISTRATION_TOKEN"); err != nil {
				log.Printf("heartbeat: failed to delete registration token config: %v", err)
			} else {
				log.Println("heartbeat: registration token consumed (one-time use)")
			}
		}
	}
	return nil
}

func resolveManagedPaths() []string {
	raw := strings.TrimSpace(os.Getenv("VEILKEY_MANAGED_PATHS"))
	if raw != "" {
		return normalizeManagedPathList(strings.Split(raw, ","))
	}
	if fromContext := resolveManagedPathsFromContextFile(); len(fromContext) > 0 {
		return fromContext
	}
	return nil
}

func resolveManagedPathsFromContextFile() []string {
	contextFile := strings.TrimSpace(os.Getenv("VEILKEY_CONTEXT_FILE"))
	if contextFile == "" {
		contextDir := strings.TrimSpace(os.Getenv("VEILKEY_CONTEXT_DIR"))
		if contextDir != "" {
			contextFile = filepath.Join(contextDir, ".veilkey", "context.json")
		}
	}
	if contextFile == "" {
		return nil
	}
	data, err := os.ReadFile(contextFile)
	if err != nil {
		return nil
	}
	var payload struct {
		ManagedPath  string   `json:"managed_path"`
		ManagedPaths []string `json:"managed_paths"`
		OSPath       string   `json:"os_path"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	paths := make([]string, 0, 1+len(payload.ManagedPaths))
	if payload.ManagedPath != "" {
		paths = append(paths, payload.ManagedPath)
	}
	paths = append(paths, payload.ManagedPaths...)
	if payload.OSPath != "" {
		paths = append(paths, payload.OSPath)
	}
	return normalizeManagedPathList(paths)
}

func normalizeManagedPathList(parts []string) []string {
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || !filepath.IsAbs(part) {
			continue
		}
		part = filepath.Clean(part)
		if part == "." || part == "/" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}
