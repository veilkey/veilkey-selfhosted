package hkm

import (
	"net/http"
	"sort"

	"veilkey-vaultcenter/internal/db"
)

type refPolicyEntry struct {
	Family          string
	DefaultScope    db.RefScope
	AllowedScopes   []db.RefScope
	DefaultStatuses map[db.RefScope]db.RefStatus
}

var builtinRefPolicies = []refPolicyEntry{
	{
		Family:        db.RefFamilyVK,
		DefaultScope:  db.RefScopeTemp,
		AllowedScopes: []db.RefScope{db.RefScopeTemp, db.RefScopeLocal, db.RefScopeExternal},
		DefaultStatuses: map[db.RefScope]db.RefStatus{
			db.RefScopeTemp:     db.RefStatusTemp,
			db.RefScopeLocal:    db.RefStatusActive,
			db.RefScopeExternal: db.RefStatusActive,
		},
	},
	{
		Family:        db.RefFamilyVE,
		DefaultScope:  db.RefScopeTemp,
		AllowedScopes: []db.RefScope{db.RefScopeTemp, db.RefScopeLocal, db.RefScopeExternal},
		DefaultStatuses: map[db.RefScope]db.RefStatus{
			db.RefScopeTemp:     db.RefStatusTemp,
			db.RefScopeLocal:    db.RefStatusActive,
			db.RefScopeExternal: db.RefStatusActive,
		},
	},
}

func (h *Handler) handleRefPolicy(w http.ResponseWriter, r *http.Request) {
	policies := builtinRefPolicies
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].Family < policies[j].Family
	})
	resp := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		scopes := make([]string, 0, len(policy.AllowedScopes))
		defaultStatuses := map[string]string{}
		for _, scope := range policy.AllowedScopes {
			scopes = append(scopes, string(scope))
			defaultStatuses[string(scope)] = string(policy.DefaultStatuses[scope])
		}
		resp = append(resp, map[string]any{
			"family":           policy.Family,
			"default_scope":    string(policy.DefaultScope),
			"allowed_scopes":   scopes,
			"default_statuses": defaultStatuses,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"policies": resp,
		"count":    len(resp),
	})
}
