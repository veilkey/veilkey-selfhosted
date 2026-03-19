package hkm

import (
	"net/http"
	"sort"
	"strings"

	"veilkey-vaultcenter/internal/db"
)

const (
	trackedRefAuditReasonMissingAgent   = "missing_agent"
	trackedRefAuditReasonDuplicateRefID = "duplicate_ref_id"
)

type trackedRefAuditEntry struct {
	RefCanonical     string `json:"ref"`
	VaultRuntimeHash string `json:"vault_runtime_hash,omitempty"`
	Family           string `json:"family"`
	Scope            string `json:"scope"`
	ID               string `json:"id"`
	Version          int    `json:"version"`
	Status           string `json:"status"`
}

type trackedRefAuditIssue struct {
	Reason string                 `json:"reason"`
	Key    string                 `json:"key"`
	Refs   []trackedRefAuditEntry `json:"refs"`
}

type trackedRefAuditReport struct {
	Blocked []trackedRefAuditEntry `json:"blocked"`
	Stale   []trackedRefAuditIssue `json:"stale"`
	Counts  map[string]int         `json:"counts"`
}

// Audit class policy:
// - blocked: immediate runtime stop or quarantine is required
// - stale: central cleanup or ownership repair is required, but runtime stop is not implied
// Add a new top-level class only when the operator action differs from these two actions.

func tokenRefToAuditEntry(ref interface {
	GetRefCanonical() string
	GetAgentHash() string
	GetRefFamily() string
	GetRefScope() string
	GetRefID() string
	GetVersion() int
	GetStatus() string
}) trackedRefAuditEntry {
	return trackedRefAuditEntry{
		RefCanonical:     ref.GetRefCanonical(),
		VaultRuntimeHash: ref.GetAgentHash(),
		Family:           ref.GetRefFamily(),
		Scope:            ref.GetRefScope(),
		ID:               ref.GetRefID(),
		Version:          ref.GetVersion(),
		Status:           ref.GetStatus(),
	}
}

type auditTokenRef struct{ trackedRefAuditEntry }

func (a auditTokenRef) GetRefCanonical() string { return a.RefCanonical }
func (a auditTokenRef) GetAgentHash() string    { return a.VaultRuntimeHash }
func (a auditTokenRef) GetRefFamily() string    { return a.Family }
func (a auditTokenRef) GetRefScope() string     { return a.Scope }
func (a auditTokenRef) GetRefID() string        { return a.ID }
func (a auditTokenRef) GetVersion() int         { return a.Version }
func (a auditTokenRef) GetStatus() string       { return a.Status }

func buildTrackedRefAudit(refs []db.TokenRef, agents []db.Agent) trackedRefAuditReport {
	agentExists := make(map[string]bool, len(agents))
	for _, agent := range agents {
		if agent.AgentHash != "" {
			agentExists[agent.AgentHash] = true
		}
	}

	blocked := make([]trackedRefAuditEntry, 0)
	stale := make([]trackedRefAuditIssue, 0)
	dups := make(map[string][]trackedRefAuditEntry)
	ownership := make(map[string][]trackedRefAuditEntry)

	for _, ref := range refs {
		entry := auditTokenRef{trackedRefAuditEntry{
			RefCanonical:     ref.RefCanonical,
			VaultRuntimeHash: ref.AgentHash,
			Family:           ref.RefFamily,
			Scope:            string(ref.RefScope),
			ID:               ref.RefID,
			Version:          ref.Version,
			Status:           string(ref.Status),
		}}
		if ref.Status == "block" {
			blocked = append(blocked, tokenRefToAuditEntry(entry))
		}
		if ref.AgentHash == "" {
			stale = append(stale, trackedRefAuditIssue{
				Reason: "missing_owner",
				Key:    ref.RefCanonical,
				Refs:   []trackedRefAuditEntry{tokenRefToAuditEntry(entry)},
			})
		}
		if ref.AgentHash != "" && !agentExists[ref.AgentHash] {
			stale = append(stale, trackedRefAuditIssue{
				Reason: trackedRefAuditReasonMissingAgent,
				Key:    ref.AgentHash,
				Refs:   []trackedRefAuditEntry{tokenRefToAuditEntry(entry)},
			})
		}
		dupKey := strings.Join([]string{ref.AgentHash, ref.RefFamily, ref.RefID}, "|")
		dups[dupKey] = append(dups[dupKey], tokenRefToAuditEntry(entry))
		ownerKey := strings.Join([]string{ref.RefFamily, ref.RefID}, "|")
		ownership[ownerKey] = append(ownership[ownerKey], tokenRefToAuditEntry(entry))
	}

	for key, entries := range dups {
		if len(entries) < 2 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool {
			return compareTrackedRefEntries(entries[i], entries[j]) < 0
		})
		stale = append(stale, trackedRefAuditIssue{
			Reason: trackedRefAuditReasonDuplicateRefID,
			Key:    key,
			Refs:   entries,
		})
	}

	for key, entries := range ownership {
		if len(entries) < 2 {
			continue
		}
		seenAgents := make(map[string]bool)
		for _, entry := range entries {
			if entry.VaultRuntimeHash == "" {
				continue
			}
			seenAgents[entry.VaultRuntimeHash] = true
		}
		if len(seenAgents) < 2 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].VaultRuntimeHash == entries[j].VaultRuntimeHash {
				return compareTrackedRefEntries(entries[i], entries[j]) < 0
			}
			return entries[i].VaultRuntimeHash < entries[j].VaultRuntimeHash
		})
		stale = append(stale, trackedRefAuditIssue{
			Reason: "agent_mismatch",
			Key:    key,
			Refs:   entries,
		})
	}

	sort.Slice(blocked, func(i, j int) bool {
		return compareTrackedRefEntries(blocked[i], blocked[j]) < 0
	})
	sort.Slice(stale, func(i, j int) bool {
		if stale[i].Reason == stale[j].Reason {
			return stale[i].Key < stale[j].Key
		}
		return stale[i].Reason < stale[j].Reason
	})

	return trackedRefAuditReport{
		Blocked: blocked,
		Stale:   stale,
		Counts: map[string]int{
			"total_refs": len(refs),
			"blocked":    len(blocked),
			"stale":      len(stale),
		},
	}
}

func compareTrackedRefEntries(a, b trackedRefAuditEntry) int {
	if a.Version != b.Version {
		if a.Version > b.Version {
			return -1
		}
		return 1
	}
	if refScopeRank(a.Scope) != refScopeRank(b.Scope) {
		if refScopeRank(a.Scope) < refScopeRank(b.Scope) {
			return -1
		}
		return 1
	}
	if refStatusRank(a.Status) != refStatusRank(b.Status) {
		if refStatusRank(a.Status) < refStatusRank(b.Status) {
			return -1
		}
		return 1
	}
	if a.RefCanonical < b.RefCanonical {
		return -1
	}
	if a.RefCanonical > b.RefCanonical {
		return 1
	}
	return 0
}

func refScopeRank(scope string) int {
	switch scope {
	case string(refScopeLocal):
		return 0
	case string(refScopeExternal):
		return 1
	case string(refScopeTemp):
		return 2
	default:
		return 9
	}
}

func refStatusRank(status string) int {
	switch status {
	case string(refStatusActive):
		return 0
	case string(refStatusTemp):
		return 1
	case string(refStatusArchive):
		return 2
	case string(refStatusRevoke):
		return 3
	case string(refStatusBlock):
		return 4
	default:
		return 9
	}
}

func (h *Handler) loadTrackedRefAuditReport() (trackedRefAuditReport, error) {
	refs, err := h.deps.DB().ListRefs()
	if err != nil {
		return trackedRefAuditReport{}, err
	}
	agents, err := h.deps.DB().ListAgents()
	if err != nil {
		return trackedRefAuditReport{}, err
	}
	return buildTrackedRefAudit(refs, agents), nil
}

func (h *Handler) handleTrackedRefAudit(w http.ResponseWriter, r *http.Request) {
	report, err := h.loadTrackedRefAuditReport()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load tracked ref audit")
		return
	}
	respondJSON(w, http.StatusOK, report)
}
