package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type VeilKeyClient struct {
	baseURL string
	client  *http.Client
	cache   sync.Map
}

type ExactLookupMatch struct {
	Ref        string `json:"ref"`
	Family     string `json:"family"`
	Scope      string `json:"scope"`
	ID         string `json:"id"`
	SecretName string `json:"secret_name,omitempty"`
	Status     string `json:"status,omitempty"`
}

func NewVeilKeyClient(baseURL string) *VeilKeyClient {
	return &VeilKeyClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *VeilKeyClient) Issue(value string) (string, error) {
	if vk, ok := c.cache.Load(value); ok {
		return vk.(string), nil
	}

	body, err := json.Marshal(map[string]string{"plaintext": value})
	if err != nil {
		return "", err
	}

	resp, err := c.client.Post(
		c.baseURL+"/api/encrypt",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("API returned %d: (unreadable body)", resp.StatusCode)
		}
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	c.cache.Store(value, result.Token)
	return result.Token, nil
}

func (c *VeilKeyClient) Resolve(token string) (string, error) {
	refs := resolveCandidates(token)
	var lastErr error
	for _, ref := range refs {
		value, err := c.resolveOnce(ref)
		if err == nil {
			return value, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("resolve failed: no candidates")
	}
	return "", lastErr
}

func (c *VeilKeyClient) ExactLookup(plaintext string) ([]ExactLookupMatch, error) {
	body, err := json.Marshal(map[string]string{"plaintext": plaintext})
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Post(
		c.baseURL+"/api/lookup/exact",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("exact lookup failed %d: (unreadable body)", resp.StatusCode)
		}
		return nil, fmt.Errorf("exact lookup failed %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Matches []ExactLookupMatch `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Matches, nil
}

func resolveCandidates(token string) []string {
	ref := token
	switch {
	case strings.HasPrefix(ref, "VK:") || strings.HasPrefix(ref, "VE:"):
		// Keep scoped refs intact (VK:SCOPE:REF / VE:SCOPE:KEY).
		// Only trim the legacy single-colon form (VK:hash / VE:key).
		if strings.Count(ref, ":") == 1 {
			if idx := strings.IndexByte(ref, ':'); idx >= 0 {
				return []string{ref[idx+1:]}
			}
		}
		parts := strings.SplitN(ref, ":", 3)
		if len(parts) == 3 && parts[0] == "VK" {
			return []string{ref, parts[2]}
		}
		return []string{ref}
	default:
		return []string{ref}
	}
}

func (c *VeilKeyClient) resolveOnce(ref string) (string, error) {
	resp, err := c.client.Get(c.baseURL + "/api/resolve/" + url.PathEscape(ref))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("resolve failed %d: (unreadable body)", resp.StatusCode)
		}
		return "", fmt.Errorf("resolve failed %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Value, nil
}

func (c *VeilKeyClient) HealthCheck() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(c.baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
