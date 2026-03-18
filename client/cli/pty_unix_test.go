//go:build !windows

package main

import "testing"

func TestTransformPastedInputLeavesExistingRefs(t *testing.T) {
	t.Setenv("VEILKEY_PLAINTEXT_ACTION", "issue-temp-and-block")
	t.Setenv("VEILKEY_STATE_DIR", t.TempDir())

	d := &SecretDetector{config: &CompiledConfig{}, cache: map[string]string{}, scanOnly: true}
	got := transformPastedInput(d, []byte("VK:TEMP:abcd1234\n"))
	if got != "VK:TEMP:abcd1234\n" {
		t.Fatalf("expected existing ref to pass through, got %q", got)
	}
}

func TestTransformPastedInputLeavesKnownCommands(t *testing.T) {
	t.Setenv("VEILKEY_PLAINTEXT_ACTION", "issue-temp-and-block")
	t.Setenv("VEILKEY_STATE_DIR", t.TempDir())

	d := &SecretDetector{config: &CompiledConfig{}, cache: map[string]string{}, scanOnly: true}
	got := transformPastedInput(d, []byte("printf demo\n"))
	if got != "printf demo\n" {
		t.Fatalf("expected plain command to pass through, got %q", got)
	}
}

func TestTransformPastedInputIssuesTempForStandalonePayload(t *testing.T) {
	t.Setenv("VEILKEY_PLAINTEXT_ACTION", "issue-temp-and-block")
	t.Setenv("VEILKEY_STATE_DIR", t.TempDir())

	logger := NewSessionLogger(t.TempDir() + "/session.log")
	d := &SecretDetector{
		config:   &CompiledConfig{},
		client:   &VeilKeyClient{},
		logger:   logger,
		cache:    map[string]string{"demo-secret-value-1234": "VK:TEMP:testref"},
		scanOnly: false,
	}

	got := transformPastedInput(d, []byte("demo-secret-value-1234\n"))
	if got != "VK:TEMP:testref\n" {
		t.Fatalf("expected standalone payload to issue temp ref, got %q", got)
	}
}

func TestTransformPastedInputRespectsPasteModeOff(t *testing.T) {
	t.Setenv("VEILKEY_PLAINTEXT_ACTION", "issue-temp-and-block")
	t.Setenv("VEILKEY_STATE_DIR", t.TempDir())

	if err := setPasteMode("off"); err != nil {
		t.Fatalf("set paste mode: %v", err)
	}

	d := &SecretDetector{config: &CompiledConfig{}, cache: map[string]string{}, scanOnly: true}
	got := transformPastedInput(d, []byte("standalone-secret-value\n"))
	if got != "standalone-secret-value\n" {
		t.Fatalf("expected standalone payload to remain unchanged when paste mode is off, got %q", got)
	}
}

func TestTransformPastedInputLeavesTerminalControlSequence(t *testing.T) {
	t.Setenv("VEILKEY_PLAINTEXT_ACTION", "issue-temp-and-block")
	t.Setenv("VEILKEY_STATE_DIR", t.TempDir())

	logger := NewSessionLogger(t.TempDir() + "/session.log")
	d := &SecretDetector{
		config:   &CompiledConfig{},
		client:   &VeilKeyClient{},
		logger:   logger,
		cache:    map[string]string{"\x1b[58;1u": "VK:TEMP:should-not-issue"},
		scanOnly: false,
	}

	got := transformPastedInput(d, []byte("\x1b[58;1u"))
	if got != "\x1b[58;1u" {
		t.Fatalf("expected control sequence to pass through unchanged, got %q", got)
	}
	if d.Stats.Detections != 0 {
		t.Fatalf("expected no detections for terminal control sequence, got %d", d.Stats.Detections)
	}
}
