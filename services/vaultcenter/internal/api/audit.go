package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
	"veilkey-vaultcenter/internal/db"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func (s *Server) saveAuditEvent(entityType, entityID, action, actorType, actorID, reason, source string, before, after map[string]any) {
	beforeJSON := "{}"
	if len(before) > 0 {
		if data, err := json.Marshal(before); err == nil {
			beforeJSON = string(data)
		}
	}

	afterJSON := "{}"
	if len(after) > 0 {
		if data, err := json.Marshal(after); err == nil {
			afterJSON = string(data)
		}
	}

	_ = s.db.SaveAuditEvent(&db.AuditEvent{
		EventID:    crypto.GenerateUUID(),
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		ActorType:  actorType,
		ActorID:    actorID,
		Reason:     reason,
		Source:     source,
		BeforeJSON: beforeJSON,
		AfterJSON:  afterJSON,
		CreatedAt:  time.Now().UTC(),
	})
}

func actorIDForRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return normalizeActorRemoteAddr(r.RemoteAddr)
}

func normalizeActorRemoteAddr(remote string) string {
	raw := strings.TrimSpace(remote)
	if raw == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(raw)
	if err == nil && host != "" {
		return host
	}
	return raw
}
