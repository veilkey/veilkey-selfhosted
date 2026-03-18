package hkm

import (
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"slices"
)

type trackedRefCleanupAction struct {
	Reason string   `json:"reason"`
	Key    string   `json:"key"`
	Delete []string `json:"delete"`
	Keep   []string `json:"keep,omitempty"`
	Manual bool     `json:"manual"`
}

type trackedRefCleanupResponse struct {
	Status  string                    `json:"status"`
	Apply   bool                      `json:"apply"`
	Actions []trackedRefCleanupAction `json:"actions"`
	Counts  map[string]int            `json:"counts"`
}

func buildTrackedRefCleanupActions(report trackedRefAuditReport, reasonFilter map[string]bool) []trackedRefCleanupAction {
	actions := make([]trackedRefCleanupAction, 0, len(report.Stale))
	for _, issue := range report.Stale {
		if len(reasonFilter) > 0 && !reasonFilter[issue.Reason] {
			continue
		}
		action := trackedRefCleanupAction{
			Reason: issue.Reason,
			Key:    issue.Key,
		}
		switch issue.Reason {
		case "missing_owner", trackedRefAuditReasonMissingAgent:
			for _, ref := range issue.Refs {
				action.Delete = append(action.Delete, ref.RefCanonical)
			}
		case trackedRefAuditReasonDuplicateRefID:
			if len(issue.Refs) > 0 {
				action.Keep = append(action.Keep, issue.Refs[0].RefCanonical)
			}
			for _, ref := range issue.Refs[1:] {
				action.Delete = append(action.Delete, ref.RefCanonical)
			}
		default:
			action.Manual = true
			for _, ref := range issue.Refs {
				action.Keep = append(action.Keep, ref.RefCanonical)
			}
		}
		slices.Sort(action.Delete)
		slices.Sort(action.Keep)
		actions = append(actions, action)
	}
	return actions
}

// HandleTrackedRefCleanup is the exported wrapper used by the parent api
// package to mount the cleanup route under an admin-auth guard.
func (h *Handler) HandleTrackedRefCleanup(w http.ResponseWriter, r *http.Request) {
	h.handleTrackedRefCleanup(w, r)
}

func (h *Handler) handleTrackedRefCleanup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Apply   bool     `json:"apply"`
		Reasons []string `json:"reasons"`
	}
	if r.Body != nil {
		_ = httputil.DecodeJSON(r, &req)
	}

	report, err := h.loadTrackedRefAuditReport()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load tracked ref audit")
		return
	}

	reasonFilter := make(map[string]bool, len(req.Reasons))
	for _, reason := range req.Reasons {
		if reason != "" {
			reasonFilter[reason] = true
		}
	}
	actions := buildTrackedRefCleanupActions(report, reasonFilter)
	resp := trackedRefCleanupResponse{
		Status:  "preview",
		Apply:   req.Apply,
		Actions: actions,
		Counts: map[string]int{
			"actions":           len(actions),
			"delete_candidates": 0,
			"manual_actions":    0,
			"deleted":           0,
		},
	}
	for _, action := range actions {
		resp.Counts["delete_candidates"] += len(action.Delete)
		if action.Manual {
			resp.Counts["manual_actions"]++
		}
	}
	if !req.Apply {
		respondJSON(w, http.StatusOK, resp)
		return
	}

	resp.Status = "applied"
	for _, action := range actions {
		if action.Manual {
			continue
		}
		for _, ref := range action.Delete {
			if err := h.deleteTrackedRef(ref); err == nil {
				resp.Counts["deleted"]++
			}
		}
	}
	respondJSON(w, http.StatusOK, resp)
}
