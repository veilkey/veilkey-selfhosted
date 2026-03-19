package secrets

import (
	"net/http"

	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/httputil"
)

const vaultcenterOnlyDecryptMessage = "localvault direct plaintext handling is disabled; use vaultcenter"

func respondJSON(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}

func isValidResourceName(name string) bool {
	return httputil.IsValidResourceName(name)
}

// vkRef constructs a VK ref string.
func vkRef(scope db.RefScope, id string) string { return makeRef(refFamilyVK, scope, id) }

