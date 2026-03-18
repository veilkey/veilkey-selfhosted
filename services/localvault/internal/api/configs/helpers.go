package configs

import (
	"net/http"

	"veilkey-localvault/internal/db"
	"github.com/veilkey/veilkey-go-package/httputil"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}

func isValidResourceName(name string) bool {
	return httputil.IsValidResourceName(name)
}

// veRef constructs a VE ref string.
func veRef(scope db.RefScope, key string) string { return makeRef(refFamilyVE, scope, key) }
