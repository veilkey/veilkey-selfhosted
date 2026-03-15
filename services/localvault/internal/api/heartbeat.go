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
)

func (s *Server) StartHeartbeat(hubURL, label string, port int, interval time.Duration) {
	if hubURL == "" {
		log.Println("VEILKEY_KEYCENTER_URL not set, heartbeat disabled")
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

	body, err := json.Marshal(map[string]interface{}{
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
	})
	if err != nil {
		return fmt.Errorf("heartbeat marshal failed: %w", err)
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("heartbeat rejected: status=%d (failed to read body: %v)", resp.StatusCode, err)
		}
		if resp.StatusCode == http.StatusConflict {
			var payload struct {
				Status             string `json:"status"`
				ExpectedKeyVersion int    `json:"expected_key_version"`
			}
			if json.Unmarshal(body, &payload) == nil && payload.Status == "rotation_required" && payload.ExpectedKeyVersion > 0 {
				if err := s.db.UpdateNodeVersion(payload.ExpectedKeyVersion); err != nil {
					return fmt.Errorf("heartbeat rotation update failed: %w", err)
				}
				if s.identity != nil {
					s.identity.Version = payload.ExpectedKeyVersion
				}
				return errors.New("rotation_required")
			}
		}
		return fmt.Errorf("heartbeat rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
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
