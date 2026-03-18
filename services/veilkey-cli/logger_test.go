package main

import (
	"os"
	"testing"
)

func TestSessionLoggerLogAndRead(t *testing.T) {
	dir := "/tmp/veilkey-cli-test-logger"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)

	logPath := dir + "/session.log"
	logger := NewSessionLogger(logPath)

	logger.Log("VK:5:abc", "github-pat", 90, "ghp_***")
	logger.Log("VK:5:def", "aws-key", 85, "AKIA***")

	entries, err := logger.ReadEntries()
	if err != nil {
		t.Fatalf("ReadEntries failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].VeilKey != "VK:5:abc" {
		t.Errorf("expected VK:5:abc, got %s", entries[0].VeilKey)
	}
	if entries[0].Pattern != "github-pat" {
		t.Errorf("expected github-pat, got %s", entries[0].Pattern)
	}
	if entries[1].Confidence != 85 {
		t.Errorf("expected confidence 85, got %d", entries[1].Confidence)
	}
}

func TestSessionLoggerCount(t *testing.T) {
	dir := "/tmp/veilkey-cli-test-count"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)

	logPath := dir + "/session.log"
	logger := NewSessionLogger(logPath)

	if logger.Count() != 0 {
		t.Error("empty log should have count 0")
	}

	logger.Log("VK:5:a", "test", 80, "***")
	logger.Log("VK:5:b", "test", 80, "***")
	logger.Log("VK:5:c", "test", 80, "***")

	if logger.Count() != 3 {
		t.Errorf("expected count 3, got %d", logger.Count())
	}
}

func TestSessionLoggerClear(t *testing.T) {
	dir := "/tmp/veilkey-cli-test-clear"
	os.MkdirAll(dir, 0755)
	defer os.RemoveAll(dir)

	logPath := dir + "/session.log"
	logger := NewSessionLogger(logPath)

	logger.Log("VK:5:a", "test", 80, "***")

	if logger.Count() != 1 {
		t.Fatal("expected 1 entry before clear")
	}

	logger.Clear()

	if logger.Count() != 0 {
		t.Error("after clear, count should be 0")
	}
}
