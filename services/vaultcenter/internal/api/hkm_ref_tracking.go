package api

import (
	"fmt"
	"net/http"
	"strings"
	"veilkey-vaultcenter/internal/db"
)

func parseCanonicalRef(ref string) (db.RefParts, error) {
	return db.ParseCanonicalRef(strings.TrimSpace(ref))
}

func normalizeScopeStatus(family string, scope string, status string, fallbackScope string) (string, string, error) {
	return db.NormalizeRefState(family, scope, status, fallbackScope)
}

func (s *Server) upsertTrackedRef(ref string, version int, status string, agentHash string) error {
	return s.upsertTrackedRefNamed(ref, version, status, agentHash, "")
}

func (s *Server) upsertTrackedRefNamed(ref string, version int, status string, agentHash string, secretName string) error {
	parts, err := parseCanonicalRef(ref)
	if err != nil {
		return err
	}
	if version == 0 {
		version = 1
	}
	parts.Scope, status, err = normalizeScopeStatus(parts.Family, parts.Scope, status, "")
	if err != nil {
		return err
	}
	ref = parts.Canonical()
	if existing, err := s.db.GetRef(ref); err == nil && existing != nil {
		if existing.AgentHash != "" && agentHash != "" && existing.AgentHash != agentHash {
			return fmt.Errorf("ref %s belongs to different agent", ref)
		}
		return s.db.UpdateRefWithName(ref, ref, version, status, secretName)
	}
	return s.db.SaveRefWithName(parts, ref, version, status, agentHash, secretName)
}

func (s *Server) resolveTrackedRefVersion(ref string, previousRef string, version int) int {
	if version > 0 {
		return version
	}
	if existing, err := s.db.GetRef(ref); err == nil && existing != nil && existing.Version > 0 {
		return existing.Version
	}
	if previousRef != "" {
		if previous, err := s.db.GetRef(previousRef); err == nil && previous != nil && previous.Version > 0 {
			return previous.Version
		}
	}
	return 1
}

func (s *Server) syncTrackedRef(ref string, previousRef string, version int, status string, agentHash string) error {
	resolvedVersion := s.resolveTrackedRefVersion(ref, previousRef, version)
	if err := s.upsertTrackedRef(ref, resolvedVersion, status, agentHash); err != nil {
		return err
	}
	if previousRef != "" && previousRef != ref {
		if err := s.db.CarrySecretCatalogIdentity(previousRef, ref); err != nil {
			return err
		}
	}
	s.saveAuditEvent(
		"tracked_ref",
		ref,
		"sync",
		"agent",
		agentHash,
		"",
		"tracked_refs_sync",
		map[string]any{
			"previous_ref": previousRef,
		},
		map[string]any{
			"ref":        ref,
			"version":    resolvedVersion,
			"status":     status,
			"agent_hash": agentHash,
		},
	)
	if previousRef != "" && previousRef != ref {
		previous, err := s.db.GetRef(previousRef)
		if err == nil && previous.AgentHash != "" && agentHash != "" && previous.AgentHash != agentHash {
			return fmt.Errorf("previous ref %s belongs to different agent", previousRef)
		}
		if err := s.deleteTrackedRef(previousRef); err != nil {
			return err
		}
		s.saveAuditEvent(
			"tracked_ref",
			previousRef,
			"delete",
			"agent",
			agentHash,
			"replaced_by_new_ref",
			"tracked_refs_sync",
			map[string]any{
				"ref": previousRef,
			},
			map[string]any{
				"replaced_by": ref,
			},
		)
	}
	return nil
}

func (s *Server) deleteTrackedRef(ref string) error {
	if _, err := s.db.GetRef(ref); err != nil {
		return err
	}
	return s.db.DeleteRef(ref)
}

func (s *Server) handleTrackedRefSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultNodeUUID    string `json:"vault_node_uuid"`
		NodeID           string `json:"node_id"`
		VaultRuntimeHash string `json:"vault_runtime_hash"`
		Agent            string `json:"agent"`
		Ref              string `json:"ref"`
		PreviousRef      string `json:"previous_ref"`
		Version          int    `json:"version"`
		Status           string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ref == "" {
		s.respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	if parts, err := parseCanonicalRef(req.Ref); err == nil {
		_, req.Status, _ = normalizeScopeStatus(parts.Family, parts.Scope, req.Status, "")
	}
	var (
		agent *agentInfo
		err   error
	)
	nodeID := strings.TrimSpace(req.VaultNodeUUID)
	if nodeID == "" {
		nodeID = strings.TrimSpace(req.NodeID)
	}
	if nodeID != "" {
		record, recordErr := s.db.GetAgentByNodeID(nodeID)
		if recordErr != nil {
			s.respondError(w, http.StatusBadRequest, recordErr.Error())
			return
		}
		if availErr := validateAgentAvailability(record); availErr != nil {
			s.respondAgentLookupError(w, availErr)
			return
		}
		agent = agentToInfo(record)
	} else {
		target := req.VaultRuntimeHash
		if target == "" {
			target = req.Agent
		}
		if target == "" {
			s.respondError(w, http.StatusBadRequest, "vault_node_uuid (or node_id) or vault_runtime_hash is required")
			return
		}
		agent, err = s.findAgent(target)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.syncTrackedRef(req.Ref, req.PreviousRef, req.Version, req.Status, agent.AgentHash); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to sync tracked ref: "+err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"node_id":            agent.NodeID,
		"vault_node_uuid":    agent.NodeID,
		"vault_runtime_hash": agent.AgentHash,
		"agent_hash":         agent.AgentHash,
		"status":             "ok",
		"ref":                req.Ref,
		"previous_ref":       req.PreviousRef,
		"version":            s.resolveTrackedRefVersion(req.Ref, req.PreviousRef, req.Version),
		"lifecycle":          req.Status,
	})
}
