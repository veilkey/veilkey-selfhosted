package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"net/http"
	"net/http/httptest"

	"veilkey-localvault/internal/db"
)

func TestResolveManagedPathsNormalizesAbsolutePaths(t *testing.T) {
	t.Setenv("VEILKEY_MANAGED_PATHS", "/Users/demo/work/app, /Users/demo/work/app/../app/sub ,relative,/Users/demo/work/app")

	got := resolveManagedPaths()
	want := []string{"/Users/demo/work/app", "/Users/demo/work/app/sub"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("managed_paths = %#v, want %#v", got, want)
	}
}

func TestResolveManagedPathsEmptyWhenUnset(t *testing.T) {
	t.Setenv("VEILKEY_MANAGED_PATHS", "")
	if got := resolveManagedPaths(); len(got) != 0 {
		t.Fatalf("managed_paths = %#v, want empty", got)
	}
}

func TestResolveManagedPathsFromContextFile(t *testing.T) {
	dir := t.TempDir()
	contextDir := filepath.Join(dir, ".veilkey")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	contextFile := filepath.Join(contextDir, "context.json")
	if err := os.WriteFile(contextFile, []byte("{\"version\":1,\"managed_path\":\"/srv/apps/demo\"}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("VEILKEY_MANAGED_PATHS", "")
	t.Setenv("VEILKEY_CONTEXT_FILE", contextFile)

	got := resolveManagedPaths()
	want := []string{"/srv/apps/demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("managed_paths = %#v, want %#v", got, want)
	}
}

func TestSendHeartbeatOnceAppliesPlannedRotation(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "localvault.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := database.SaveNodeInfo(&db.NodeInfo{NodeID: "node-rotate", DEK: []byte("dummy-dek"), DEKNonce: []byte("dummy-nonce12"), Version: 7}); err != nil {
		t.Fatalf("SaveNodeInfo: %v", err)
	}
	server := NewServer(database, nil, nil)
	server.SetIdentity(&NodeIdentity{NodeID: "node-rotate", Version: 7, VaultHash: "93a8094e", VaultName: "rotate-vault"})

	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if attempts == 1 {
			if payload["key_version"] != float64(7) {
				t.Fatalf("first key_version = %#v", payload["key_version"])
			}
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":               "rotation_required",
				"expected_key_version": 8,
				"rotation_required":    true,
			})
			return
		}
		if payload["key_version"] != float64(8) {
			t.Fatalf("second key_version = %#v", payload["key_version"])
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	if err := server.SendHeartbeatOnce(ts.URL, "rotate-vault", 10180); err == nil || err.Error() != "rotation_required" {
		t.Fatalf("expected rotation_required sentinel, got %v", err)
	}
	info, err := database.GetNodeInfo()
	if err != nil {
		t.Fatalf("GetNodeInfo: %v", err)
	}
	if info.Version != 8 {
		t.Fatalf("node version = %d, want 8", info.Version)
	}
	if server.identity.Version != 8 {
		t.Fatalf("identity version = %d, want 8", server.identity.Version)
	}

	body, err := json.Marshal(map[string]any{
		"vault_node_uuid": server.identity.NodeID,
		"node_id":         server.identity.NodeID,
		"vault_hash":      server.identity.VaultHash,
		"vault_name":      server.identity.VaultName,
		"key_version":     server.identity.Version,
		"label":           "rotate-vault",
		"port":            10180,
		"secrets_count":   0,
		"configs_count":   0,
		"version":         server.identity.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	_ = resp.Body.Close()
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestSendHeartbeatOnceUsesLatestNodeVersionFromDB(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "localvault.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := database.SaveNodeInfo(&db.NodeInfo{NodeID: "node-latest", DEK: []byte("dummy-dek"), DEKNonce: []byte("dummy-nonce12"), Version: 7}); err != nil {
		t.Fatalf("SaveNodeInfo: %v", err)
	}
	server := NewServer(database, nil, nil)
	server.SetIdentity(&NodeIdentity{NodeID: "node-latest", Version: 6, VaultHash: "abcd1234", VaultName: "latest-vault"})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["key_version"] != float64(7) {
			t.Fatalf("key_version = %#v, want 7 from db", payload["key_version"])
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	if err := server.SendHeartbeatOnce(ts.URL, "latest-vault", 10180); err != nil {
		t.Fatalf("SendHeartbeatOnce: %v", err)
	}
	if server.identity.Version != 7 {
		t.Fatalf("identity version = %d, want 7", server.identity.Version)
	}
}
