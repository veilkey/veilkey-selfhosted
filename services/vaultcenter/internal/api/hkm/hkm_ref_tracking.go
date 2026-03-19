package hkm

import (
	"fmt"
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"strings"
	"veilkey-vaultcenter/internal/db"
)

func parseCanonicalRef(ref string) (db.RefParts, error) {
	return db.ParseCanonicalRef(strings.TrimSpace(ref))
}

func normalizeScopeStatus(family string, scope db.RefScope, status db.RefStatus, fallbackScope db.RefScope) (db.RefScope, db.RefStatus, error) {
	if scope == "" {
		scope = fallbackScope
	}
	if scope == "" {
		scope = db.RefScopeTemp
	}
	switch scope {
	case db.RefScopeLocal, db.RefScopeExternal:
		if status == "" {
			status = db.RefStatusActive
		}
	case db.RefScopeTemp:
		if status == "" {
			status = db.RefStatusTemp
		}
	default:
		return "", "", fmt.Errorf("unsupported %s scope: %s", family, scope)
	}
	return scope, status, nil
}

func (h *Handler) upsertTrackedRef(ref string, version int, status db.RefStatus, agentHash string) error {
	return h.upsertTrackedRefNamed(ref, version, status, agentHash, "")
}

func (h *Handler) upsertTrackedRefNamed(ref string, version int, status db.RefStatus, agentHash string, secretName string) error {
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
	if existing, err := h.deps.DB().GetRef(ref); err == nil && existing != nil {
		if existing.AgentHash != "" && agentHash != "" && existing.AgentHash != agentHash {
			return fmt.Errorf("ref %s belongs to different agent", ref)
		}
		return h.deps.DB().UpdateRefWithName(ref, ref, version, status, secretName)
	}
	return h.deps.DB().SaveRefWithName(parts, ref, version, status, agentHash, secretName)
}

func (h *Handler) resolveTrackedRefVersion(ref string, previousRef string, version int) int {
	if version > 0 {
		return version
	}
	if existing, err := h.deps.DB().GetRef(ref); err == nil && existing != nil && existing.Version > 0 {
		return existing.Version
	}
	if previousRef != "" {
		if previous, err := h.deps.DB().GetRef(previousRef); err == nil && previous != nil && previous.Version > 0 {
			return previous.Version
		}
	}
	return 1
}

func (h *Handler) syncTrackedRef(ref string, previousRef string, version int, status db.RefStatus, agentHash string) error {
	resolvedVersion := h.resolveTrackedRefVersion(ref, previousRef, version)
	if err := h.upsertTrackedRef(ref, resolvedVersion, status, agentHash); err != nil {
		return err
	}
	if previousRef != "" && previousRef != ref {
		if err := h.deps.DB().CarrySecretCatalogIdentity(previousRef, ref); err != nil {
			return err
		}
	}
	h.deps.SaveAuditEvent(
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
		previous, err := h.deps.DB().GetRef(previousRef)
		if err == nil && previous.AgentHash != "" && agentHash != "" && previous.AgentHash != agentHash {
			return fmt.Errorf("previous ref %s belongs to different agent", previousRef)
		}
		if err := h.deleteTrackedRef(previousRef); err != nil {
			return err
		}
		h.deps.SaveAuditEvent(
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

func (h *Handler) deleteTrackedRef(ref string) error {
	if _, err := h.deps.DB().GetRef(ref); err != nil {
		return err
	}
	return h.deps.DB().DeleteRef(ref)
}

// NormalizeScopeStatus is the exported wrapper for normalizeScopeStatus,
// used by the api package for local config ref tracking.
func NormalizeScopeStatus(family string, scope db.RefScope, status db.RefStatus, fallbackScope db.RefScope) (db.RefScope, db.RefStatus, error) {
	return normalizeScopeStatus(family, scope, status, fallbackScope)
}

// UpsertTrackedRef is the exported wrapper for upsertTrackedRef,
// used by the api package for local config ref tracking.
func (h *Handler) UpsertTrackedRef(ref string, version int, status db.RefStatus, agentHash string) error {
	return h.upsertTrackedRef(ref, version, status, agentHash)
}

// DeleteTrackedRef is the exported wrapper for deleteTrackedRef,
// used by the api package for local config ref tracking.
func (h *Handler) DeleteTrackedRef(ref string) error {
	return h.deleteTrackedRef(ref)
}

func (h *Handler) handleTrackedRefSync(w http.ResponseWriter, r *http.Request) {
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
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ref == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	var resolvedStatus db.RefStatus
	if parts, err := parseCanonicalRef(req.Ref); err == nil {
		_, resolvedStatus, _ = normalizeScopeStatus(parts.Family, parts.Scope, db.RefStatus(req.Status), "")
	} else {
		resolvedStatus = db.RefStatus(req.Status)
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
		record, recordErr := h.deps.DB().GetAgentByNodeID(nodeID)
		if recordErr != nil {
			respondError(w, http.StatusBadRequest, recordErr.Error())
			return
		}
		if availErr := validateAgentAvailability(record); availErr != nil {
			h.respondAgentLookupError(w, availErr)
			return
		}
		agent = agentToInfo(record)
	} else {
		target := req.VaultRuntimeHash
		if target == "" {
			target = req.Agent
		}
		if target == "" {
			respondError(w, http.StatusBadRequest, "vault_node_uuid (or node_id) or vault_runtime_hash is required")
			return
		}
		agent, err = h.findAgent(target)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.syncTrackedRef(req.Ref, req.PreviousRef, req.Version, resolvedStatus, agent.AgentHash); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to sync tracked ref: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"node_id":            agent.NodeID,
		"vault_node_uuid":    agent.NodeID,
		"vault_runtime_hash": agent.AgentHash,
		"agent_hash":         agent.AgentHash,
		"status":             "ok",
		"ref":                req.Ref,
		"previous_ref":       req.PreviousRef,
		"version":            h.resolveTrackedRefVersion(req.Ref, req.PreviousRef, req.Version),
		"lifecycle":          resolvedStatus,
	})
}
