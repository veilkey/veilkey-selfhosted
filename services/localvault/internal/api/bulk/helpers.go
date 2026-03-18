package bulk

import (
	"net/http"

	"github.com/veilkey/veilkey-go-package/httputil"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	httputil.RespondJSON(w, status, data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	httputil.RespondError(w, status, msg)
}
