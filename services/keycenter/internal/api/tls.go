package api

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// NewTLSHTTPClient creates an HTTP client configured for TLS inter-service communication.
// If caPath is non-empty, the CA certificate is added to the trust pool.
// If insecure is true, certificate verification is skipped (for self-signed certs in dev).
func NewTLSHTTPClient(caPath string, insecure bool) (*http.Client, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if caPath != "" {
		caCert, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = pool
	}

	if insecure {
		tlsConfig.InsecureSkipVerify = true
	}

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

// InitHTTPClientFromEnv creates an HTTP client based on VEILKEY_TLS_CA and
// VEILKEY_TLS_INSECURE environment variables.
func InitHTTPClientFromEnv() *http.Client {
	caPath := os.Getenv("VEILKEY_TLS_CA")
	insecure := os.Getenv("VEILKEY_TLS_INSECURE") == "1"

	if caPath == "" && !insecure {
		// Default client with TLS support (uses system CA pool)
		return &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	client, err := NewTLSHTTPClient(caPath, insecure)
	if err != nil {
		// Fall back to default client
		return &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return client
}

// AgentScheme returns the URL scheme for agent communication.
// If VEILKEY_AGENT_SCHEME is set, that value is used directly.
// Otherwise defaults to "https" when TLS is configured (VEILKEY_TLS_CERT is set),
// or "http" for backward compatibility when TLS is not configured.
func AgentScheme() string {
	if scheme := os.Getenv("VEILKEY_AGENT_SCHEME"); scheme != "" {
		return scheme
	}
	if os.Getenv("VEILKEY_TLS_CERT") != "" {
		return "https"
	}
	return "http"
}
