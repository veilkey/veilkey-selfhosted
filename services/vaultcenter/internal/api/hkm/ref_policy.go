package hkm

import (
	"net/http"
	"slices"
	"veilkey-vaultcenter/internal/db"
)

func (h *Handler) handleRefPolicy(w http.ResponseWriter, r *http.Request) {
	policies := db.ListRefPolicies()
	resp := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		scopes := make([]string, 0, len(policy.AllowedScopes))
		defaultStatuses := map[string]string{}
		for scope, status := range policy.AllowedScopes {
			scopes = append(scopes, scope)
			defaultStatuses[scope] = status
		}
		slices.Sort(scopes)
		resp = append(resp, map[string]any{
			"family":           policy.Family,
			"default_scope":    policy.DefaultScope,
			"allowed_scopes":   scopes,
			"default_statuses": defaultStatuses,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"policies": resp,
		"count":    len(resp),
	})
}
