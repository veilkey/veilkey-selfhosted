package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestWatchlistMatching(t *testing.T) {
	cfg := testConfig()
	logger := NewSessionLogger("/tmp/veilkey-cli-test/session.log")
	d := NewSecretDetector(cfg, nil, logger, true)

	// Manually add watchlist entries
	d.watchlist = []WatchEntry{
		{Value: "super-secret-password-123", VK: "VK:5:c3VwZXI="},
		{Value: "another-api-key-456", VK: "VK:5:YW5vdGhlcg=="},
	}

	line := "connecting with super-secret-password-123 to database"
	processed := d.ProcessLine(line)

	if strings.Contains(processed, "super-secret-password-123") {
		t.Error("watchlist value should be replaced")
	}
	if !strings.Contains(processed, "VK:5:c3VwZXI=") {
		t.Error("should contain VK token replacement")
	}
}

func TestWatchlistMultipleValues(t *testing.T) {
	cfg := testConfig()
	logger := NewSessionLogger("/tmp/veilkey-cli-test/session.log")
	d := NewSecretDetector(cfg, nil, logger, true)

	d.watchlist = []WatchEntry{
		{Value: "secret-A", VK: "VK:5:QQ=="},
		{Value: "secret-B", VK: "VK:5:Qg=="},
	}

	line := "auth: secret-A, backup: secret-B"
	processed := d.ProcessLine(line)

	if strings.Contains(processed, "secret-A") || strings.Contains(processed, "secret-B") {
		t.Error("both watchlist values should be replaced")
	}
	if !strings.Contains(processed, "VK:5:QQ==") || !strings.Contains(processed, "VK:5:Qg==") {
		t.Error("both VK tokens should appear in output")
	}
}

func TestWatchlistPaused(t *testing.T) {
	cfg := testConfig()
	logger := NewSessionLogger("/tmp/veilkey-cli-test/session.log")
	d := NewSecretDetector(cfg, nil, logger, true)

	d.watchlist = []WatchEntry{
		{Value: "secret-value", VK: "VK:5:c2VjcmV0"},
	}

	// When paused, watchlist should not match
	d.Paused = true
	line := "data: secret-value"
	processed := d.ProcessLine(line)

	if !strings.Contains(processed, "secret-value") {
		t.Error("when paused, watchlist values should NOT be replaced")
	}

	// When resumed, watchlist should match
	d.Paused = false
	processed2 := d.ProcessLine(line)

	if strings.Contains(processed2, "secret-value") {
		t.Error("when active, watchlist values should be replaced")
	}
}

func TestWatchlistFileLoad(t *testing.T) {
	dir := "/tmp/veilkey-cli-test-watchlist"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)

	// Write a test watchlist file
	watchlistPath := dir + "/watchlist"
	content := "my-password-123\tVK:5:cGFzcw==\nanother-key\tVK:5:a2V5\n"
	os.WriteFile(watchlistPath, []byte(content), 0644)

	// Set state dir env
	os.Setenv("VEILKEY_STATE_DIR", dir)
	defer os.Unsetenv("VEILKEY_STATE_DIR")

	cfg := testConfig()
	logger := NewSessionLogger(dir + "/session.log")
	d := NewSecretDetector(cfg, nil, logger, true)

	if len(d.watchlist) != 2 {
		t.Fatalf("expected 2 watchlist entries, got %d", len(d.watchlist))
	}
	if d.watchlist[0].Value != "my-password-123" {
		t.Errorf("expected my-password-123, got %s", d.watchlist[0].Value)
	}
	if d.watchlist[0].VK != "VK:5:cGFzcw==" {
		t.Errorf("expected VK:5:cGFzcw==, got %s", d.watchlist[0].VK)
	}
}

func TestReloadWatchlist(t *testing.T) {
	dir := "/tmp/veilkey-cli-test-reload"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)

	watchlistPath := dir + "/watchlist"
	os.WriteFile(watchlistPath, []byte("val1\tVK:5:djE=\n"), 0644)

	os.Setenv("VEILKEY_STATE_DIR", dir)
	defer os.Unsetenv("VEILKEY_STATE_DIR")

	cfg := testConfig()
	logger := NewSessionLogger(dir + "/session.log")
	d := NewSecretDetector(cfg, nil, logger, true)

	if len(d.watchlist) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(d.watchlist))
	}

	// Add a new entry to the file
	os.WriteFile(watchlistPath, []byte("val1\tVK:5:djE=\nval2\tVK:5:djI=\n"), 0644)

	d.ReloadWatchlist()

	if len(d.watchlist) != 2 {
		t.Fatalf("after reload, expected 2 entries, got %d", len(d.watchlist))
	}
}

func TestWatchlistExpiryPrune(t *testing.T) {
	dir := "/tmp/veilkey-cli-test-expiry"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)

	past := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	content := "expired-val\tVK:TEMP:dead\t" + past + "\n" +
		"valid-val\tVK:TEMP:beef\t" + future + "\n" +
		"no-ttl-val\tVK:5:abcd1234\n"
	watchlistPath := dir + "/watchlist"
	os.WriteFile(watchlistPath, []byte(content), 0644)

	os.Setenv("VEILKEY_STATE_DIR", dir)
	defer os.Unsetenv("VEILKEY_STATE_DIR")

	cfg := testConfig()
	logger := NewSessionLogger(dir + "/session.log")
	d := NewSecretDetector(cfg, nil, logger, true)

	if len(d.watchlist) != 2 {
		t.Fatalf("expected 2 watchlist entries after prune, got %d", len(d.watchlist))
	}
	if d.watchlist[0].Value != "valid-val" {
		t.Errorf("expected valid-val first, got %s", d.watchlist[0].Value)
	}
	if d.watchlist[1].Value != "no-ttl-val" {
		t.Errorf("expected no-ttl-val second, got %s", d.watchlist[1].Value)
	}

	data, _ := os.ReadFile(watchlistPath)
	if strings.Contains(string(data), "expired-val") {
		t.Error("expired entry should be removed from watchlist file")
	}
	if !strings.Contains(string(data), "valid-val") {
		t.Error("valid entry should remain in watchlist file")
	}
}
