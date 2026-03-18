package httputil

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
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

var validResourceName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// IsValidResourceName reports whether name matches [A-Z_][A-Z0-9_]*.
func IsValidResourceName(name string) bool {
	return validResourceName.MatchString(name)
}

func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, map[string]string{"error": message})
}

func PathVal(r *http.Request, key string) string {
	return strings.TrimSpace(r.PathValue(key))
}

func DecodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
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
	return normalizeRemoteAddr(r.RemoteAddr)
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
	h := strings.TrimSpace(hash)
	if len(h) > 8 {
		h = h[:8]
	}
	n := strings.TrimSpace(name)
	if n == "" {
		return h
	}
	if h == "" {
		return n
	}
	return n + ":" + h
}

func normalizeRemoteAddr(remote string) string {
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
