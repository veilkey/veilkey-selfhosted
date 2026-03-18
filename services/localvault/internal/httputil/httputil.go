package httputil

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"regexp"
)

// JoinPath joins a base URL with path elements. Panics if base is not a valid URL,
// which would always indicate a programming error with a hard-coded base.
func JoinPath(base string, elem ...string) string {
	result, err := url.JoinPath(base, elem...)
	if err != nil {
		panic("httputil.JoinPath: " + err.Error())
	}
	return result
}

// RespondJSON writes a JSON response.
func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

// RespondError writes a JSON error response.
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, map[string]string{"error": message})
}

var validResourceName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// IsValidResourceName reports whether name matches [A-Z_][A-Z0-9_]*.
func IsValidResourceName(name string) bool {
	return validResourceName.MatchString(name)
}

// MaxBulkItems is the maximum number of items in a bulk operation.
const MaxBulkItems = 200
