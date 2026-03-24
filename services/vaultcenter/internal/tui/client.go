package tui

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"net/http/cookiejar"
	"time"
)

// Client is an HTTP client for the VaultCenter admin API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a new API client for the given VaultCenter URL.
// Set VEILKEY_TLS_INSECURE=1 to skip TLS certificate verification.
func NewClient(baseURL string) *Client {
	jar, _ := cookiejar.New(nil)
	insecure := os.Getenv("VEILKEY_TLS_INSECURE") == "1"
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 10 * time.Second,
			Jar:     jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
			},
		},
	}
}

// ── Types ──

type TempRef struct {
	RefCanonical string     `json:"ref_canonical"`
	SecretName   string     `json:"secret_name"`
	AgentHash    string     `json:"agent_hash"`
	Status       string     `json:"status"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ── Auth ──

// Unlock sends the master password to unlock the server.
func (c *Client) Unlock(password string) error {
	body, _ := json.Marshal(map[string]string{"password": password})
	resp, err := c.http.Post(c.baseURL+"/api/unlock", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unlock request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unlock failed (%d): %s", resp.StatusCode, string(msg))
	}
	return nil
}

// LoginTOTP authenticates with a TOTP code.
func (c *Client) LoginTOTP(code string) error {
	body, _ := json.Marshal(map[string]string{"code": code})
	resp, err := c.http.Post(c.baseURL+"/api/admin/session/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed (%d): %s", resp.StatusCode, string(msg))
	}
	return nil
}

// LoginPassword authenticates with admin password.
func (c *Client) LoginPassword(password string) error {
	body, _ := json.Marshal(map[string]string{"password": password})
	resp, err := c.http.Post(c.baseURL+"/api/admin/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed (%d): %s", resp.StatusCode, string(msg))
	}
	return nil
}

func (c *Client) Status() (map[string]any, error) {
	return c.getJSON("/api/status")
}

// ── Keycenter ──

func (c *Client) ListTempRefs() ([]TempRef, error) {
	data, err := c.getJSON("/api/keycenter/temp-refs")
	if err != nil {
		return nil, err
	}
	return decodeList[TempRef](data, "refs")
}

func (c *Client) RevealRef(ref string) (string, error) {
	data, err := c.getJSON("/api/keycenter/temp-refs/" + ref + "/value")
	if err != nil {
		return "", err
	}
	if v, ok := data["value"].(string); ok {
		return v, nil
	}
	return "", fmt.Errorf("no value in response")
}

func (c *Client) CreateTempRef(name, value string) (map[string]any, error) {
	return c.postJSON("/api/keycenter/temp-refs", marshal(map[string]string{"name": name, "value": value}))
}

func (c *Client) PromoteRef(ref, name, vaultHash string) (map[string]any, error) {
	return c.postJSON("/api/keycenter/promote", marshal(map[string]string{
		"ref": ref, "name": name, "vault_hash": vaultHash,
	}))
}

// ── Vaults ──

func (c *Client) ListVaults() ([]map[string]any, error) {
	data, err := c.getJSON("/api/vault-inventory")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "vaults")
}

// GetVaultKeys retrieves secrets for a vault. Use vault_runtime_hash, not vault_hash.
func (c *Client) GetVaultKeys(runtimeHash string) ([]map[string]any, error) {
	data, err := c.getJSON("/api/vaults/" + runtimeHash + "/keys")
	if err != nil {
		return nil, err
	}
	list, err := decodeList[map[string]any](data, "secrets")
	if err != nil || len(list) == 0 {
		list, _ = decodeList[map[string]any](data, "keys")
	}
	return list, nil
}

func (c *Client) GetSecretMeta(vaultHash, name string) (map[string]any, error) {
	return c.getJSON("/api/vaults/" + vaultHash + "/keys/" + name + "/meta")
}

func (c *Client) GetSecretBindings(vaultHash, name string) ([]map[string]any, error) {
	data, err := c.getJSON("/api/vaults/" + vaultHash + "/keys/" + name + "/bindings")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "bindings")
}

func (c *Client) GetVaultAudit(vaultHash string) ([]map[string]any, error) {
	data, err := c.getJSON("/api/vaults/" + vaultHash + "/audit")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "events")
}

// ── Secret Create ──

func (c *Client) CreateVaultSecret(runtimeHash, name, value string) (map[string]any, error) {
	return c.postJSON("/api/vaults/"+runtimeHash+"/keys", marshal(map[string]string{
		"name":  name,
		"value": value,
	}))
}

// ── Secret Update/Delete ──

func (c *Client) UpdateVaultSecret(runtimeHash, name, value string) (map[string]any, error) {
	return c.putJSON("/api/vaults/"+runtimeHash+"/keys/"+name, marshal(map[string]string{
		"name":  name,
		"value": value,
	}))
}

func (c *Client) DeleteVaultSecret(runtimeHash, name string) error {
	return c.deleteJSON("/api/vaults/" + runtimeHash + "/keys/" + name)
}

// ── Config CRUD ──

func (c *Client) SaveConfig(key, value string) (map[string]any, error) {
	return c.postJSON("/api/configs", marshal(map[string]string{"key": key, "value": value}))
}

func (c *Client) DeleteConfig(key string) error {
	return c.deleteJSON("/api/configs/" + key)
}

// ── Approvals ──

func (c *Client) ListRebindApprovals() ([]map[string]any, error) {
	data, err := c.getJSON("/api/admin/approvals/rebind")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "agents")
}

func (c *Client) ApproveRebind(agent string) (map[string]any, error) {
	return c.postJSON("/api/admin/approvals/rebind/"+agent+"/approve", []byte("{}"))
}

// ── Secret Reveal ──

// RevealAuthorize opens a reveal window for a secret ref.
func (c *Client) RevealAuthorize(ref, reason string) error {
	body := marshal(map[string]string{"ref": ref, "reason": reason})
	_, err := c.postJSON("/api/admin/reveal-authorize", body)
	return err
}

// RevealSecret decrypts a vault secret (requires prior authorize).
func (c *Client) RevealSecret(ref string) (string, error) {
	data, err := c.postJSON("/api/admin/reveal", marshal(map[string]string{"ref": ref}))
	if err != nil {
		return "", err
	}
	if v, ok := data["value"].(string); ok {
		return v, nil
	}
	return "", fmt.Errorf("no value in response")
}

// ── Agents ──

func (c *Client) ListAgents() ([]map[string]any, error) {
	data, err := c.getJSON("/api/agents")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "agents")
}

func (c *Client) ScheduleAllRotations() (map[string]any, error) {
	return c.postJSON("/api/admin/rotations/schedule-all", []byte("{}"))
}

// ── Functions ──

func (c *Client) ListFunctions() ([]map[string]any, error) {
	data, err := c.getJSON("/api/functions/global")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "functions")
}

func (c *Client) RunFunction(name string) (map[string]any, error) {
	return c.postJSON("/api/functions/global/"+name+"/run", []byte("{}"))
}

func (c *Client) ListBindings() ([]map[string]any, error) {
	// Bindings requires binding_type+target_name, use vault-level bindings instead
	data, err := c.getJSON("/api/catalog/secrets")
	if err != nil {
		return nil, err
	}
	// Extract binding info from secrets that have bindings
	secrets, _ := decodeList[map[string]any](data, "secrets")
	var bindings []map[string]any
	for _, s := range secrets {
		if bc, ok := s["binding_count"]; ok {
			if count, ok := bc.(float64); ok && count > 0 {
				bindings = append(bindings, s)
			}
		}
	}
	return bindings, nil
}

// ── Catalog & Audit ──

func (c *Client) ListSecretCatalog() ([]map[string]any, error) {
	data, err := c.getJSON("/api/catalog/secrets")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "secrets")
}

func (c *Client) ListAuditEvents() ([]map[string]any, error) {
	data, err := c.getJSON("/api/admin/audit/recent")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "events")
}

// ── Settings ──

func (c *Client) AuthSettings() (map[string]any, error) {
	return c.getJSON("/api/admin/auth/settings")
}

func (c *Client) ListRegistrationTokens() ([]map[string]any, error) {
	data, err := c.getJSON("/api/admin/registration-tokens")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "tokens")
}

func (c *Client) CreateRegistrationToken(label string) (map[string]any, error) {
	return c.postJSON("/api/admin/registration-tokens", marshal(map[string]string{"label": label}))
}

func (c *Client) RevokeRegistrationToken(tokenID string) error {
	return c.deleteJSON("/api/admin/registration-tokens/" + tokenID)
}

func (c *Client) GetNodeInfo() (map[string]any, error) {
	return c.getJSON("/api/node-info")
}

func (c *Client) ListConfigs() ([]map[string]any, error) {
	data, err := c.getJSON("/api/configs")
	if err != nil {
		return nil, err
	}
	return decodeList[map[string]any](data, "configs")
}

// ── HTTP helpers ──

func (c *Client) getJSON(path string) (map[string]any, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

func (c *Client) postJSON(path string, body []byte) (map[string]any, error) {
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

func (c *Client) putJSON(path string, body []byte) (map[string]any, error) {
	req, err := http.NewRequest("PUT", c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

func (c *Client) deleteJSON(path string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func decodeList[T any](data map[string]any, key string) ([]T, error) {
	raw, ok := data[key]
	if !ok {
		return nil, nil
	}
	b, _ := json.Marshal(raw)
	var list []T
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", key, err)
	}
	return list, nil
}

func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
