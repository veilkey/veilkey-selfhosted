package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	agentapi "github.com/veilkey/veilkey-go-package/agentapi"
	sharedhttp "github.com/veilkey/veilkey-go-package/httputil"
	"github.com/veilkey/veilkey-go-package/netutil"
)

// Re-exports from shared package — use via this package to avoid double-import churn.

func JoinPath(base string, elem ...string) string { return sharedhttp.JoinPath(base, elem...) }
func IsValidResourceName(name string) bool        { return sharedhttp.IsValidResourceName(name) }
func RespondJSON(w http.ResponseWriter, status int, data any) {
	sharedhttp.RespondJSON(w, status, data)
}
func RespondError(w http.ResponseWriter, status int, message string) {
	sharedhttp.RespondError(w, status, message)
}
func DecodeJSON(r *http.Request, dst any) error  { return sharedhttp.DecodeJSON(r, dst) }
func PathVal(r *http.Request, key string) string { return sharedhttp.PathVal(r, key) }

const MaxBulkItems = sharedhttp.MaxBulkItems
const ContentTypeJSON = sharedhttp.ContentTypeJSON
const HeaderCFConnectingIP = sharedhttp.HeaderCFConnectingIP
const HeaderXRealIP = sharedhttp.HeaderXRealIP
const HeaderXForwardedFor = sharedhttp.HeaderXForwardedFor

// Agent API path constants — re-exported from agentapi for backward compatibility.
const (
	AgentPathConfigs      = agentapi.PathConfigs
	AgentPathConfigsBulk  = agentapi.PathConfigsBulk
	AgentPathSecrets      = agentapi.PathSecrets
	AgentPathSecretFields = agentapi.PathSecretFields
	AgentPathCipher       = agentapi.PathCipher
	AgentPathResolve      = agentapi.PathResolve
	AgentPathRekey        = agentapi.PathRekey
)

// AgentScheme returns the URL scheme for agent communication.
func AgentScheme() string {
	if scheme := os.Getenv("VEILKEY_AGENT_SCHEME"); scheme != "" {
		return scheme
	}
	if os.Getenv("VEILKEY_TLS_CERT") != "" {
		return "https"
	}
	return "http"
}

func RequestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func EncodeStringList(items []string) string {
	b, _ := json.Marshal(items)
	return string(b)
}

func DecodeStringList(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func ActorIDForRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return netutil.NormalizeRemoteAddr(r.RemoteAddr)
}

// ParseListWindow parses limit/offset query params. Returns errMsg if invalid.
func ParseListWindow(r *http.Request) (limit, offset int, errMsg string) {
	limit = 50
	offset = 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if v, err := parsePositiveInt(raw); err != nil {
			return 0, 0, "invalid limit"
		} else if v > 500 {
			return 0, 0, "limit must be <= 500"
		} else {
			limit = v
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if v, err := parsePositiveInt(raw); err != nil {
			return 0, 0, "invalid offset"
		} else {
			offset = v
		}
	}
	return limit, offset, ""
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not an integer")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// FormatVaultID returns "name:hash8" identifying a vault.
func FormatVaultID(name, hash string) string {
	return netutil.FormatVaultID(name, hash)
}
