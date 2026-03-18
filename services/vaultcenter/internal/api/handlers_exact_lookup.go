package api

import (
	"net/http"
	"strings"

	"github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-vaultcenter/internal/db"
)

type exactLookupMatch struct {
	Ref        string `json:"ref"`
	Family     string `json:"family"`
	Scope      string `json:"scope"`
	ID         string `json:"id"`
	SecretName string `json:"secret_name,omitempty"`
	Status     string `json:"status,omitempty"`
}

func (s *Server) handleExactLookup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Plaintext string `json:"plaintext"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	plaintext := strings.TrimSpace(req.Plaintext)
	if plaintext == "" {
		s.respondError(w, http.StatusBadRequest, "plaintext is required")
		return
	}

	matches, err := s.exactLookupHostRefs(plaintext)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "exact lookup failed")
		return
	}

	s.saveAuditEvent("token_ref", "exact_lookup", "exact_lookup", "api", actorIDForRequest(r), "", "api", nil, map[string]any{
		"count": len(matches),
	})

	s.respondJSON(w, http.StatusOK, map[string]any{
		"matches": matches,
		"count":   len(matches),
	})
}

func (s *Server) exactLookupHostRefs(plaintext string) ([]exactLookupMatch, error) {
	refs, err := s.db.ListRefs()
	if err != nil {
		return nil, err
	}

	matches := make([]exactLookupMatch, 0)
	for i := range refs {
		tracked := refs[i]
		if tracked.AgentHash != "" {
			continue
		}

		resolved, ok, err := s.resolveLookupCandidate(&tracked)
		if err != nil {
			return nil, err
		}
		if !ok || resolved != plaintext {
			continue
		}

		matches = append(matches, exactLookupMatch{
			Ref:        tracked.RefCanonical,
			Family:     tracked.RefFamily,
			Scope:      string(tracked.RefScope),
			ID:         tracked.RefID,
			SecretName: tracked.SecretName,
			Status:     string(tracked.Status),
		})
	}

	return matches, nil
}

// resolveHostSecretInline decrypts a locally-stored (non-agent) secret and
// returns its plaintext value. It mirrors hkm.Handler.resolveHostTrackedSecret.
func (s *Server) resolveHostSecretInline(tracked *db.TokenRef) (string, error) {
	if tracked == nil || tracked.AgentHash != "" {
		return "", nil
	}
	var (
		secret *db.Secret
		err    error
	)
	if tracked.SecretName != "" {
		secret, err = s.db.GetSecretByName(tracked.SecretName)
	}
	if (err != nil || secret == nil) && tracked.RefID != "" {
		secret, err = s.db.GetSecretByRef(tracked.RefID)
	}
	if err != nil || secret == nil {
		return "", err
	}
	localDEK, err := s.GetLocalDEK()
	if err != nil {
		return "", err
	}
	plaintext, err := crypto.Decrypt(localDEK, secret.Ciphertext, secret.Nonce)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *Server) resolveLookupCandidate(tracked *db.TokenRef) (string, bool, error) {
	if tracked == nil || tracked.AgentHash != "" {
		return "", false, nil
	}

	if tracked.RefScope == "TEMP" && tracked.Ciphertext != "" {
		plaintext, err := s.resolveTempRef(tracked)
		if err != nil {
			return "", false, err
		}
		return plaintext, true, nil
	}

	resolved, err := s.resolveHostSecretInline(tracked)
	if err != nil {
		return "", false, err
	}
	if resolved == "" {
		return "", false, nil
	}
	return resolved, true, nil
}
