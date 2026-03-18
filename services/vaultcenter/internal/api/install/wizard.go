package install

import "net/http"

func RenderInstallWizard(w http.ResponseWriter) {
	body, ok := embeddedInstallIndex()
	if !ok {
		http.Error(w, "install wizard UI not available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}
