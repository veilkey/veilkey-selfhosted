package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

	resp, err := s.httpClient.Post(target.URL+"/api/tracked-refs/sync", httputil.ContentTypeJSON, bytes.NewReader(body))
	if err != nil {
		result.Status = "degraded"
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
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
