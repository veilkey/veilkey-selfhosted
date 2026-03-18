package api

import (
	"net/http"
	"time"
)

type tempRefItem struct {
	RefCanonical string     `json:"ref_canonical"`
	SecretName   string     `json:"secret_name"`
	AgentHash    string     `json:"agent_hash"`
	Status       string     `json:"status"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (s *Server) handleKeycenterTempRefs(w http.ResponseWriter, r *http.Request) {
	refs, err := s.db.ListActiveTempRefs()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list temp refs")
		return
	}
	items := make([]tempRefItem, 0, len(refs))
	for _, ref := range refs {
		items = append(items, tempRefItem{
			RefCanonical: ref.RefCanonical,
			SecretName:   ref.SecretName,
			AgentHash:    ref.AgentHash,
			Status:       ref.Status,
			ExpiresAt:    ref.ExpiresAt,
			CreatedAt:    ref.CreatedAt,
		})
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"refs": items})
}
