package main

import (
	"math"
	"strings"
	"testing"
)

func testConfig() *CompiledConfig {
	cfg, err := LoadConfig("")
	if err != nil {
		panic(err)
	}
	return cfg
}

func testDetector() *SecretDetector {
	cfg := testConfig()
	logger := NewSessionLogger("/tmp/veilkey-cli-test/session.log")
	return NewSecretDetector(cfg, nil, logger, true)
}

func TestShannonEntropy(t *testing.T) {
	// Low entropy (repeated characters)
	low := shannonEntropy("aaaaaaaaaaaaaaaa")
	if low != 0 {
		t.Errorf("expected 0, got %f", low)
	}

	// High entropy (random)
	high := shannonEntropy("aB3$xY9!mK2@pL5#")
	if high < 3.5 {
		t.Errorf("expected > 3.5, got %f", high)
	}

	// Empty string
	if shannonEntropy("") != 0 {
		t.Error("empty string should have 0 entropy")
	}
}

func TestDetectGitHubPAT(t *testing.T) {
	d := testDetector()
	token := "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a"
	results := d.DetectSecrets(token)
	if len(results) == 0 {
		t.Fatal("expected to detect GitHub PAT")
	}
	if results[0].Pattern != "github-pat" {
		t.Errorf("expected github-pat, got %s", results[0].Pattern)
	}
}

func TestDetectGitLabPAT(t *testing.T) {
	d := testDetector()
	results := d.DetectSecrets("glpat-AbCdEfGhIjKlMnOpQrSt1234")
	if len(results) == 0 {
		t.Fatal("expected to detect GitLab PAT")
	}
	if results[0].Pattern != "gitlab-pat" {
		t.Errorf("expected gitlab-pat, got %s", results[0].Pattern)
	}
}

func TestDetectAWSKey(t *testing.T) {
	d := testDetector()
	// AWS access key pattern requires AKIA + [A-Z2-7]{16}
	results := d.DetectSecrets("AKIAIOSFODNNENEXAMPL")
	if len(results) == 0 {
		t.Fatal("expected to detect AWS access key")
	}
}

func TestExcludeTestValues(t *testing.T) {
	d := testDetector()
	// Values containing "example" should be excluded
	results := d.DetectSecrets("password=example_token_placeholder")
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Value), "example") {
			t.Error("should exclude values containing 'example'")
		}
	}
}

func TestDoubleSubstitutionPrevention(t *testing.T) {
	d := testDetector()

	// Already replaced VeilKey values should be protected
	line := "VK:5:YWJjMTIz ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a"
	processed := d.ProcessLine(line)

	if !strings.Contains(processed, "VK:5:YWJjMTIz") {
		t.Error("existing VeilKey should be preserved")
	}
	if strings.Contains(processed, "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a") {
		t.Error("new secret should be detected and replaced")
	}
}

func TestScopedVKTokenProtection(t *testing.T) {
	d := testDetector()

	tests := []struct {
		name  string
		token string
	}{
		{"TEMP token", "VK:TEMP:abcd1234ef567890"},
		{"LOCAL token", "VK:LOCAL:a1b2c3d4"},
		{"EXTERNAL token", "VK:EXTERNAL:abcd1234abcd1234"},
	}

	for _, tt := range tests {
		line := tt.token + " ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a"
		processed := d.ProcessLine(line)

		if !strings.Contains(processed, tt.token) {
			t.Errorf("%s: scoped VK token %s should be preserved, got: %s", tt.name, tt.token, processed)
		}
		if strings.Contains(processed, "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a") {
			t.Errorf("%s: new secret next to scoped token should still be detected", tt.name)
		}
	}
}

func TestMultipleSecretsPerLine(t *testing.T) {
	d := testDetector()
	line := "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a glpat-AbCdEfGhIjKlMnOpQrSt1234"
	results := d.DetectSecrets(line)
	if len(results) < 2 {
		t.Errorf("expected 2+ detections, got %d", len(results))
	}
}

func TestContextBoost(t *testing.T) {
	d := testDetector()

	// Use a GitHub PAT which is always detected regardless of context
	token := "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ123456789a"

	// Without context keyword
	r1 := d.DetectSecrets(token)
	// With context keyword ("password" triggers sensitive context boost)
	r2 := d.DetectSecrets("password=" + token)

	if len(r1) == 0 || len(r2) == 0 {
		t.Fatal("expected detections")
	}

	// Context should result in higher confidence
	if r2[0].Confidence <= r1[0].Confidence {
		t.Errorf("context should boost confidence: %d <= %d", r2[0].Confidence, r1[0].Confidence)
	}
}

func TestEntropyCalculation(t *testing.T) {
	// 2 characters with uniform distribution → entropy = 1.0
	e := shannonEntropy("ab")
	if math.Abs(e-1.0) > 0.01 {
		t.Errorf("expected ~1.0, got %f", e)
	}
}
