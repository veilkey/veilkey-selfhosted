package api

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

var serverStartTime = time.Now().UTC()

func (s *Server) registerAdminRoutes(mux *http.ServeMux) {
	trusted := s.requireTrustedIP
	ready := s.requireUnlocked

	mux.HandleFunc("GET /api/admin/diagnostics", trusted(ready(s.handleDiagnostics)))
	mux.HandleFunc("GET /api/admin/tls-info", trusted(ready(s.handleTLSInfo)))
	mux.HandleFunc("POST /api/lock", trusted(ready(s.handleLock)))
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(serverStartTime)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"version":       os.Getenv("VEILKEY_VERSION"),
		"uptime":        formatUptime(uptime),
		"uptime_seconds": int(uptime.Seconds()),
		"go_version":    runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"goroutines":    runtime.NumGoroutine(),
		"memory_mb":     memStats.Alloc / 1024 / 1024,
		"db_path":       os.Getenv("VEILKEY_DB_PATH"),
		"addr":          os.Getenv("VEILKEY_ADDR"),
		"vault_name":    os.Getenv("VEILKEY_VAULT_NAME"),
	})
}

func (s *Server) handleTLSInfo(w http.ResponseWriter, r *http.Request) {
	certPath := os.Getenv("VEILKEY_TLS_CERT")
	if certPath == "" {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"tls_enabled": false,
		})
		return
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to read cert")
		return
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decode cert PEM")
		return
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to parse cert")
		return
	}

	var sans []string
	for _, dns := range cert.DNSNames {
		sans = append(sans, "DNS:"+dns)
	}
	for _, ip := range cert.IPAddresses {
		sans = append(sans, "IP:"+ip.String())
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"tls_enabled": true,
		"subject":     cert.Subject.CommonName,
		"issuer":      cert.Issuer.CommonName,
		"san":         sans,
		"not_before":  cert.NotBefore.UTC().Format(time.RFC3339),
		"not_after":   cert.NotAfter.UTC().Format(time.RFC3339),
		"expires_in":  formatUptime(time.Until(cert.NotAfter)),
		"self_signed": cert.Issuer.CommonName == cert.Subject.CommonName,
	})
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	s.kekMu.Lock()
	s.kek = nil
	s.locked = true
	s.kekMu.Unlock()
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "locked",
	})
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	return strings.Join(parts, "")
}

