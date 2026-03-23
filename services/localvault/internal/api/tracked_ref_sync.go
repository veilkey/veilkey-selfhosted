package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/httputil"
)

type trackedRefSyncResult struct {
	Status   string   `json:"status"`
	URL      string   `json:"url,omitempty"`
	Source   string   `json:"source,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
}

func (s *Server) syncTrackedRefWithVaultcenter(ref string, previousRef string, version int, status db.RefStatus) trackedRefSyncResult {
	target := s.resolveVaultcenterTarget()
	result := trackedRefSyncResult{
		Status:   "skipped",
		URL:      target.URL,
		Source:   target.Source,
		Warnings: target.Warnings,
	}
	if target.URL == "" || s.identity == nil || strings.TrimSpace(s.identity.NodeID) == "" {
		return result
	}

	body, err := json.Marshal(map[string]interface{}{
		"vault_node_uuid": strings.TrimSpace(s.identity.NodeID),
		"node_id":         strings.TrimSpace(s.identity.NodeID),
		"ref":             ref,
		"previous_ref":    previousRef,
		"version":         version,
		"status":          status,
	})
	if err != nil {
		result.Status = "degraded"
		result.Error = err.Error()
		return result
	}

	syncReq, reqErr := http.NewRequest(http.MethodPost, target.URL+"/api/tracked-refs/sync", bytes.NewReader(body))
	if reqErr != nil {
		result.Status = "degraded"
		result.Error = reqErr.Error()
		return result
	}
	syncReq.Header.Set("Content-Type", httputil.ContentTypeJSON)
	if auth := s.agentAuthHeader(); auth != "" {
		syncReq.Header.Set("Authorization", auth)
	}
	resp, err := s.httpClient.Do(syncReq)
	if err != nil {
		result.Status = "degraded"
		result.Error = err.Error()
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		payload, err := io.ReadAll(resp.Body)
		result.Status = "degraded"
		if err != nil {
			result.Error = fmt.Sprintf("tracked ref sync rejected: status=%d (failed to read body: %v)", resp.StatusCode, err)
		} else {
			result.Error = fmt.Sprintf("tracked ref sync rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
		}
		return result
	}
	result.Status = "ok"
	return result
}
